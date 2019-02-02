package backend

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"sync"

	"github.com/fberrez/samantha/backend/provider"
	"github.com/fberrez/samantha/backend/provider/watson"
	"github.com/google/uuid"
	"github.com/juju/errors"
	log "github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v2"
)

type (
	// Backend is the application backend. It manages a list of activated
	// backend providers. These are clients of some NLP/NLU services
	// such as IBM Watson, Google Dialogflow...
	Backend struct {
		// activatedProvider is the running backend provider.
		activatedProvider provider.Provider

		// capsuleInChan is a only-read channel which is used to receive capsule containing
		// all informations about a new user input, sent by the frontend.
		capsuleInChan <-chan []byte

		// backendErrorChan is the channel which sends to the frontend errors that
		// could occured on the backend side.
		backendErrorChan chan<- []byte

		// wg is local wait group which handles all providers routines.
		wg *sync.WaitGroup
	}

	// CapsuleIn is the capsule containing all informations about a new
	// user input received on the frontend part.
	CapsuleIn struct {
		// RespondTo is the original message UUID corresponding to this capsule.
		RespondTo uuid.UUID `json:"respondTo" yaml:"respondTo"`

		// ProviderLabel is the provider label (ex: telegram).
		ProviderLabel string `json:"label" yaml:"label"`

		// ContentType is the input type
		ContentType provider.ContentType `json:"contentType" yaml:"contentType"`

		// Content is a slice of bytes containing the user input received by a provider
		// and send to the frontend package. It can be a text or a media such as an
		// image.
		Content []byte `json:"input" yaml:"input"`

		// User is the name of the user
		User string `json:"user" yaml:"user"`
	}

	// CapsuleOut is a response that can be sent to the frontend when an error
	// occured.
	CapsuleOut struct {
		// RespondTo is the original message UUID corresponding to this capsule.
		RespondTo uuid.UUID `json:"respondTo" yaml:"respondTo"`

		// ProviderLabel is the provider label (ex: telegram).
		ProviderLabel string `json:"label" yaml:"label"`

		// ContentType is the input type
		ContentType provider.ContentType `json:"contentType" yaml:"contentType"`

		// Content is a slice of bytes containing the user input received by a provider
		// and send to the frontend package. It can be a text or a media such as an
		// image.
		Content []byte `json:"input" yaml:"input"`

		// User is the name of the user
		User string `json:"user" yaml:"user"`

		// Error is the error when the
		Error error `json:"error" yamle:"error"`
	}
)

const (
	// configFile is the name of the environment variable
	// containing the the configuration file path.
	configFile = "BACKEND_CONFIG_FILE"

	// defaultConfigFilePath is the default path of the configuration file
	// when the environment variable has not been initialized.
	defaultConfigFilePath = "backend/config.yaml"
)

var (
	// logger is a global logger of the package
	logger = log.WithField("package", "backend")

	// providerCollection indexes all implemented providers.
	providerCollection map[string]provider.Provider = map[string]provider.Provider{
		"watson": &watson.Watson{},
	}
)

// New initiliazes a new backend providers manager.
func New(backendErrorChan chan<- []byte, capsuleInChan <-chan []byte) (*Backend, error) {
	// Loads a new structured configuration with the informations of a given
	// configuration file.
	providerConfig, err := loadConfig()
	if err != nil {
		return nil, errors.Annotate(err, "initiliazing frontend")
	}

	// Loads backend providers defined as activated.
	p, err := loadProvider(providerConfig)
	if err != nil {
		return nil, errors.Annotate(err, "initiliazing frontend")
	}

	return &Backend{
		activatedProvider: p,
		capsuleInChan:     capsuleInChan,
		backendErrorChan:  backendErrorChan,
		wg:                &sync.WaitGroup{},
	}, nil
}

// Start starts frontend providers and user inputs listening.
func (b *Backend) Start(wg *sync.WaitGroup) {
	defer wg.Done()
	localLogger := logger.WithField("action", "listening")

	// Initializes a local function which will stop all activated providers when
	// a channel has been closed.
	stop := func(b *Backend) {
		localLogger.Info("Closing backend providers")
		b.stopProvider()
		b.wg.Wait()
	}

	b.wg.Add(1)
	localLogger.Info("Starting listening loop")
listeningLoop:
	for {
		select {
		case data, ok := <-b.capsuleInChan:
			if !ok {
				stop(b)
				break listeningLoop
			}

			capsule, err := b.unmarshalCapsule(data)
			if err != nil {
				localLogger.WithError(err).Error("Error occured while receiving a new capsule sent by frontend")
				break
			}

			localLogger.Debugf("Capsule received from %s: %s", capsule.ProviderLabel, string(capsule.Content))
			response, err := b.activatedProvider.Message(string(capsule.Content))
			if err != nil {
				if err = b.errorHandler(capsule, err); err != nil {
					localLogger.WithError(err).Error("Error occured while sending capsule content to the backend provider")
				}
				break
			}

			localLogger.Debugf("Response received from %s: %s", b.activatedProvider.GetLabel(), response.String())
		}
	}
}

// loadConfig loads the providers configuration from file defined in a environment variable.
// It returns an array of structured providers configuration.
func loadConfig() (*provider.Config, error) {
	// Gets the config file path.
	path := os.Getenv(configFile)
	if path == "" {
		path = defaultConfigFilePath
	}

	log.WithField("filename", path).Info("Parsing config file")

	// Reads config file.
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.Annotate(err, "cannot read config file")
	}

	var c *provider.Config

	// Unmarshals the read bytes.
	if err = yaml.Unmarshal(data, &c); err != nil {
		return nil, errors.Annotate(err, "cannot unmarshal config file")
	}

	c.Label = strings.ToLower(c.Label)

	return c, nil
}

// loadProviders loads the providers if they are declared as activated.
func loadProvider(providerConfig *provider.Config) (provider.Provider, error) {
	p, ok := providerCollection[providerConfig.Label]
	if !ok {
		return nil, errors.NotFoundf("provider called `%s`", providerConfig.Label)
	}

	var err error
	p, err = p.Initialize(providerConfig)
	if err != nil {
		annotation := fmt.Sprintf("loading provider %s", providerConfig.Label)
		return nil, errors.Annotate(err, annotation)
	}

	return p, nil
}

// unmarshalCapsule unmarshals a given slice of bytes to a structured capsule.
// It also verifies its validity.
func (b *Backend) unmarshalCapsule(data []byte) (*CapsuleIn, error) {
	capsule := CapsuleIn{}

	// Unmarshals the slice of bytes
	if err := yaml.Unmarshal(data, &capsule); err != nil {
		return nil, errors.Annotate(err, "unmarshaling capsule")
	}

	// Verifies the capsule validity
	if err := capsuleIsValid(&capsule); err != nil {
		return nil, errors.Annotate(err, "unmarshaling capsule")
	}

	return &capsule, nil
}

// message sends the capsule content according to the input type.
// It returns a structured response.
func (b *Backend) message(capsule *CapsuleIn) (*provider.Response, error) {
	switch capsule.ContentType {
	case provider.Text:
		return b.activatedProvider.Message(string(capsule.Content))
	case provider.Image:
		return nil, errors.NotImplementedf("%s message handling", capsule.ContentType)
	case provider.Audio:
		return nil, errors.NotImplementedf("%s message handling", capsule.ContentType)
	default:
		return nil, errors.NotFoundf("input type %s", capsule.ContentType)
	}
}

// capsuleIsValid checks the capsule validity (all fields are correctly initialized
// with an other value than the zero value).
func capsuleIsValid(capsule *CapsuleIn) error {
	if capsule.RespondTo == uuid.Nil && capsule.RespondTo.Version().String() == "VERSION_4" {
		return errors.NotValidf("uuid %s", capsule.RespondTo)
	}

	if len(capsule.Content) == 0 {
		return errors.NotProvisionedf("content (capsule %s)", capsule.RespondTo)
	}

	if len(capsule.ContentType) == 0 {
		return errors.NotAssignedf("input type %s (capsule %s)", capsule.ContentType, capsule.RespondTo)
	}

	if len(capsule.ProviderLabel) == 0 {
		return errors.NotAssignedf("provider (capsule %s)", capsule.RespondTo)
	}

	if len(capsule.User) == 0 {
		return errors.NotAssignedf("user (capsule %s)", capsule.RespondTo)
	}

	return nil
}

// errorHandler handles error that can occured on sending message to backend
// providers. It marshal a CapsuleOut and sends it on the backend error channel.
func (b *Backend) errorHandler(original *CapsuleIn, err error) error {
	capsuleOut := &CapsuleOut{
		RespondTo:     original.RespondTo,
		ProviderLabel: original.ProviderLabel,
		ContentType:   provider.ErrorType,
		Content:       original.Content,
		User:          original.User,
		Error:         err,
	}

	data, err := yaml.Marshal(capsuleOut)
	if err != nil {
		return errors.Annotate(err, "handling error")
	}

	b.backendErrorChan <- data

	return nil
}

func (b *Backend) stopProvider() {
	b.activatedProvider.Stop()
	b.wg.Done()
}
