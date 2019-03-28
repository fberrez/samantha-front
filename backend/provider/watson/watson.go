package watson

import (
	"encoding/json"
	"strings"

	"github.com/fberrez/samantha/backend/provider"
	"github.com/google/uuid"
	"github.com/juju/errors"
	"github.com/watson-developer-cloud/go-sdk/assistantv2"
	"github.com/watson-developer-cloud/go-sdk/core"
)

type (
	// Watson is client which communicates with the IBM Watson Assistant API
	Watson struct {
		// service is a http client which will communicates with the API
		service *assistantv2.AssistantV2

		// userID is the unique identifier of the current session.
		userID uuid.UUID

		// assistantID is the ID of the Watson Assistant.
		// I can be found on the IBM Cloud (bluemix)
		assistantID string

		// SessionID is the ID of the connection session.
		sessionID *string
	}

	// Config is the struct representing the config file.
	// All these data are required to build a new connection.
	Config struct {
		// URL is the API URL.
		URL string `json:"url" yaml:"url"`

		// Version is the version of the API.
		Version string `json:"version" yaml:"yaml"`

		// IAMApiKey is the API key.
		IAMApiKey string `json:"apiKey" yaml:"apiKey"`
	}

	// ResponseWatson is the struct which represents a response given by Watson
	// to a user input.
	// It is useful to parse the string-formatted response to its structured format.
	ResponseWatson struct {
		// StatusCode is the status code of the response.
		StatusCode int `json:"StatusCode"`

		// Result is the result of the response.
		Result *ResultWatson `json:"Result"`
	}

	// ResultWatson is an intermediate struct of the Watson response.
	ResultWatson struct {
		// Output is the output of the response
		Output *OutputWatson `json:"output"`
	}

	// OutputWatson contains the response values and the its intents.
	OutputWatson struct {
		// Generics is a slice containing all response values.
		Generics []*Generic `json:"generic"`

		// Intents is a slice containing all intents values.
		Intents []*Intent `json:"intents"`
	}

	// Generic is a response value.
	Generic struct {
		// ResponseType represents the response type (ex: text)
		ResponseType string `json:"response_type"`
		// Text is the text of the value
		Text string `json:"text"`
	}

	// Intent represents a response intent.
	Intent struct {
		// Intent is the value of the intent.
		Intent string `json:"intent"`

		// Confidence is the confidence with which Watson gives the intention.
		Confidence float32 `json:"confidence"`
	}
)

const (
	label = "watson"
)

// Initialize initializes a new IBM Watson client and returns a new Watson struct.
func (w *Watson) Initialize(config *provider.Config) (provider.Provider, error) {
	service, err := assistantv2.
		NewAssistantV2(&assistantv2.AssistantV2Options{
			URL:       config.URL,
			Version:   config.Version,
			IAMApiKey: config.Token,
		})

	if err != nil {
		return nil, errors.Annotate(err, "initializing a new IBM Watson service")
	}

	client := &Watson{
		service:     service,
		assistantID: config.AssistantID,
		userID:      config.UserID,
	}

	if err := client.CreateSession(config.AssistantID); err != nil {
		return nil, errors.Annotate(err, "initializing a new IBM Watson service")
	}

	return client, nil
}

// CreateSession creates a new client session which would communicate
// with a IBM Watson Assistant.
func (w *Watson) CreateSession(id string) error {
	response, err := w.service.CreateSession(&assistantv2.CreateSessionOptions{
		AssistantID: core.StringPtr(id),
	})

	if err != nil {
		return errors.Annotate(err, "creating a new IBM Watson session")
	}

	// Cast response.Result to the specific dataType
	createSessionResult := w.service.GetCreateSessionResult(response)
	w.sessionID = createSessionResult.SessionID
	return nil
}

// Message sends the user input to the IBM Watson Assistant and return a structured
// result of this text processing.
func (w *Watson) Message(message string) (*provider.Response, error) {
	// Call the assistant Message method
	response, err := w.service.
		Message(&assistantv2.MessageOptions{
			AssistantID: core.StringPtr(w.assistantID),
			SessionID:   w.sessionID,
			Input: &assistantv2.MessageInput{
				Text: core.StringPtr(message),
			},
			Context: &assistantv2.MessageContext{
				Global: &assistantv2.MessageContextGlobal{
					System: &assistantv2.MessageContextGlobalSystem{
						UserID: core.StringPtr(w.userID.String()),
					},
				},
			},
		})

	// Check successful call
	if err != nil {
		return nil, errors.Annotate(err, "sending a message to IBM Watson Assistant")
	}

	return convertResponse(response.String())
}

// GetLabel returns the provider label.
func (w *Watson) GetLabel() string {
	return label
}

// convertResponse converts a response, given as a string, and returns a structured
// response.
func convertResponse(response string) (*provider.Response, error) {
	wResponse := ResponseWatson{}
	if err := json.Unmarshal([]byte(response), &wResponse); err != nil {
		return nil, errors.Annotate(err, "converting watson response")
	}

	outputs := []*provider.Output{}
	intents := []*provider.Intent{}
	for _, generic := range wResponse.Result.Output.Generics {
		// In case of multiline response
		for _, response := range strings.Split(generic.Text, "\n") {
			output := &provider.Output{
				ResponseType: generic.ResponseType,
				Text:         response,
			}

			outputs = append(outputs, output)
		}

	}

	for _, intent := range wResponse.Result.Output.Intents {
		intent := &provider.Intent{
			Intent:     intent.Intent,
			Confidence: intent.Confidence,
		}

		intents = append(intents, intent)
	}

	return &provider.Response{
		StatusCode: wResponse.StatusCode,
		Outputs:    outputs,
		Intents:    intents,
	}, nil
}

// Stop deletes the session which communicates with the IBM Watson Assistant.
func (w *Watson) Stop() error {
	// Call the assistant DeleteSession method
	_, err := w.service.
		DeleteSession(&assistantv2.DeleteSessionOptions{
			AssistantID: core.StringPtr(w.assistantID),
			SessionID:   w.sessionID,
		})

	return err
}
