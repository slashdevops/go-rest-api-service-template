package config

import (
	"fmt"
	"net/mail"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	ValidMailSMTPPorts = "25|465|587|1025|2525"
	// MailSenderSMTP and MailSenderMailgun are the transports mail.sender
	// selects between. Named constants rather than bare strings because the
	// value is compared in two packages -- here and in the composition root --
	// and a typo in either is a startup failure rather than a compile error.
	MailSenderSMTP    = "smtp"
	MailSenderMailgun = "mailgun"

	ValidMailSender           = MailSenderSMTP + "|" + MailSenderMailgun
	ValidMailMaxWorkerCount   = 50
	ValidMailMinWorkerCount   = 1
	ValidMailMaxWorkerTimeout = 10 * time.Second
	ValidMailMinWorkerTimeout = 1 * time.Second
	ValidMailMaxQueueSize     = 10000
	ValidMailMinQueueSize     = 1

	DefaultMailSMTPHost     = ""
	DefaultMailSMTPUsername = ""
	DefaultMailSMTPPassword = ""
	DefaultMailSMTPPort     = 587
	// DefaultMailSMTPRequireTLS is secure-by-default: delivery fails rather than
	// falling back to plaintext when TLS cannot be negotiated. Dev/CI targets
	// that use a plaintext relay (e.g. mailpit on :1025) opt out explicitly.
	DefaultMailSMTPRequireTLS = true
	DefaultMailSenderName     = "goapitemplate me"
	DefaultMailSenderAddress  = "no-reply@goapitemplate.local"
	DefaultMailAPIURL         = ""
	DefaultMailAPIKey         = ""
	DefaultMailWorkerCount    = 5
	DefaultMailWorkerTimeout  = 5 * time.Second
	// DefaultMailQueueSize buffers bursts so Enqueue (called synchronously from
	// HTTP handlers) does not block once all workers are busy.
	DefaultMailQueueSize = 256
	DefaultMailSender    = "smtp"
)

type MailConfig struct {
	// When smtp is used
	SMTPHost       Field[string]
	SMTPPort       Field[int]
	SMTPUsername   Field[string]
	SMTPPassword   Field[string]
	SMTPRequireTLS Field[bool]

	SenderName    Field[string]
	SenderAddress Field[string]

	// when mailgun or other service is used
	APIURL Field[string]
	APIKey Field[string]

	MailSender        Field[string]
	MailWorkerCount   Field[int]
	MailWorkerTimeout Field[time.Duration]
	MailQueueSize     Field[int]
}

func NewMailConfig() *MailConfig {
	return &MailConfig{
		SMTPHost:          NewField("mail.smtp.host", "MAIL_SMTP_HOST", "SMTP Host", DefaultMailSMTPHost),
		SMTPPort:          NewField("mail.smtp.port", "MAIL_SMTP_PORT", "SMTP Port", DefaultMailSMTPPort),
		SMTPUsername:      NewField("mail.smtp.username", "MAIL_SMTP_USERNAME", "SMTP Username", DefaultMailSMTPUsername),
		SMTPPassword:      NewField("mail.smtp.password", "MAIL_SMTP_PASSWORD", "SMTP Password", DefaultMailSMTPPassword),
		SMTPRequireTLS:    NewField("mail.smtp.require.tls", "MAIL_SMTP_REQUIRE_TLS", "Require TLS for SMTP delivery (fail instead of sending in plaintext)", DefaultMailSMTPRequireTLS),
		SenderName:        NewField("mail.sender.name", "MAIL_SENDER_NAME", "Sender Name", DefaultMailSenderName),
		SenderAddress:     NewField("mail.sender.address", "MAIL_SENDER_ADDRESS", "Sender Address", DefaultMailSenderAddress),
		APIURL:            NewField("mail.api.url", "MAIL_API_URL", "Mail API URL", DefaultMailAPIURL),
		APIKey:            NewField("mail.api.key", "MAIL_API_KEY", "Mail API Key", DefaultMailAPIKey),
		MailSender:        NewField("mail.sender", "MAIL_SENDER", "Mail Sender", DefaultMailSender),
		MailWorkerCount:   NewField("mail.worker.count", "MAIL_WORKER_COUNT", "Mail Worker Count", DefaultMailWorkerCount),
		MailWorkerTimeout: NewField("mail.worker.timeout", "MAIL_WORKER_TIMEOUT", "Mail Worker Timeout", DefaultMailWorkerTimeout),
		MailQueueSize:     NewField("mail.queue.size", "MAIL_QUEUE_SIZE", "Mail Queue Size (buffered messages before Enqueue blocks)", DefaultMailQueueSize),
	}
}

func (ref *MailConfig) ParseEnvVars() {
	ref.SMTPHost.Value = GetEnv(ref.SMTPHost.EnVarName, ref.SMTPHost.Value)
	ref.SMTPPort.Value = GetEnv(ref.SMTPPort.EnVarName, ref.SMTPPort.Value)
	ref.SMTPUsername.Value = GetEnv(ref.SMTPUsername.EnVarName, ref.SMTPUsername.Value)
	ref.SMTPPassword.Value = GetEnv(ref.SMTPPassword.EnVarName, ref.SMTPPassword.Value)
	ref.SMTPRequireTLS.Value = GetEnv(ref.SMTPRequireTLS.EnVarName, ref.SMTPRequireTLS.Value)
	ref.SenderName.Value = GetEnv(ref.SenderName.EnVarName, ref.SenderName.Value)
	ref.SenderAddress.Value = GetEnv(ref.SenderAddress.EnVarName, ref.SenderAddress.Value)
	ref.APIURL.Value = GetEnv(ref.APIURL.EnVarName, ref.APIURL.Value)
	ref.APIKey.Value = GetEnv(ref.APIKey.EnVarName, ref.APIKey.Value)
	ref.MailSender.Value = GetEnv(ref.MailSender.EnVarName, ref.MailSender.Value)
	ref.MailWorkerCount.Value = GetEnv(ref.MailWorkerCount.EnVarName, ref.MailWorkerCount.Value)
	ref.MailWorkerTimeout.Value = GetEnv(ref.MailWorkerTimeout.EnVarName, ref.MailWorkerTimeout.Value)
	ref.MailQueueSize.Value = GetEnv(ref.MailQueueSize.EnVarName, ref.MailQueueSize.Value)
}

func (ref *MailConfig) Validate() error {
	// Validation keys on mail.sender, because mail.sender is what actually
	// selects the transport.
	//
	// It used to be an OR across the two families -- "either SMTP host or API
	// URL must be set" -- which accepted a configuration the service could not
	// run. Setting only mail.api.url satisfied it, and startup then failed with
	// "SMTPHost must be between 1 and 255 characters": an error naming a setting
	// the operator deliberately did not set, from a code path they did not
	// choose. Checking what the sender needs is what makes the message match the
	// mistake.
	switch ref.MailSender.Value {
	case MailSenderSMTP:
		if ref.SMTPHost.Value == "" {
			return &InvalidConfigurationError{
				Field:   "mail.smtp.host",
				Value:   "",
				Message: "mail.smtp.host is required when mail.sender is " + MailSenderSMTP,
			}
		}

	case MailSenderMailgun:
		if ref.APIURL.Value == "" {
			return &InvalidConfigurationError{
				Field:   "mail.api.url",
				Value:   "",
				Message: "mail.api.url is required when mail.sender is " + MailSenderMailgun,
			}
		}

		if ref.APIKey.Value == "" {
			return &InvalidConfigurationError{
				Field:   "mail.api.key",
				Value:   "",
				Message: "mail.api.key is required when mail.sender is " + MailSenderMailgun,
			}
		}
	}

	if ref.SMTPHost.Value != "" && !slices.Contains(strings.Split(ValidMailSMTPPorts, "|"), strconv.Itoa(ref.SMTPPort.Value)) {
		return &InvalidConfigurationError{
			Field:   "mail.smtp.port",
			Value:   ref.SMTPPort.Value,
			Message: "Invalid SMTP port. Must be one of [" + ValidMailSMTPPorts + "]",
		}
	}

	if ref.MailSender.Value != "" && !slices.Contains(strings.Split(ValidMailSender, "|"), ref.MailSender.Value) {
		return &InvalidConfigurationError{
			Field:   "mail.sender",
			Value:   ref.MailSender.Value,
			Message: "Invalid mail sender. Must be one of [" + ValidMailSender + "]",
		}
	}

	if ref.MailSender.Value == "smtp" {
		if ref.SMTPHost.Value != "" && ref.SMTPUsername.Value == "" {
			return &InvalidConfigurationError{
				Field:   "mail.smtp.username",
				Value:   ref.SMTPUsername.Value,
				Message: "SMTP username must be set",
			}
		}

		if ref.SMTPUsername.Value != "" && ref.SMTPPassword.Value == "" {
			return &InvalidConfigurationError{
				Field:   "mail.smtp.password",
				Value:   ref.SMTPPassword.Value,
				Message: "SMTP password must be set",
			}
		}
	}

	if ref.MailSender.Value == "mailgun" {
		if ref.APIURL.Value != "" {
			if _, err := url.Parse(ref.APIURL.Value); err != nil {
				return &InvalidConfigurationError{
					Field:   "mail.api.url",
					Value:   ref.APIURL.Value,
					Message: "Invalid Mail API URL",
				}
			}

			if ref.APIKey.Value == "" {
				return &InvalidConfigurationError{
					Field:   "mail.api.key",
					Value:   ref.APIKey.Value,
					Message: "Mail API Key must be set",
				}
			}
		}
	}

	if ref.SenderName.Value == "" {
		return &InvalidConfigurationError{
			Field:   "mail.sender.name",
			Value:   ref.SenderName.Value,
			Message: "Mail sender name must be set",
		}
	}

	if _, err := mail.ParseAddress(ref.SenderAddress.Value); err != nil {
		return &InvalidConfigurationError{
			Field:   "mail.sender.address",
			Value:   ref.SenderAddress.Value,
			Message: "Invalid mail sender address",
		}
	}
	if ref.MailWorkerCount.Value < ValidMailMinWorkerCount || ref.MailWorkerCount.Value > ValidMailMaxWorkerCount {
		return &InvalidConfigurationError{
			Field:   "mail.worker.count",
			Value:   ref.MailWorkerCount.Value,
			Message: fmt.Sprintf("Invalid mail worker count. Must be between %d and %d", ValidMailMinWorkerCount, ValidMailMaxWorkerCount),
		}
	}

	if ref.MailWorkerTimeout.Value < ValidMailMinWorkerTimeout || ref.MailWorkerTimeout.Value > ValidMailMaxWorkerTimeout {
		return &InvalidConfigurationError{
			Field:   "mail.worker.timeout",
			Value:   ref.MailWorkerTimeout.Value,
			Message: fmt.Sprintf("Invalid mail worker timeout. Must be between %.0f and %.0f seconds", ValidMailMinWorkerTimeout.Seconds(), ValidMailMaxWorkerTimeout.Seconds()),
		}
	}

	if ref.MailQueueSize.Value < ValidMailMinQueueSize || ref.MailQueueSize.Value > ValidMailMaxQueueSize {
		return &InvalidConfigurationError{
			Field:   "mail.queue.size",
			Value:   ref.MailQueueSize.Value,
			Message: fmt.Sprintf("Invalid mail queue size. Must be between %d and %d", ValidMailMinQueueSize, ValidMailMaxQueueSize),
		}
	}

	return nil
}
