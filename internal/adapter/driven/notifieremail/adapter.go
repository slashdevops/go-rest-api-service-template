// Package notifieremail is the driven adapter that satisfies the
// notifier.Notifier port by combining the existing internal/templates
// renderer with the github.com/slashdevops/mailer queue. It owns the
// "how" of an email: subject lines, sender identity, MIME type, and
// the API endpoints that the templates link back to.
//
// Use-cases under internal/core stay free of mailer + templates
// imports and only depend on notifier.Notifier.
package notifieremail

import (
	"context"
	"net/url"
	"time"

	"github.com/slashdevops/mailer"
	"go.opentelemetry.io/otel/metric"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driven/notifieremail/templates"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/notifier"
)

// Config wires the adapter to its infrastructure dependencies.
type Config struct {
	Queue                       mailer.MailQueueService
	Meter                       metric.Meter // optional; nil disables metrics
	SenderName                  string
	SenderEmail                 string
	UserVerificationWebEndpoint string
	UserResetPasswordEndpoint   string
}

// Adapter implements notifier.Notifier on top of mailer + templates.
type Adapter struct {
	queue                       mailer.MailQueueService
	metrics                     *enqueueMetrics
	senderName                  string
	senderEmail                 string
	userVerificationWebEndpoint string
	userResetPasswordEndpoint   string
}

// New constructs an Adapter from a Config.
func New(cfg Config) *Adapter {
	mx, _ := newEnqueueMetrics(cfg.Meter)
	return &Adapter{
		queue:                       cfg.Queue,
		metrics:                     mx,
		senderName:                  cfg.SenderName,
		senderEmail:                 cfg.SenderEmail,
		userVerificationWebEndpoint: cfg.UserVerificationWebEndpoint,
		userResetPasswordEndpoint:   cfg.UserResetPasswordEndpoint,
	}
}

// SendAccountVerification implements notifier.Notifier.
func (a *Adapter) SendAccountVerification(ctx context.Context, to notifier.Recipient, token, ttlHuman string) error {
	emailContent, err := templates.NewEmailAccountVerification(&templates.EmailAccountVerificationConf{
		VerificationWebEndpoint: a.userVerificationWebEndpoint,
		VerificationToken:       token,
		VerificationTTL:         ttlHuman,
		UserName:                to.Name,
		HTML:                    true,
	})
	if err != nil {
		return err
	}

	return a.enqueue(ctx, to, templateAccountVerification, "Account Verification", emailContent.Render())
}

// SendPasswordReset implements notifier.Notifier.
func (a *Adapter) SendPasswordReset(ctx context.Context, to notifier.Recipient, token, ttlHuman string) error {
	emailContent, err := templates.NewEmailResetPassword(&templates.EmailResetPasswordConf{
		ResetPasswordAPIEndpoint: a.userResetPasswordEndpoint,
		ResetPasswordToken:       token,
		ResetPasswordTTL:         ttlHuman,
		UserName:                 to.Name,
		HTML:                     true,
	})
	if err != nil {
		return err
	}

	return a.enqueue(ctx, to, templatePasswordReset, "Reset Password", emailContent.Render())
}

// SendAccountExists implements notifier.Notifier.
//
// The sign-in URL is the password-reset page's origin: both are the frontend,
// and this mail points at the place a person signs in rather than carrying
// anything that acts on the account.
func (a *Adapter) SendAccountExists(ctx context.Context, to notifier.Recipient) error {
	emailContent, err := templates.NewEmailAccountExists(&templates.EmailAccountExistsConf{
		SignInURL: a.signInURL(),
		UserName:  to.Name,
		HTML:      true,
	})
	if err != nil {
		return err
	}

	return a.enqueue(ctx, to, templateAccountExists, "You already have an account", emailContent.Render())
}

// signInURL derives where a person signs in from the password-reset page, which
// is the one frontend URL this adapter already holds. Deriving it beats adding
// a setting that would have to be kept in step with that one.
func (a *Adapter) signInURL() string {
	if parsed, err := url.Parse(a.userResetPasswordEndpoint); err == nil && parsed.Host != "" {
		parsed.Path = "/"
		parsed.RawQuery = ""

		return parsed.String()
	}

	return a.userResetPasswordEndpoint
}

func (a *Adapter) enqueue(ctx context.Context, to notifier.Recipient, template, subject, body string) error {
	start := time.Now()

	mailContent, err := mailer.NewMailContentBuilder().
		WithFromName(a.senderName).
		WithFromAddress(a.senderEmail).
		WithToName(to.Name).
		WithToAddress(to.Email).
		WithMimeType(mailer.MimeTypeTextHTML).
		WithSubject(subject).
		WithBody(body).
		Build()
	if err != nil {
		a.metrics.record(ctx, start, template, resultError)
		return err
	}

	err = a.queue.Enqueue(mailContent)

	result := resultOK
	if err != nil {
		result = resultError
	}
	a.metrics.record(ctx, start, template, result)

	return err
}
