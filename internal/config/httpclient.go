package config

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	ValidHTTPClientMaxIdleConns             = 200
	ValidHTTPClientMinIdleConns             = 1
	ValidHTTPClientMaxIdleConnsPerHost      = 200
	ValidHTTPClientMinIdleConnsPerHost      = 1
	ValidHTTPClientMaxIdleConnTimeout       = 120 * time.Second
	ValidHTTPClientMinIdleConnTimeout       = 1 * time.Second
	ValidHTTPClientMaxTLSHandshakeTimeout   = 15 * time.Second
	ValidHTTPClientMinTLSHandshakeTimeout   = 1 * time.Second
	ValidHTTPClientMaxExpectContinueTimeout = 5 * time.Second
	ValidHTTPClientMinExpectContinueTimeout = 1 * time.Second
	ValidHTTPClientMaxTimeout               = 600 * time.Second
	ValidHTTPClientMinTimeout               = 1 * time.Second
	ValidHTTPClientMaxMaxRetries            = 15
	ValidHTTPClientMinMaxRetries            = 1
	ValidHTTPClientRetryStrategies          = "exponential|fixed|jitter"

	DefaultHTTPClientMaxIdleConns          = 100
	DefaultHTTPClientMaxIdleConnsPerHost   = 100
	DefaultHTTPClientIdleConnTimeout       = 90 * time.Second
	DefaultHTTPClientTLSHandshakeTimeout   = 10 * time.Second
	DefaultHTTPClientExpectContinueTimeout = 1 * time.Second
	DefaultHTTPClientDisableKeepAlives     = false

	// DefaultHTTPClientAllowPrivateAddresses is false: an outbound request to
	// a loopback or private address is refused. The URLs this client dials
	// are supplied by operators (LLM engines, identity providers), so without
	// the guard a grant on "create engine" reaches inside the perimeter. The
	// dev stack and an on-premises deployment set it to true; link-local
	// (cloud metadata) is refused whatever this says.
	DefaultHTTPClientAllowPrivateAddresses = false
	DefaultHTTPClientTimeout               = 120 * time.Second
	DefaultHTTPClientMaxRetries            = 10
	DefaultHTTPClientRetryStrategy         = "jitter"
)

type HTTPClientConfig struct {
	RetryStrategy         Field[string]
	MaxIdleConns          Field[int]
	MaxIdleConnsPerHost   Field[int]
	IdleConnTimeout       Field[time.Duration]
	TLSHandshakeTimeout   Field[time.Duration]
	ExpectContinueTimeout Field[time.Duration]
	DisableKeepAlives     Field[bool]
	AllowPrivateAddresses Field[bool]
	Timeout               Field[time.Duration]
	MaxRetries            Field[int]
}

func NewHTTPClientConfig() *HTTPClientConfig {
	return &HTTPClientConfig{
		MaxIdleConns:          NewField("http.client.max.idle.conns", "HTTP_CLIENT_MAX_IDLE_CONNS", "Maximum number of idle connections", DefaultHTTPClientMaxIdleConns),
		MaxIdleConnsPerHost:   NewField("http.client.max.idle.conns.per.host", "HTTP_CLIENT_MAX_IDLE_CONNS_PER_HOST", "Maximum number of idle connections per host", DefaultHTTPClientMaxIdleConnsPerHost),
		IdleConnTimeout:       NewField("http.client.idle.conn.timeout", "HTTP_CLIENT_IDLE_CONN_TIMEOUT", "Idle connection timeout", DefaultHTTPClientIdleConnTimeout),
		TLSHandshakeTimeout:   NewField("http.client.tls.handshake.timeout", "HTTP_CLIENT_TLS_HANDSHAKE_TIMEOUT", "TLS handshake timeout", DefaultHTTPClientTLSHandshakeTimeout),
		ExpectContinueTimeout: NewField("http.client.expect.continue.timeout", "HTTP_CLIENT_EXPECT_CONTINUE_TIMEOUT", "Expect continue timeout", DefaultHTTPClientExpectContinueTimeout),
		AllowPrivateAddresses: NewField("http.client.allow.private.addresses", "HTTP_CLIENT_ALLOW_PRIVATE_ADDRESSES", "Let outbound requests (LLM engines, identity providers) reach loopback and private addresses. Off by default; link-local is refused regardless", DefaultHTTPClientAllowPrivateAddresses),
		DisableKeepAlives:     NewField("http.client.disable.keep.alives", "HTTP_CLIENT_DISABLE_KEEP_ALIVES", "Disable keep-alives", DefaultHTTPClientDisableKeepAlives),
		Timeout:               NewField("http.client.timeout", "HTTP_CLIENT_TIMEOUT", "Timeout for HTTP requests", DefaultHTTPClientTimeout),
		MaxRetries:            NewField("http.client.max.retries", "HTTP_CLIENT_MAX_RETRIES", "Maximum number of retries for HTTP requests", DefaultHTTPClientMaxRetries),
		RetryStrategy:         NewField("http.client.retry.strategy", "HTTP_CLIENT_RETRY_STRATEGY", "Retry strategy for HTTP request. Valid values are: ["+ValidHTTPClientRetryStrategies+"]", DefaultHTTPClientRetryStrategy),
	}
}

func (c *HTTPClientConfig) ParseEnvVars() {
	c.MaxIdleConns.Value = GetEnv(c.MaxIdleConns.EnVarName, c.MaxIdleConns.Value)
	c.MaxIdleConnsPerHost.Value = GetEnv(c.MaxIdleConnsPerHost.EnVarName, c.MaxIdleConnsPerHost.Value)
	c.IdleConnTimeout.Value = GetEnv(c.IdleConnTimeout.EnVarName, c.IdleConnTimeout.Value)
	c.TLSHandshakeTimeout.Value = GetEnv(c.TLSHandshakeTimeout.EnVarName, c.TLSHandshakeTimeout.Value)
	c.ExpectContinueTimeout.Value = GetEnv(c.ExpectContinueTimeout.EnVarName, c.ExpectContinueTimeout.Value)
	c.DisableKeepAlives.Value = GetEnv(c.DisableKeepAlives.EnVarName, c.DisableKeepAlives.Value)
	c.AllowPrivateAddresses.Value = GetEnv(c.AllowPrivateAddresses.EnVarName, c.AllowPrivateAddresses.Value)
	c.Timeout.Value = GetEnv(c.Timeout.EnVarName, c.Timeout.Value)
	c.MaxRetries.Value = GetEnv(c.MaxRetries.EnVarName, c.MaxRetries.Value)
	c.RetryStrategy.Value = GetEnv(c.RetryStrategy.EnVarName, c.RetryStrategy.Value)
}

func (c *HTTPClientConfig) Validate() error {
	if c.MaxIdleConns.Value < ValidHTTPClientMinIdleConns || c.MaxIdleConns.Value > ValidHTTPClientMaxIdleConns {
		return &InvalidConfigurationError{
			Field:   "http.client.max.idle.conns",
			Value:   fmt.Sprintf("%d", c.MaxIdleConns.Value),
			Message: fmt.Sprintf("invalid http.client.max.idle.conns, must be between %d and %d", ValidHTTPClientMinIdleConns, ValidHTTPClientMaxIdleConns),
		}
	}

	if c.MaxIdleConnsPerHost.Value < ValidHTTPClientMinIdleConnsPerHost || c.MaxIdleConnsPerHost.Value > ValidHTTPClientMaxIdleConnsPerHost {
		return &InvalidConfigurationError{
			Field:   "http.client.max.idle.conns.per.host",
			Value:   fmt.Sprintf("%d", c.MaxIdleConnsPerHost.Value),
			Message: fmt.Sprintf("invalid http.client.max.idle.conns.per.host, must be between %d and %d", ValidHTTPClientMinIdleConnsPerHost, ValidHTTPClientMaxIdleConnsPerHost),
		}
	}

	if c.IdleConnTimeout.Value < ValidHTTPClientMinIdleConnTimeout || c.IdleConnTimeout.Value > ValidHTTPClientMaxIdleConnTimeout {
		return &InvalidConfigurationError{
			Field:   "http.client.idle.conn.timeout",
			Value:   fmt.Sprintf("%d", c.IdleConnTimeout.Value),
			Message: fmt.Sprintf("invalid http.client.idle.conn.timeout, must be between %d and %d", ValidHTTPClientMinIdleConnTimeout, ValidHTTPClientMaxIdleConnTimeout),
		}
	}

	if c.TLSHandshakeTimeout.Value < ValidHTTPClientMinTLSHandshakeTimeout || c.TLSHandshakeTimeout.Value > ValidHTTPClientMaxTLSHandshakeTimeout {
		return &InvalidConfigurationError{
			Field:   "http.client.tls.handshake.timeout",
			Value:   fmt.Sprintf("%d", c.TLSHandshakeTimeout.Value),
			Message: fmt.Sprintf("invalid http.client.tls.handshake.timeout, must be between %d and %d", ValidHTTPClientMinTLSHandshakeTimeout, ValidHTTPClientMaxTLSHandshakeTimeout),
		}
	}

	if c.ExpectContinueTimeout.Value < ValidHTTPClientMinExpectContinueTimeout || c.ExpectContinueTimeout.Value > ValidHTTPClientMaxExpectContinueTimeout {
		return &InvalidConfigurationError{
			Field:   "http.client.expect.continue.timeout",
			Value:   fmt.Sprintf("%d", c.ExpectContinueTimeout.Value),
			Message: fmt.Sprintf("invalid http.client.expect.continue.timeout, must be between %d and %d", ValidHTTPClientMinExpectContinueTimeout, ValidHTTPClientMaxExpectContinueTimeout),
		}
	}

	if c.Timeout.Value < ValidHTTPClientMinTimeout || c.Timeout.Value > ValidHTTPClientMaxTimeout {
		return &InvalidConfigurationError{
			Field:   "http.client.timeout",
			Value:   fmt.Sprintf("%d", c.Timeout.Value),
			Message: fmt.Sprintf("invalid http.client.timeout, must be between %d and %d", ValidHTTPClientMinTimeout, ValidHTTPClientMaxTimeout),
		}
	}

	if !slices.Contains(strings.Split(ValidHTTPClientRetryStrategies, "|"), c.RetryStrategy.Value) {
		return &InvalidConfigurationError{
			Field:   "http.client.retry.strategy",
			Value:   c.RetryStrategy.Value,
			Message: fmt.Sprintf("invalid http.client.retry.strategy, must be one of: %s", ValidHTTPClientRetryStrategies),
		}
	}

	if c.MaxRetries.Value < ValidHTTPClientMinMaxRetries || c.MaxRetries.Value > ValidHTTPClientMaxMaxRetries {
		return &InvalidConfigurationError{
			Field:   "http.client.max.retries",
			Value:   fmt.Sprintf("%d", c.MaxRetries.Value),
			Message: fmt.Sprintf("invalid http.client.max.retries, must be between %d and %d", ValidHTTPClientMinMaxRetries, ValidHTTPClientMaxMaxRetries),
		}
	}

	return nil
}
