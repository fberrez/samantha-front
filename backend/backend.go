package backend

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"sync"

	"github.com/fberrez/samantha/backend/provider"
	"github.com/fberrez/samantha/backend/provider/watson"
	"github.com/fberrez/samantha/capsule"
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

		capsule chan *capsule.Capsule

		// wg is local wait group which handles all providers routines.
		wg *sync.WaitGroup
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
func New(capsuleChan chan *capsule.Capsule) (*Backend, error) {
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
		capsule:           capsuleChan,
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
		case capsule, ok := <-b.capsule:
			if !ok {
				stop(b)
				break listeningLoop
			}

			localLogger.Debugf("Capsule received from %s: %s", capsule.FrontendProvider, capsule.Content)
			response, err := b.activatedProvider.Message(capsule.Content)
			if err != nil {
				if err = b.errorHandler(capsule, err); err != nil {
					localLogger.WithError(err).Error("Error occurred while sending capsule content to the backend provider")
				}
				break
			}

			localLogger.Debugf("Response received from %s: %s", b.activatedProvider.GetLabel(), response.String())

			for _, output := range response.Outputs {
				capsule.Responses = append(capsule.Responses, output.Text)
			}

			b.capsule <- capsule
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

// errorHandler handles error that can occurred on sending message to backend
// providers. It marshal a CapsuleOut and sends it on the backend error channel.
func (b *Backend) errorHandler(original *capsule.Capsule, err error) error {
	original.Error = err

	b.capsule <- original

	return nil
}

func (b *Backend) stopProvider() {
	b.activatedProvider.Stop()
	b.wg.Done()
}
