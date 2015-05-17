package yetanothersmtpd

import "io"

// SessionHandler is an object providing callbacks for handling a single SMTP
// session. One session can handle multiple messages, each of which will have
// its own MessageHandler.
type SessionHandler interface {
	// Authenticate allows the Handler to authenticate the client using
	// username and password. MTAs that only talk to other SMTP servers do
	// not need to provide a meaningful implementation.
	Authenticate(username, password string) error

	// GetMessageHandler will be called for every new message in a given
	// user session. If the message is to be handled it must return a
	// non-nil MessageHandler and no error.
	GetMessageHandler(from string) (MessageHandler, error)

	// HandleHELO will be called as the client introduces itself to the
	// server. It does not require a meaningful implementation if the server
	// makes no distinction based on the HELO/EHLO name.
	HandleHELO(heloName string, extended bool) error

	// HandleSessionError would be invoked if the code *outside* of the
	// handler errors produces an error. The session itself will terminate
	// but this is a chance to log an error the way.
	HandleSessionError(err error)

	// MaxMessageSize returns the maximum message size allowed in this
	// SMTP session.
	MaxMessageSize() uint64
}

// MessageHandler is an object providing callbacks for handling a single message
// within an SMTP session that can contain multiple messages.
type MessageHandler interface {
	// AddRecipient handles adding a recipient to the message envelope.
	AddRecipient(recipient string) error

	// GetDataWriter yields a data sink to which the message data will be
	// written. Message will be considered received iff all 'Write' calls
	// AND the 'Close' call succeed.
	GetDataWriter() (io.WriteCloser, error)
}
