package config

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestNewMailConfig(t *testing.T) {
	config := NewMailConfig()

	if config.SMTPHost.Value != DefaultMailSMTPHost {
		t.Errorf("Expected SMTPHost to be %s, got %s", DefaultMailSMTPHost, config.SMTPHost.Value)
	}
	if config.SMTPPort.Value != DefaultMailSMTPPort {
		t.Errorf("Expected SMTPPort to be %d, got %d", DefaultMailSMTPPort, config.SMTPPort.Value)
	}
	if config.SMTPUsername.Value != DefaultMailSMTPUsername {
		t.Errorf("Expected SMTPUsername to be %s, got %s", DefaultMailSMTPUsername, config.SMTPUsername.Value)
	}
	if config.SMTPPassword.Value != DefaultMailSMTPPassword {
		t.Errorf("Expected SMTPPassword to be %s, got %s", DefaultMailSMTPPassword, config.SMTPPassword.Value)
	}
	if config.SenderName.Value != DefaultMailSenderName {
		t.Errorf("Expected SenderName to be %s, got %s", DefaultMailSenderName, config.SenderName.Value)
	}
	if config.SenderAddress.Value != DefaultMailSenderAddress {
		t.Errorf("Expected SenderAddress to be %s, got %s", DefaultMailSenderAddress, config.SenderAddress.Value)
	}
	if config.APIURL.Value != DefaultMailAPIURL {
		t.Errorf("Expected APIURL to be %s, got %s", DefaultMailAPIURL, config.APIURL.Value)
	}
	if config.APIKey.Value != DefaultMailAPIKey {
		t.Errorf("Expected APIKey to be %s, got %s", DefaultMailAPIKey, config.APIKey.Value)
	}
	if config.MailSender.Value != DefaultMailSender {
		t.Errorf("Expected MailSender to be %s, got %s", DefaultMailSender, config.MailSender.Value)
	}
	if config.MailWorkerCount.Value != DefaultMailWorkerCount {
		t.Errorf("Expected MailWorkerCount to be %d, got %d", DefaultMailWorkerCount, config.MailWorkerCount.Value)
	}
	if config.MailWorkerTimeout.Value != DefaultMailWorkerTimeout {
		t.Errorf("Expected MailWorkerTimeout to be %v, got %v", DefaultMailWorkerTimeout, config.MailWorkerTimeout.Value)
	}
	if config.SMTPRequireTLS.Value != DefaultMailSMTPRequireTLS {
		t.Errorf("Expected SMTPRequireTLS to be %v, got %v", DefaultMailSMTPRequireTLS, config.SMTPRequireTLS.Value)
	}
	if config.MailQueueSize.Value != DefaultMailQueueSize {
		t.Errorf("Expected MailQueueSize to be %d, got %d", DefaultMailQueueSize, config.MailQueueSize.Value)
	}
}

func TestParseEnvVars_mail(t *testing.T) {
	os.Setenv("MAIL_SMTP_HOST", "smtp.example.com")
	os.Setenv("MAIL_SMTP_PORT", "465")
	os.Setenv("MAIL_SMTP_USERNAME", "testuser")
	os.Setenv("MAIL_SMTP_PASSWORD", "testpass")
	os.Setenv("MAIL_SENDER_NAME", "Test Sender")
	os.Setenv("MAIL_SENDER_ADDRESS", "test@example.com")
	os.Setenv("MAIL_API_URL", "https://api.mailgun.net")
	os.Setenv("MAIL_API_KEY", "test_api_key")
	os.Setenv("MAIL_SENDER", "mailgun")
	os.Setenv("MAIL_WORKER_COUNT", "10")
	os.Setenv("MAIL_WORKER_TIMEOUT", "8s")
	os.Setenv("MAIL_SMTP_REQUIRE_TLS", "false")
	os.Setenv("MAIL_QUEUE_SIZE", "512")

	config := NewMailConfig()
	config.ParseEnvVars()

	if config.SMTPHost.Value != "smtp.example.com" {
		t.Errorf("Expected SMTPHost to be smtp.example.com, got %s", config.SMTPHost.Value)
	}
	if config.SMTPPort.Value != 465 {
		t.Errorf("Expected SMTPPort to be 465, got %d", config.SMTPPort.Value)
	}
	if config.SMTPUsername.Value != "testuser" {
		t.Errorf("Expected SMTPUsername to be testuser, got %s", config.SMTPUsername.Value)
	}
	if config.SMTPPassword.Value != "testpass" {
		t.Errorf("Expected SMTPPassword to be testpass, got %s", config.SMTPPassword.Value)
	}
	if config.SenderName.Value != "Test Sender" {
		t.Errorf("Expected SenderName to be Test Sender, got %s", config.SenderName.Value)
	}
	if config.SenderAddress.Value != "test@example.com" {
		t.Errorf("Expected SenderAddress to be test@example.com, got %s", config.SenderAddress.Value)
	}
	if config.APIURL.Value != "https://api.mailgun.net" {
		t.Errorf("Expected APIURL to be https://api.mailgun.net, got %s", config.APIURL.Value)
	}
	if config.APIKey.Value != "test_api_key" {
		t.Errorf("Expected APIKey to be test_api_key, got %s", config.APIKey.Value)
	}
	if config.MailSender.Value != "mailgun" {
		t.Errorf("Expected MailSender to be mailgun, got %s", config.MailSender.Value)
	}
	if config.MailWorkerCount.Value != 10 {
		t.Errorf("Expected MailWorkerCount to be 10, got %d", config.MailWorkerCount.Value)
	}
	if config.MailWorkerTimeout.Value != 8*time.Second {
		t.Errorf("Expected MailWorkerTimeout to be 8s, got %v", config.MailWorkerTimeout.Value)
	}
	if config.SMTPRequireTLS.Value != false {
		t.Errorf("Expected SMTPRequireTLS to be false, got %v", config.SMTPRequireTLS.Value)
	}
	if config.MailQueueSize.Value != 512 {
		t.Errorf("Expected MailQueueSize to be 512, got %d", config.MailQueueSize.Value)
	}

	// Clean up environment variables
	os.Unsetenv("MAIL_SMTP_REQUIRE_TLS")
	os.Unsetenv("MAIL_QUEUE_SIZE")
	os.Unsetenv("MAIL_SMTP_HOST")
	os.Unsetenv("MAIL_SMTP_PORT")
	os.Unsetenv("MAIL_SMTP_USERNAME")
	os.Unsetenv("MAIL_SMTP_PASSWORD")
	os.Unsetenv("MAIL_SENDER_NAME")
	os.Unsetenv("MAIL_SENDER_ADDRESS")
	os.Unsetenv("MAIL_API_URL")
	os.Unsetenv("MAIL_API_KEY")
	os.Unsetenv("MAIL_SENDER")
	os.Unsetenv("MAIL_WORKER_COUNT")
	os.Unsetenv("MAIL_WORKER_TIMEOUT")
}

func TestValidate_mail(t *testing.T) {
	config := NewMailConfig()

	// Test valid SMTP configuration
	config.SMTPHost.Value = "smtp.example.com"
	config.SMTPUsername.Value = "testuser"
	config.SMTPPassword.Value = "testpass"
	config.MailSender.Value = "smtp"
	// Keep APIURL empty for SMTP config
	config.APIURL.Value = ""

	err := config.Validate()
	if err != nil {
		t.Errorf("Expected no error for valid SMTP config, got %v", err)
	}

	// Test invalid SMTP port
	config.SMTPPort.Value = 999
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "mail.smtp.port" {
		t.Errorf("Expected InvalidConfigurationError with field 'mail.smtp.port', got %v", err)
	}
	config.SMTPPort.Value = DefaultMailSMTPPort

	// Test invalid mail sender
	config.MailSender.Value = "invalid"
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "mail.sender" {
		t.Errorf("Expected InvalidConfigurationError with field 'mail.sender', got %v", err)
	}
	config.MailSender.Value = "smtp"

	// Test missing SMTP username for smtp sender
	config.SMTPUsername.Value = ""
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "mail.smtp.username" {
		t.Errorf("Expected InvalidConfigurationError with field 'mail.smtp.username', got %v", err)
	}
	config.SMTPUsername.Value = "testuser"

	// Test missing SMTP password when username is set
	config.SMTPPassword.Value = ""
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "mail.smtp.password" {
		t.Errorf("Expected InvalidConfigurationError with field 'mail.smtp.password', got %v", err)
	}
	config.SMTPPassword.Value = "testpass"

	// Test valid mailgun configuration
	// Reset config for Mailgun test
	config = NewMailConfig()
	config.MailSender.Value = "mailgun"
	config.APIURL.Value = "https://api.mailgun.net"
	config.APIKey.Value = "test_api_key"
	// Keep SMTPHost empty for Mailgun config
	config.SMTPHost.Value = ""

	err = config.Validate()
	if err != nil {
		t.Errorf("Expected no error for valid mailgun config, got %v", err)
	}

	// Test invalid API URL for mailgun
	config.APIURL.Value = ":/invalid-url"
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "mail.api.url" {
		t.Errorf("Expected InvalidConfigurationError with field 'mail.api.url', got %v", err)
	}
	config.APIURL.Value = "https://api.mailgun.net"

	// Test missing API key for mailgun
	config.APIKey.Value = ""
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "mail.api.key" {
		t.Errorf("Expected InvalidConfigurationError with field 'mail.api.key', got %v", err)
	}
	config.APIKey.Value = "test_api_key"

	// Test empty sender name
	config.SenderName.Value = ""
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "mail.sender.name" {
		t.Errorf("Expected InvalidConfigurationError with field 'mail.sender.name', got %v", err)
	}
	config.SenderName.Value = DefaultMailSenderName

	// Test invalid sender address
	config.SenderAddress.Value = "invalid-email"
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "mail.sender.address" {
		t.Errorf("Expected InvalidConfigurationError with field 'mail.sender.address', got %v", err)
	}
	config.SenderAddress.Value = DefaultMailSenderAddress

	// Test invalid worker count (too low)
	config.MailWorkerCount.Value = 0
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "mail.worker.count" {
		t.Errorf("Expected InvalidConfigurationError with field 'mail.worker.count', got %v", err)
	}
	config.MailWorkerCount.Value = DefaultMailWorkerCount

	// Test invalid worker timeout (too short)
	config.MailWorkerTimeout.Value = 500 * time.Millisecond
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "mail.worker.timeout" {
		t.Errorf("Expected InvalidConfigurationError with field 'mail.worker.timeout', got %v", err)
	}
	config.MailWorkerTimeout.Value = DefaultMailWorkerTimeout

	// Test invalid queue size (too low)
	config.MailQueueSize.Value = 0
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "mail.queue.size" {
		t.Errorf("Expected InvalidConfigurationError with field 'mail.queue.size', got %v", err)
	}
	config.MailQueueSize.Value = DefaultMailQueueSize

	// mail.sender decides which transport is used, so BOTH families being
	// configured is not an error -- an operator trying Mailgun should not have
	// to delete their SMTP settings to do it.
	//
	// This replaces a mutual-exclusion rule that existed to compensate for the
	// validation not keying on mail.sender at all. That rule came paired with an
	// OR ("either SMTP host or API URL must be set") which accepted a
	// configuration the service could not run: setting only mail.api.url passed,
	// and startup then failed with "SMTPHost must be between 1 and 255
	// characters" -- an error naming a setting the operator deliberately did not
	// set.
	config.SMTPHost.Value = "smtp.example.com"
	config.APIURL.Value = "https://api.mailgun.net/v3/d/messages"
	config.APIKey.Value = "key"
	config.MailSender.Value = MailSenderSMTP
	config.SMTPUsername.Value = "user"
	config.SMTPPassword.Value = "pass"

	if err = config.Validate(); err != nil {
		t.Errorf("both families configured must be accepted; mail.sender chooses. got %v", err)
	}
}

// Each sender requires its OWN settings, and the error names the setting the
// operator actually has to fix.
func TestValidate_mailRequiresWhatTheSenderNeeds(t *testing.T) {
	for name, tc := range map[string]struct {
		sender    string
		smtpHost  string
		apiURL    string
		apiKey    string
		wantField string
	}{
		"smtp without a host":      {MailSenderSMTP, "", "", "", "mail.smtp.host"},
		"mailgun without a url":    {MailSenderMailgun, "", "", "key", "mail.api.url"},
		"mailgun without a key":    {MailSenderMailgun, "", "https://api.mailgun.net/v3/d/messages", "", "mail.api.key"},
		"mailgun ignores smtp":     {MailSenderMailgun, "", "", "", "mail.api.url"},
		"smtp ignores the api url": {MailSenderSMTP, "", "https://api.mailgun.net/v3/d/messages", "key", "mail.smtp.host"},
	} {
		t.Run(name, func(t *testing.T) {
			c := NewMailConfig()
			c.MailSender.Value = tc.sender
			c.SMTPHost.Value = tc.smtpHost
			c.APIURL.Value = tc.apiURL
			c.APIKey.Value = tc.apiKey

			err := c.Validate()

			invalidErr, ok := errors.AsType[*InvalidConfigurationError](err)
			if err == nil || !ok || invalidErr.Field != tc.wantField {
				t.Fatalf("got %v, want an InvalidConfigurationError naming %q", err, tc.wantField)
			}
		})
	}
}

// The happy paths, so the cases above cannot pass by refusing everything.
func TestValidate_mailAcceptsEachSenderProperlyConfigured(t *testing.T) {
	smtp := NewMailConfig()
	smtp.MailSender.Value = MailSenderSMTP
	smtp.SMTPHost.Value = "smtp.example.com"
	smtp.SMTPUsername.Value = "user"
	smtp.SMTPPassword.Value = "pass"

	if err := smtp.Validate(); err != nil {
		t.Errorf("a configured smtp sender was refused: %v", err)
	}

	mg := NewMailConfig()
	mg.MailSender.Value = MailSenderMailgun
	mg.APIURL.Value = "https://api.mailgun.net/v3/mg.example.com/messages"
	mg.APIKey.Value = "key-secret"

	if err := mg.Validate(); err != nil {
		t.Errorf("a configured mailgun sender was refused: %v", err)
	}
}
