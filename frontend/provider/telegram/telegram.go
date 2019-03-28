package telegram

import (
	"time"

	"github.com/fberrez/samantha/capsule"
	"github.com/fberrez/samantha/frontend/provider"
	"github.com/google/uuid"
	"github.com/juju/errors"
	log "github.com/sirupsen/logrus"
	tb "gopkg.in/tucnak/telebot.v2"
)

type (
	// Telegram contains all variables needed to make a stable
	// connection with the Telegram API.
	Telegram struct {
		// Bot is the handler which handles the message sent by users
		Bot *tb.Bot

		// AuthorizedUsers is a authorized users slice.
		AuthorizedUsers []*provider.User

		// pendingMessages is a slice containing received messages that have not
		// been answered.
		pendingMessages []*message

		// userInput is a channel connected to the frontend manager. It is used to
		// send user messages to that manager.
		userInput chan<- *provider.CapsuleProvider
	}

	// message represents user messages.
	message struct {
		// uuid is the message uuid.
		uuid uuid.UUID

		// messageType is the message type.
		contentType provider.ContentType

		// content is the message content.
		content []byte

		// user is the user who sent the message.
		user *tb.User
	}
)

const (
	// pollerTimeout is the poller timeout. When the poller does not receive any
	// message from users, it pauses its listening.
	pollerTimeout = 10 * time.Second

	// label is the provider label.
	label = "telegram"
)

var (
	// logger is a global logger of the package
	logger = log.WithFields(log.Fields{
		"package":  "frontend",
		"provider": label,
	})
)

// Initialize initiliazes a provider with the given label, api token, slice
// of authorized users and user inputs write-only channel.
func (t *Telegram) Initialize(config *provider.Config) (provider.Provider, error) {
	logger.Debugf("Initializing %s", label)

	bot, err := tb.NewBot(tb.Settings{
		Token:  config.Token,
		Poller: &tb.LongPoller{Timeout: pollerTimeout},
	})
	if err != nil {
		return nil, errors.Annotate(err, "initializing telegram")
	}

	return &Telegram{
		Bot:             bot,
		AuthorizedUsers: config.AuthorizedUsers,
		pendingMessages: []*message{},
		userInput:       config.UserInput,
	}, nil
}

// Start starts the provider handlers.
func (t *Telegram) Start() {
	localLogger := log.WithField("ui", label)
	localLogger.Debugf("Starting %s", label)

	// Declares telegram handlers
	t.Bot.Handle(tb.OnText, t.textMessageHandler())
	t.Bot.Handle(tb.OnPhoto, t.photoMessageHandler())
	t.Bot.Handle(tb.OnAudio, t.audioMessageHandler())

	t.Bot.Start()
}

// Message sends the text message to the user.
func (t *Telegram) Message(capsule *capsule.Capsule) error {
	if capsule.Error != nil && len(capsule.Error.Error()) > 0 {
		return t.sendErrorMessage(capsule.OriginalMessage, capsule.Error)
	}

	return t.sendTextMessage(capsule.OriginalMessage, capsule.Responses)
}

// GetLabel returns the label of the provider
func (t *Telegram) GetLabel() string {
	return label
}

// Stop closes the user inputs channel and the telegram listener.
func (t *Telegram) Stop() {
	close(t.userInput)
	t.Bot.Stop()
}

// textMessageHandler handles text messages sent by users.
func (t *Telegram) textMessageHandler() func(*tb.Message) {
	return func(message *tb.Message) {
		localLogger := logger.WithField("action", "receiving user message")

		// Verifies if the user is an authorized user.
		userIsValid := false
		for _, user := range t.AuthorizedUsers {
			if user.Name == message.Sender.Username && user.ID == message.Sender.ID {
				userIsValid = true
				break
			}
		}

		if !userIsValid {
			localLogger.WithFields(log.Fields{
				"from":      message.Sender.Username,
				"sender_id": message.Sender.ID,
				"message":   message.Text,
			}).Debug("User message received from unauthorized user")
			return
		}

		localLogger.WithFields(log.Fields{
			"from":      message.Sender.Username,
			"sender_id": message.Sender.ID,
			"message":   message.Text,
		}).Debug("User message received")

		// Sends the user input to the frontend manager.
		if err := t.processUserMessage(message, provider.Text); err != nil {
			// If an error occured, it generates a system log message and sends it to
			// the user.
			systemlog := provider.SystemLog(err.Error(), provider.ErrorStatus)
			t.Bot.Send(message.Sender, systemlog)
		}
	}
}

// photoMessageHandler handles photo message sent by user.
func (t *Telegram) photoMessageHandler() func(*tb.Message) {
	return func(message *tb.Message) {
		t.Bot.Send(message.Sender, provider.SystemLog("Photo message handling is not implemented", provider.ErrorStatus))
	}
}

// audioMessageHandler handles audio message sent by user.
func (t *Telegram) audioMessageHandler() func(*tb.Message) {
	return func(message *tb.Message) {
		t.Bot.Send(message.Sender, provider.SystemLog("Audio message handling is not implemented", provider.ErrorStatus))
	}
}

// processUserMessage processes a user message by adding it to the pending messages
// slice, converting it to a provider capsule and sending it to the frontend manager.
func (t *Telegram) processUserMessage(userMessage *tb.Message, contentType provider.ContentType) error {
	// Generates a new version 4 UUID.
	uuid, err := uuid.NewRandom()
	if err != nil {
		return errors.Annotate(err, "proccessing user message")
	}

	// Initializes a message.
	message := &message{
		uuid: uuid,
		user: userMessage.Sender,
	}

	// Defines the input type and converts the input content to an array of byte
	switch contentType {
	case provider.Text:
		message.contentType = provider.Text
		message.content = []byte(userMessage.Text)
	case provider.Audio:
		return errors.NotImplementedf("%s message handling", contentType)
	case provider.Image:
		return errors.NotImplementedf("%s message handling", contentType)
	default:
		return errors.NotFoundf("input type %s", contentType)
	}

	// Adds the current message to the slice containing pending messages.
	t.pendingMessages = append(t.pendingMessages, message)
	// Sends the provider capsule-formatted message to the frontend manager.
	t.userInput <- messageToCapsuleProvider(message)
	return nil
}

// messageToCapsuleProvider converts a given message to a provider.CapsuleProvider
func messageToCapsuleProvider(msg *message) *provider.CapsuleProvider {
	return &provider.CapsuleProvider{
		OriginalMessage: msg.uuid,
		ProviderLabel:   label,
		Content:         string(msg.content),
		User:            msg.user.Username,
	}
}

// findPendingMessage returns the pending message corresponding to the given
// uuid.
func (t *Telegram) findPendingMessage(uuid uuid.UUID) (*message, error) {
	if len(t.pendingMessages) == 0 {
		return nil, errors.NotProvisionedf("pending messages")
	}

	for i, m := range t.pendingMessages {
		if m.uuid == uuid {
			// Cut the slice
			t.pendingMessages = append(t.pendingMessages[:i], t.pendingMessages[i+1:]...)
			return m, nil
		}
	}

	return nil, errors.NotFoundf("message (uuid: %s)", uuid)
}

// sendTextMessage responds to a user with a text message.
func (t *Telegram) sendTextMessage(respondTo uuid.UUID, responses []string) error {
	pendingMessage, err := t.findPendingMessage(respondTo)
	if err != nil {
		return err
	}

	for _, response := range responses {
		t.Bot.Send(pendingMessage.user, response)
	}

	return nil
}

// sendErrorMessage responds to a user with a system log message containing the
// error message.
func (t *Telegram) sendErrorMessage(respondTo uuid.UUID, error error) error {
	pendingMessage, err := t.findPendingMessage(respondTo)
	if err != nil {
		return err
	}

	systemLogMessage := provider.SystemLog(error.Error(), provider.ErrorStatus)
	t.Bot.Send(pendingMessage.user, systemLogMessage)
	return nil
}
