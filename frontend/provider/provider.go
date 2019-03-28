package provider

import (
	"fmt"

	"github.com/fberrez/samantha/capsule"
	"github.com/google/uuid"
)

type (
	// Provider is the interface of a frontend provider.
	Provider interface {
		// Initialize initiliazes a provider with the given label, api token, slice
		// of authorized users and user inputs write-only channel.
		Initialize(*Config) (Provider, error)

		// Start starts the provider handlers.
		Start()

		// Message sends the text message to the user.
		Message(capsule *capsule.Capsule) error

		// GetLabel returns the label of the provider
		GetLabel() string

		// Stop closes the provider listener.
		Stop()
	}

	// Config is a structured configuration for provider
	Config struct {
		// Token is the API provider token
		Token string

		// AutorizedUsers is a slice containing all authorized users.
		// These users are authorized to use the frontend provider.
		AuthorizedUsers []*User

		// UserInput is a only-write channel which is used to send local capsules to
		// the frontend manager.
		UserInput chan<- *CapsuleProvider
	}

	// CapsuleProvider is the capsule which user to transfer data between
	// frontend package and providers.
	CapsuleProvider struct {
		// OriginalMessage is the original message UUID corresponding to this capsule.
		OriginalMessage uuid.UUID `json:"originalMessage" yaml:"originalMessage"`

		// ProviderLabel is the provider label (ex: telegram).
		ProviderLabel string `json:"providerLabel" yaml:"providerLabel"`

		// Content is a string representing the user input.
		Content string `json:"content" yaml:"content"`

		// User is the name of the user
		User string `json:"user" yaml:"user"`
	}

	// User represents a user of the provider.
	User struct {
		// ID is the user ID.
		ID int `json:"id" yaml:"id"`

		// Name is the user name.
		Name string `json:"name" yaml:"name"`
	}

	// ContentType is used to classify a user input which can has a specific type
	// such as text, image...
	ContentType string

	// SystemLogStatus is a predefined status for system loggin.
	SystemLogStatus string
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

	// ErrorStatus is the system log status when we want to send an error message to the user.
	ErrorStatus SystemLogStatus = "Error"

	// Info is the system log status when we want to send an info message to the user.
	Info SystemLogStatus = "Info"

	// Delimiter is used to separate responses and display it as a multibubble message.
	Delimiter string = "|"
)

// SystemLog returns a new formatted string which would correspond to a system
// message.
func SystemLog(content string, status SystemLogStatus) string {
	return fmt.Sprintf("[SYSTEM]%s: %s", status, content)
}
