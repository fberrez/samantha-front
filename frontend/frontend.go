package frontend

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"sync"

	"github.com/fberrez/samantha/frontend/provider"
	"github.com/fberrez/samantha/frontend/provider/telegram"
	"github.com/juju/errors"
	log "github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v2"
)

type (
	// Frontend is the application frontend. It manages a list of activated
	// frontend providers such as Telegram, Messenger...
	Frontend struct {
		// activatedProviders is a slice containing all activated frontend providers.
		activatedProviders []provider.Provider

		// userInput is a only-read channel which receives local capsules sent by
		// the frontend providers.
		userInput <-chan *provider.CapsuleProvider

		// backendErrorChan is the channel which receives errors that could occured
		// on the backend side.
		backendErrorChan <-chan []byte

		// capsuleOutChan is a only-write channel which is used to send capsule containing
		// all informations about a new user input to the backend.
		capsuleOutChan chan<- []byte

		// wg is local wait group which handles all providers routines.
		wg *sync.WaitGroup
	}

	// ProviderConfig is a structured provider configuration.
	ProviderConfig struct {
		// Label is the provider label.
		Label string `json:"label" yaml:"label"`

		// IsActivated defines if the provider is activated or not.
		IsActivated bool `json:"isActivated" yaml:"isActivated"`

		// Token is the API provider token
		Token string `json:"token" yaml:"token"`

		// AutorizedUsers is a slice containing all authorized users.
		// These users are authorized to use the frontend provider.
		AuthorizedUsers []*provider.User `json:"authorizedUsers" yaml:"authorizedUsers"`
	}
)

const (
	// configFile is the name of the environment variable
	// containing the the configuration file path.
	configFile = "FRONTEND_CONFIG_FILE"

	// defaultConfigFilePath is the default path of the configuration file
	// when the environment variable has not been initialized.
	defaultConfigFilePath = "frontend/config.yaml"
)

var (
	// logger is a global logger of the package
	logger = log.WithField("package", "frontend")

	// providerCollection indexes all implemented providers.
	providerCollection map[string]provider.Provider = map[string]provider.Provider{
		"telegram": &telegram.Telegram{},
	}
)

// New initiliazes a new frontend providers manager.
func New(backendErrorChan <-chan []byte, capsuleOutChan chan<- []byte) (*Frontend, error) {
	// Loads a new structured configuration with the informations of a given
	// configuration file.
	providerConfig, err := loadConfig()
	if err != nil {
		return nil, errors.Annotate(err, "initiliazing frontend")
	}

	// Initializes a userInput channel.
	userInput := make(chan *provider.CapsuleProvider)

	// Loads frontend providers defined as activated.
	providers, err := loadProvider(providerConfig, userInput)
	if err != nil {
		return nil, errors.Annotate(err, "initiliazing frontend")
	}

	return &Frontend{
		activatedProviders: providers,
		userInput:          userInput,
		backendErrorChan:   backendErrorChan,
		capsuleOutChan:     capsuleOutChan,
		wg:                 &sync.WaitGroup{},
	}, nil
}

// Start starts frontend providers and user inputs listening.
func (f *Frontend) Start(wg *sync.WaitGroup) {
	defer wg.Done()

	localLogger := logger.WithField("action", "listening")

	for _, provider := range f.activatedProviders {
		f.wg.Add(1)
		go provider.Start()
	}

	// Initializes a local function which will stop all activated providers when
	// a channel has been closed.
	stop := func(f *Frontend) {
		localLogger.Info("Closing frontend providers")
		f.stopProviders()
		f.wg.Wait()
	}

	localLogger.Info("Starting listening loop")
listeningLoop:
	for {
		select {
		case capsule, ok := <-f.userInput:
			if !ok {
				stop(f)
				break listeningLoop
			}

			localLogger.Debugf("Capsule received from %s: %s", capsule.ProviderLabel, string(capsule.Content))
			if err := f.sendToBackend(capsule); err != nil {
				// TODO: error handling (send error as syslog to the user)

				localLogger.WithError(err).Error("Error while sending capsule from frontend to backend.")
			}
		case data, ok := <-f.backendErrorChan:
			if !ok {
				stop(f)
				break listeningLoop
			}

			capsule := provider.CapsuleIn{}
			if err := yaml.Unmarshal(data, &capsule); err != nil {
				localLogger.WithError(err).Error("Cannot process error received from backend")
			}

			if err := f.message(&capsule); err != nil {
				localLogger.WithError(err).Error("Cannot process error received from backend")
			}
		}

	}

}

// loadConfig loads the providers configuration from file defined in a environment variable.
// It returns an array of structured providers configuration.
func loadConfig() ([]*ProviderConfig, error) {
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

	var c []*ProviderConfig

	// Unmarshals the read bytes.
	if err = yaml.Unmarshal(data, &c); err != nil {
		return nil, errors.Annotate(err, "cannot unmarshal config file")
	}

	// Formats label
	for _, provider := range c {
		provider.Label = strings.ToLower(provider.Label)
	}

	return c, nil
}

// loadProviders loads the providers if they are declared as activated.
func loadProvider(providerConfig []*ProviderConfig, userInput chan<- *provider.CapsuleProvider) ([]provider.Provider, error) {
	// providers is a slice containing initiliazed provider.
	providers := []provider.Provider{}

	// Each of the providers contained in the configuration slice are loaded
	// only if they are declared as activated.
	for _, pc := range providerConfig {
		// Verifies if the provider exists in the collection of implemented providers.
		p, ok := providerCollection[pc.Label]
		if !ok {
			return nil, errors.NotFoundf("provider called `%s`", pc.Label)
		}

		// If the provider is declared as activated in the configuration file,
		// it is initialized and added to the slice of providers.
		if pc.IsActivated {
			// Initializes a new provider config which will be sent to the provider
			// for initializing it.
			config := &provider.Config{
				Token:           pc.Token,
				AuthorizedUsers: pc.AuthorizedUsers,
				UserInput:       userInput,
			}

			var err error
			p, err = p.Initialize(config)
			if err != nil {
				annotation := fmt.Sprintf("loading provider %s", pc.Label)
				return nil, errors.Annotate(err, annotation)
			}

			providers = append(providers, p)
		}
	}

	return providers, nil
}

// sendToBackend sends a given capsule to the backend using the capsule out channel.
func (f *Frontend) sendToBackend(capsule *provider.CapsuleProvider) error {
	data, err := yaml.Marshal(capsule)
	if err != nil {
		return errors.Annotate(err, "sending capsule from frontend to backend")
	}

	f.capsuleOutChan <- data
	return nil
}

func (f *Frontend) message(capsule *provider.CapsuleIn) error {
	for _, p := range f.activatedProviders {
		if capsule.ProviderLabel == p.GetLabel() {
			return p.Message(capsule.RespondTo, capsule.ContentType, capsule.Content)
		}
	}

	return errors.NotFoundf("frontend provider %s", capsule.ProviderLabel)
}

// stopProviders stop all runing providers.
func (f *Frontend) stopProviders() {
	for _, p := range f.activatedProviders {
		p.Stop()
		f.wg.Done()
	}
}
