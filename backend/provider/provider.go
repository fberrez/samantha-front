package provider

import (
	"fmt"

	"github.com/google/uuid"
)

type (
	// Provider is the interface of a backend provider.
	Provider interface {
		// Initialize initiliazes a provider with the given label, api token, slice
		// of authorized users and user inputs write-only channel.
		Initialize(config *Config) (Provider, error)

		// Message sends a text message to the API provider and returns a structured
		// result.
		Message(text string) (*Response, error)

		// GetLabel returns the label of the provider
		GetLabel() string

		// Stop closes the provider listener.
		Stop() error
	}

	// Config is a structured provider configuration.
	Config struct {
		// userID is the unique identifier of the current session.
		UserID uuid.UUID `json:"userID" yaml:"userID"`

		// Label is the Label of the provider.
		Label string `json:"label" yaml:"label"`

		// URL is the provider API URL.
		URL string `json:"url" yaml:"url"`

		// Version is the provider API version.
		Version string `json:"version" yaml:"version"`

		// Token is the provider API key.
		Token string `json:"token" yaml:"token"`

		// AssistantID is the provider Assistant ID.
		AssistantID string `json:"assistantID" yaml:"assistantID"`
	}

	// Response is a structured format of a response returned by a provider.
	Response struct {
		// StatusCode is the HTTP status code of the response
		StatusCode int `json:"statusCode" yaml:"statusCode"`

		// Outputs is a slice containing all outputs (responses content).
		Outputs []*Output `json:"output" yaml:"output"`

		// Intents is a slice containing all intents.
		Intents []*Intent `json:"intents" yaml:"intents"`
	}

	// Output represents a response output.
	Output struct {
		// ResponseType is the type of the response.
		ResponseType string `json:"responseType"`

		// Text is the text of the response.
		Text string `json:"text"`
	}

	// Intent represents a response intent.
	Intent struct {
		// Intent is the name of the intent.
		Intent string `json:"intent"`

		// Confidence is the confidence of the intent.
		Confidence float32 `json:"confidence"`
	}

	// ContentType is used to classify a user input which can has a specific type
	// such as text, image...
	ContentType string
)

const (
	// Text is the input type when the input is text-formatted.
	Text ContentType = "Text"

	// Image is the input type when the input is an image.
	Image ContentType = "Image"

	// Audio is the input type when the input is an audio file.
	Audio ContentType = "Audio"

	// ErrorType is the input type when the input is an error.
	ErrorType ContentType = "Error"
)

// String returns a string-formatted response.
func (r *Response) String() string {
	return fmt.Sprintf("StatusCode: %d Outputs: %v Intents: %v", r.StatusCode, r.Outputs, r.Intents)
}

// String returns a string-formatted output.
func (o *Output) String() string {
	return fmt.Sprintf("ResponseType: %s Text: %s", o.ResponseType, o.Text)
}

// String returns a string-formatted intent.
func (i *Intent) String() string {
	return fmt.Sprintf("Intent: %s Confidence: %f", i.Intent, i.Confidence)
}
