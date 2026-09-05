package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/slashdevops/mailer"

	"github.com/slashdevops/go-rest-api-service-template/internal/config"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driven/notifieremail"
)

// initMailService initializes the mail service based on configuration
func (a *App) initMailService(ctx context.Context) error {
	var mailerService mailer.MailerService
	var err error

	slog.Info("initializing mail service", "sender_type", a.configs.Mail.MailSender.Value)

	switch a.configs.Mail.MailSender.Value {
	case config.MailSenderSMTP:
		mailerService, err = mailer.NewMailerSMTP(mailer.MailerSMTPConf{
			SMTPHost:   a.configs.Mail.SMTPHost.Value,
			SMTPPort:   a.configs.Mail.SMTPPort.Value,
			Username:   a.configs.Mail.SMTPUsername.Value,
			Password:   a.configs.Mail.SMTPPassword.Value,
			RequireTLS: a.configs.Mail.SMTPRequireTLS.Value,
		})
		if err != nil {
			return fmt.Errorf("error creating SMTP mail service: %w", err)
		}

	case config.MailSenderMailgun:
		// mail.api.url and mail.api.key have been configurable, documented and
		// validated against since the mail config was written -- and until now
		// nothing read either one. This is the sender they always described.
		//
		// The transport lives in the mailer library rather than here because it
		// IS a mail transport, which is what that library is for; a second copy
		// in this repo would be the duplication the config audit exists to
		// remove. Added upstream in slashdevops/mailer v1.1.0.
		mailerService, err = mailer.NewMailerMailgun(mailer.MailerMailgunConf{
			APIURL: a.configs.Mail.APIURL.Value,
			APIKey: a.configs.Mail.APIKey.Value,
			// The service's shared client, so this path inherits the configured
			// timeout, retry policy and connection pool instead of keeping its
			// own that no http.client.* setting can reach.
			HTTPClient: a.httpClient,
		})
		if err != nil {
			return fmt.Errorf("error creating Mailgun mail service: %w", err)
		}

	default:
		return fmt.Errorf("unknown mail sender type: %s", a.configs.Mail.MailSender.Value)
	}

	// Instrument the actual send performed by the mail worker so send
	// count/latency are observable (the enqueue-only path cannot see delivery).
	mailerService = notifieremail.NewInstrumentedMailer(mailerService, a.telemetry.Metrics.Meter)

	a.mailServer, err = mailer.NewMailService(&mailer.MailServiceConfig{
		Ctx:         ctx,
		WorkerCount: a.configs.Mail.MailWorkerCount.Value,
		QueueSize:   a.configs.Mail.MailQueueSize.Value,
		Timeout:     a.configs.Mail.MailWorkerTimeout.Value,
		Mailer:      mailerService,
	})
	if err != nil {
		return fmt.Errorf("error creating mail service: %w", err)
	}

	// Only the SMTP sender has an SMTP posture to report. Logging
	// smtp_require_tls beside a Mailgun sender states a fact about a transport
	// that is not running, which is how an operator ends up believing they
	// checked something they did not.
	fields := []any{
		"sender", a.configs.Mail.MailSender.Value,
		"worker_count", a.configs.Mail.MailWorkerCount.Value,
		"queue_size", a.configs.Mail.MailQueueSize.Value,
	}
	if a.configs.Mail.MailSender.Value == config.MailSenderSMTP {
		fields = append(fields, "smtp_require_tls", a.configs.Mail.SMTPRequireTLS.Value)
	}

	slog.Info("mail service initialized successfully", fields...)
	return nil
}
