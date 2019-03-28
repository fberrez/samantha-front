package capsule

import (
	"github.com/google/uuid"
)

type (
	// Capsule is an object which contains all informations about a user input,
	// the action done in relation with the user input and the response that will
	// send to the user.
	// A capsule is initialized on the frontend side before to be sent to the backend
	// and the core.
	Capsule struct {
		OriginalMessage  uuid.UUID `json:"from" yaml:"from"`
		FrontendProvider string    `json:"frontendProvider" yaml:"frontendProvider"`
		Content          string    `json:"content" yaml:"content"`
		User             string    `json:"user" yaml:"user"`
		Responses        []string  `json:"responses" yaml:"responses"`
		Error            error     `json:"error" yaml:"error"`
	}
)
