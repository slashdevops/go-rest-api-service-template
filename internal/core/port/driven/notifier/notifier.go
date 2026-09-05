// Package notifier defines the driven port that use-cases call when
// they need to notify a user out-of-band (email today, possibly SMS or
// push later). The implementation lives in
// internal/adapter/driven/notifieremail which combines the existing
// internal/templates renderer with the github.com/slashdevops/mailer queue.
//
// The port is intentionally task-shaped (SendAccountVerification,
// SendPasswordReset) rather than a generic "send arbitrary email" call:
// the use-case decides *what* to notify about, the adapter decides
// *how* (template, transport, sender identity).
package notifier

import "context"

//go:generate go tool mockgen -package=mocks -destination=../../../../../mocks/service/notifier.go -source=notifier.go Notifier

// Recipient is the human target of a notification.
type Recipient struct {
	Name  string
	Email string
}

// Notifier is the driven port consumed by use-cases.
type Notifier interface {
	// SendAccountVerification delivers the email that lets a newly
	// registered (or re-verifying) user confirm ownership of their
	// address. token is the opaque verification token; ttlHuman is a
	// pre-formatted lifetime string for display.
	SendAccountVerification(ctx context.Context, to Recipient, token, ttlHuman string) error

	// SendPasswordReset delivers the email that lets a user complete a
	// password recovery flow. Same token/ttl conventions as above.
	SendPasswordReset(ctx context.Context, to Recipient, token, ttlHuman string) error

	// SendAccountExists tells the owner of an address that someone tried to
	// register with it and an account already exists.
	//
	// Registration answers the same way whether or not the address is taken, so
	// this is the only way the real owner learns it happened — and the only way
	// somebody who has forgotten they already have an account finds out, rather
	// than being told "registered" and then never receiving anything.
	//
	// It carries no token. Anyone can cause it to be sent to any address, so it
	// must not be something a recipient can be walked through.
	SendAccountExists(ctx context.Context, to Recipient) error
}
