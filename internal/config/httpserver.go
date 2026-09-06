package config

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"
)

const (
	ValidHTTPServerMaxPort              = 65535
	ValidHTTPServerMinPort              = 0
	ValidHTTPServerMaxShutdownTimeout   = 600 * time.Second
	ValidHTTPServerMinShutdownTimeout   = 1 * time.Second
	ValidHTTPServerCorsAllowedHeaders   = 2
	ValidHTTPServerMaxPprofPort         = 6060
	ValidHTTPServerMinPprofPort         = 6060
	ValidHTTPServerMaxReadHeaderTimeout = 120 * time.Second
	ValidHTTPServerMinReadHeaderTimeout = 1 * time.Second
	ValidHTTPServerMaxReadTimeout       = 1 * time.Hour
	ValidHTTPServerMinReadTimeout       = 1 * time.Second
	ValidHTTPServerMaxWriteTimeout      = 1 * time.Hour
	ValidHTTPServerMinWriteTimeout      = 1 * time.Second
	ValidHTTPServerMaxIdleTimeout       = 600 * time.Second
	ValidHTTPServerMinIdleTimeout       = 1 * time.Second
	ValidHTTPServerMaxMaxHeaderBytes    = 10 << 20 // 10 MiB
	ValidHTTPServerMinMaxHeaderBytes    = 4 << 10  // 4 KiB
	ValidHTTPServerMaxMaxBodyBytes      = 1 << 30  // 1 GiB
	ValidHTTPServerMinMaxBodyBytes      = 1 << 10  // 1 KiB
	ValidHTTPServerMaxHSTSMaxAge        = 10 * 365 * 24 * time.Hour
	ValidHTTPServerMinHSTSMaxAge        = 1 * time.Hour

	DefaultHTTPServerShutdownTimeout = 5 * time.Second
	DefaultHTTPServerAddress         = "localhost"
	DefaultHTTPServerPort            = 8080
	DefaultHTTPServerTLSEnabled      = false
	DefaultHTTPServerPprofPort       = 6060
	DefaultHTTPServerPprofAddress    = "localhost"
	DefaultHTTPServerPprofEnabled    = false

	// DefaultHTTPServerSwaggerEnabled is false. The swagger UI and its JSON
	// describe every operation, every field and every example -- including
	// the seeded administrator's credentials as the login example -- to
	// anyone who asks, on the same listener as the API. It is a development
	// aid; the dev stack switches it on, a deployment says so explicitly.
	DefaultHTTPServerSwaggerEnabled = false

	// DefaultHTTPServerReadHeaderTimeout bounds how long a client may take to
	// send its request headers. This is the Slowloris defence: without it a
	// client can hold a connection open indefinitely by dribbling out headers,
	// and Go applies no limit of its own. It is safe to enable by default
	// because it covers only the header read — never the handler.
	DefaultHTTPServerReadHeaderTimeout = 10 * time.Second

	// DefaultHTTPServerIdleTimeout bounds how long an idle keep-alive
	// connection is kept. It never affects a request in flight.
	DefaultHTTPServerIdleTimeout = 120 * time.Second

	// DefaultHTTPServerMaxHeaderBytes matches Go's own default (1 MiB) and is
	// stated explicitly so it is tunable without a code change.
	DefaultHTTPServerMaxHeaderBytes = 1 << 20

	// DefaultHTTPServerMaxBodyBytes bounds the request body on every route
	// that is not a bulk ingest. There used to be no bound at all: measured
	// 2026-09-06, a 200 MiB JSON body posted to /auth/login was read in full
	// before being rejected, taking the process from 52 MiB to 674 MiB RSS.
	// ReadTimeout is deliberately off (below), so this is the only thing that
	// stops one request from allocating whatever it likes.
	DefaultHTTPServerMaxBodyBytes = 1 << 20 // 1 MiB

	// DefaultHTTPServerMaxBodyBytesIngest is the bound for the embedding
	// ingest route, which legitimately posts a lot of text. It is a separate
	// setting because one number cannot serve both: large enough for ingest
	// is far too large for a login form.
	DefaultHTTPServerMaxBodyBytesIngest = 32 << 20 // 32 MiB

	// DefaultHTTPServerHSTSEnabled is false: Strict-Transport-Security is sent
	// when TLS terminates in this process, and this switch is for the other
	// case -- a proxy terminating TLS in front of a plaintext service. Sent
	// over plain HTTP the header is ignored by browsers, so a wrong "true" is
	// harmless; a wrong "false" behind a proxy leaves the downgrade open.
	DefaultHTTPServerHSTSEnabled = false

	// DefaultHTTPServerHSTSMaxAge is two years, the value the preload list
	// requires. Shorter means a browser forgets the pin sooner.
	DefaultHTTPServerHSTSMaxAge = 730 * 24 * time.Hour

	// DefaultHTTPServerPublicURL is empty, which makes every Location header
	// a path reference ("/api/v1/roles/<id>"). Set it to the URL clients use
	// to reach the service and Location becomes absolute. It used to be built
	// from the request's Origin header, so a caller chose the host in it.
	DefaultHTTPServerPublicURL = ""

	// DefaultHTTPServerReadTimeout is deliberately 0, meaning disabled.
	//
	// ReadTimeout covers accept-to-end-of-body, so it bounds how long a client
	// may take to upload. A bulk endpoint posts an unbounded amount of
	// text, and a large upload over a slow link is a legitimate request, not an
	// attack. ReadHeaderTimeout already closes the Slowloris hole, which is the
	// part that matters. Set this only if you know your body sizes.
	DefaultHTTPServerReadTimeout = 0 * time.Second

	// DefaultHTTPServerWriteTimeout is deliberately 0, meaning disabled.
	//
	// WriteTimeout starts when the request headers are read and covers the
	// whole handler, so it caps total request duration. That is fatal here: a
	// generation call is bounded by http.client.timeout (120s by default) but
	// may be retried up to http.client.max.retries (10) times, so a legitimate
	// request can run for roughly twenty minutes. A write deadline would cut
	// off exactly the request this service exists to serve. Set it only if you
	// have lowered the client timeout and retry budget to match.
	DefaultHTTPServerWriteTimeout = 0 * time.Second

	// DefaultHTTPServerCorsEnabled is the default value for enabling CORS
	// If enabled, the server will use the following values for CORS
	// - AllowedOrigins: "*"
	// - AllowedMethods: "GET, POST, PUT, DELETE, OPTIONS, PATCH, HEAD"
	// - AllowedHeaders: "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization, X-CSRF-Token, X-Requested-With, X-Api-Version"
	// Remember to change the values if you need to restrict the allowed origins, methods or headers
	DefaultHTTPServerCorsEnabled          = false
	DefaultHTTPServerCorsAllowCredentials = true

	// DefaultHTTPServerCorsAllowedOrigins is the default value for allowed origins
	// Could be a comma separated list of origins. Example: "http://localhost:3000, http://localhost:8080"
	DefaultHTTPServerCorsAllowedOrigins = "*" // allow all origins
	DefaultHTTPServerCorsAllowedMethods = "GET, POST, PUT, DELETE, OPTIONS, PATCH, HEAD"
	DefaultHTTPServerCorsAllowedHeaders = "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization, X-CSRF-Token, X-Requested-With, X-Api-Version, Access-Control-Allow-Headers"

	// DefaultHTTPServerTrustedProxies is empty on purpose: with no trusted
	// proxies the service ignores X-Forwarded-For and X-Real-IP and rate-limits
	// on the peer address. Trusting a forwarding header from an untrusted peer
	// lets the caller pick its own rate-limit bucket, which removes the limit
	// rather than weakening it — so the header has to be opted into per
	// deployment, naming the proxies that actually front this service.
	DefaultHTTPServerTrustedProxies = ""
)

const (
	ValidHTTPServerCorsAllowedMethods = "GET|POST|PUT|DELETE|OPTIONS|PATCH|HEAD"
)

var (
	// DefaultHTTPServerPrivateKeyFile is the default private key file for the server
	// DefaultHTTPServerPrivateKeyFile = "tls.key"
	DefaultHTTPServerPrivateKeyFile = FileVar{os.NewFile(0, "server.key"), os.O_RDONLY}

	// DefaultHTTPServerCertificateFile is the default certificate file for the server
	// DefaultHTTPServerCertificateFile = "tls.crt"
	DefaultHTTPServerCertificateFile = FileVar{os.NewFile(0, "server.crt"), os.O_RDONLY}
)

// HTTPServerConfig is the configuration for the server
type HTTPServerConfig struct {
	Address              Field[string]
	Port                 Field[int]
	ShutdownTimeout      Field[time.Duration]
	ReadHeaderTimeout    Field[time.Duration]
	ReadTimeout          Field[time.Duration]
	WriteTimeout         Field[time.Duration]
	IdleTimeout          Field[time.Duration]
	PrivateKeyFile       Field[FileVar]
	CertificateFile      Field[FileVar]
	CorsAllowedOrigins   Field[string]
	CorsAllowedMethods   Field[string]
	CorsAllowedHeaders   Field[string]
	TrustedProxies       Field[string]
	PprofAddress         Field[string]
	PprofPort            Field[int]
	MaxHeaderBytes       Field[int]
	MaxBodyBytes         Field[int]
	MaxBodyBytesIngest   Field[int]
	HSTSMaxAge           Field[time.Duration]
	PublicURL            Field[string]
	HSTSEnabled          Field[bool]
	SwaggerEnabled       Field[bool]
	TLSEnabled           Field[bool]
	PprofEnabled         Field[bool]
	CorsEnabled          Field[bool]
	CorsAllowCredentials Field[bool]
}

// NewHTTPServerConfig creates a new server configuration
func NewHTTPServerConfig() *HTTPServerConfig {
	return &HTTPServerConfig{
		Address:         NewField("http.server.address", "HTTP_SERVER_ADDRESS", "Server IP Address or Hostname", DefaultHTTPServerAddress),
		Port:            NewField("http.server.port", "HTTP_SERVER_PORT", "Server Port", DefaultHTTPServerPort),
		ShutdownTimeout: NewField("http.server.shutdown.timeout", "HTTP_SERVER_SHUTDOWN_TIMEOUT", "Server Shutdown Timeout", DefaultHTTPServerShutdownTimeout),
		PrivateKeyFile:  NewField("http.server.tls.key.file", "HTTP_SERVER_TLS_KEY_FILE", "Server Private Key File", DefaultHTTPServerPrivateKeyFile),
		CertificateFile: NewField("http.server.tls.cert.file", "HTTP_SERVER_TLS_CERT_FILE", "Server Certificate File", DefaultHTTPServerCertificateFile),
		TLSEnabled:      NewField("http.server.tls.enabled", "HTTP_SERVER_TLS_ENABLED", "Enable TLS", DefaultHTTPServerTLSEnabled),

		ReadHeaderTimeout:  NewField("http.server.read.header.timeout", "HTTP_SERVER_READ_HEADER_TIMEOUT", "Maximum time to read request headers. Bounds Slowloris-style attacks; does not limit the handler", DefaultHTTPServerReadHeaderTimeout),
		ReadTimeout:        NewField("http.server.read.timeout", "HTTP_SERVER_READ_TIMEOUT", "Maximum time to read the whole request including the body. 0 disables it, which is the default because bulk ingest bodies are unbounded", DefaultHTTPServerReadTimeout),
		WriteTimeout:       NewField("http.server.write.timeout", "HTTP_SERVER_WRITE_TIMEOUT", "Maximum time to write the response, measured from the end of the header read, so it caps total request duration. 0 disables it, which is the default because a retried generation can legitimately run for minutes", DefaultHTTPServerWriteTimeout),
		IdleTimeout:        NewField("http.server.idle.timeout", "HTTP_SERVER_IDLE_TIMEOUT", "Maximum time an idle keep-alive connection is kept open", DefaultHTTPServerIdleTimeout),
		MaxHeaderBytes:     NewField("http.server.max.header.bytes", "HTTP_SERVER_MAX_HEADER_BYTES", "Maximum size in bytes of the request headers", DefaultHTTPServerMaxHeaderBytes),
		MaxBodyBytes:       NewField("http.server.max.body.bytes", "HTTP_SERVER_MAX_BODY_BYTES", "Maximum size in bytes of a request body; a larger one is refused with 413 before it is read", DefaultHTTPServerMaxBodyBytes),
		MaxBodyBytesIngest: NewField("http.server.max.body.bytes.ingest", "HTTP_SERVER_MAX_BODY_BYTES_INGEST", "Maximum size in bytes of an embedding ingest request body; must not be below http.server.max.body.bytes", DefaultHTTPServerMaxBodyBytesIngest),
		HSTSEnabled:        NewField("http.server.hsts.enabled", "HTTP_SERVER_HSTS_ENABLED", "Send Strict-Transport-Security even without TLS in this process, for a deployment where a proxy terminates TLS. Always sent when http.server.tls.enabled is true", DefaultHTTPServerHSTSEnabled),
		HSTSMaxAge:         NewField("http.server.hsts.max.age", "HTTP_SERVER_HSTS_MAX_AGE", "Strict-Transport-Security max-age", DefaultHTTPServerHSTSMaxAge),
		PublicURL:          NewField("http.server.public.url", "HTTP_SERVER_PUBLIC_URL", "URL clients use to reach this service, e.g. https://api.example.com; when set, Location headers are absolute. Empty means Location is a path reference", DefaultHTTPServerPublicURL),

		PprofAddress:   NewField("http.server.pprof.address", "HTTP_SERVER_PPROF_ADDRESS", "Pprof Address", DefaultHTTPServerPprofAddress),
		PprofPort:      NewField("http.server.pprof.port", "HTTP_SERVER_PPROF_PORT", "Pprof Port", DefaultHTTPServerPprofPort),
		SwaggerEnabled: NewField("http.server.swagger.enabled", "HTTP_SERVER_SWAGGER_ENABLED", "Serve the swagger UI and JSON under /swagger/. Off by default: it documents every operation and example to anyone who asks", DefaultHTTPServerSwaggerEnabled),
		PprofEnabled:   NewField("http.server.pprof.enabled", "HTTP_SERVER_PPROF_ENABLED", "Enable pprof. WARNING: Enable this only for debugging, it has performance impact!", DefaultHTTPServerPprofEnabled),

		CorsEnabled:          NewField("http.server.cors.enabled", "HTTP_SERVER_CORS_ENABLED", "Enable CORS", DefaultHTTPServerCorsEnabled),
		CorsAllowCredentials: NewField("http.server.cors.allow.credentials", "HTTP_SERVER_CORS_ALLOW_CREDENTIALS", "Allow Credentials for CORS", DefaultHTTPServerCorsAllowCredentials),
		CorsAllowedOrigins:   NewField("http.server.cors.allowed.origins", "HTTP_SERVER_CORS_ALLOWED_ORIGINS", "Allowed Origins for CORS", DefaultHTTPServerCorsAllowedOrigins),
		CorsAllowedMethods:   NewField("http.server.cors.allowed.methods", "HTTP_SERVER_CORS_ALLOWED_METHODS", "Allowed Methods for CORS", DefaultHTTPServerCorsAllowedMethods),
		CorsAllowedHeaders:   NewField("http.server.cors.allowed.headers", "HTTP_SERVER_CORS_ALLOWED_HEADERS", "Allowed Headers for CORS", DefaultHTTPServerCorsAllowedHeaders),

		TrustedProxies: NewField("http.server.trusted.proxies", "HTTP_SERVER_TRUSTED_PROXIES", "Comma separated list of proxy IPs or CIDR blocks whose X-Forwarded-For and X-Real-IP headers are believed, e.g. \"10.0.0.0/8,192.0.2.10\". Empty means trust none and rate-limit on the peer address", DefaultHTTPServerTrustedProxies),
	}
}

// ParseEnvVars reads the server configuration from environment variables
// and sets the values in the configuration
func (c *HTTPServerConfig) ParseEnvVars() {
	c.Address.Value = GetEnv(c.Address.EnVarName, c.Address.Value)
	c.Port.Value = GetEnv(c.Port.EnVarName, c.Port.Value)
	c.ShutdownTimeout.Value = GetEnv(c.ShutdownTimeout.EnVarName, c.ShutdownTimeout.Value)
	c.PrivateKeyFile.Value = GetEnv(c.PrivateKeyFile.EnVarName, c.PrivateKeyFile.Value)
	c.CertificateFile.Value = GetEnv(c.CertificateFile.EnVarName, c.CertificateFile.Value)
	c.TLSEnabled.Value = GetEnv(c.TLSEnabled.EnVarName, c.TLSEnabled.Value)
	c.ReadHeaderTimeout.Value = GetEnv(c.ReadHeaderTimeout.EnVarName, c.ReadHeaderTimeout.Value)
	c.ReadTimeout.Value = GetEnv(c.ReadTimeout.EnVarName, c.ReadTimeout.Value)
	c.WriteTimeout.Value = GetEnv(c.WriteTimeout.EnVarName, c.WriteTimeout.Value)
	c.IdleTimeout.Value = GetEnv(c.IdleTimeout.EnVarName, c.IdleTimeout.Value)
	c.MaxHeaderBytes.Value = GetEnv(c.MaxHeaderBytes.EnVarName, c.MaxHeaderBytes.Value)
	c.MaxBodyBytes.Value = GetEnv(c.MaxBodyBytes.EnVarName, c.MaxBodyBytes.Value)
	c.MaxBodyBytesIngest.Value = GetEnv(c.MaxBodyBytesIngest.EnVarName, c.MaxBodyBytesIngest.Value)
	c.HSTSEnabled.Value = GetEnv(c.HSTSEnabled.EnVarName, c.HSTSEnabled.Value)
	c.HSTSMaxAge.Value = GetEnv(c.HSTSMaxAge.EnVarName, c.HSTSMaxAge.Value)
	c.PublicURL.Value = GetEnv(c.PublicURL.EnVarName, c.PublicURL.Value)

	c.PprofAddress.Value = GetEnv(c.PprofAddress.EnVarName, c.PprofAddress.Value)
	c.PprofPort.Value = GetEnv(c.PprofPort.EnVarName, c.PprofPort.Value)
	c.PprofEnabled.Value = GetEnv(c.PprofEnabled.EnVarName, c.PprofEnabled.Value)
	c.SwaggerEnabled.Value = GetEnv(c.SwaggerEnabled.EnVarName, c.SwaggerEnabled.Value)

	c.CorsEnabled.Value = GetEnv(c.CorsEnabled.EnVarName, c.CorsEnabled.Value)
	c.CorsAllowCredentials.Value = GetEnv(c.CorsAllowCredentials.EnVarName, c.CorsAllowCredentials.Value)
	c.CorsAllowedOrigins.Value = GetEnv(c.CorsAllowedOrigins.EnVarName, c.CorsAllowedOrigins.Value)
	c.CorsAllowedMethods.Value = GetEnv(c.CorsAllowedMethods.EnVarName, c.CorsAllowedMethods.Value)
	c.CorsAllowedHeaders.Value = GetEnv(c.CorsAllowedHeaders.EnVarName, c.CorsAllowedHeaders.Value)

	c.TrustedProxies.Value = GetEnv(c.TrustedProxies.EnVarName, c.TrustedProxies.Value)

}

// Validate validates the server configuration values
func (c *HTTPServerConfig) Validate() error {
	if c.Address.Value == "" || (c.Address.Value != "localhost" && net.ParseIP(c.Address.Value) == nil) {
		return &InvalidConfigurationError{
			Field:   "http.server.address",
			Value:   c.Address.Value,
			Message: "invalid http.server.address, must be a valid IP address or hostname",
		}
	}

	// validate the if is a valid IP Address or Hostname

	if c.Port.Value < ValidHTTPServerMinPort || c.Port.Value > ValidHTTPServerMaxPort || c.Port.Value == c.PprofPort.Value {
		return &InvalidConfigurationError{
			Field:   "http.server.port",
			Value:   fmt.Sprintf("%d", c.Port.Value),
			Message: fmt.Sprintf("invalid http.server.port, must be between %d and %d and not equal to http.server.pprof.port", ValidHTTPServerMinPort, ValidHTTPServerMaxPort),
		}
	}

	if c.ShutdownTimeout.Value < ValidHTTPServerMinShutdownTimeout || c.ShutdownTimeout.Value > ValidHTTPServerMaxShutdownTimeout {
		return &InvalidConfigurationError{
			Field:   "http.server.shutdown.timeout",
			Value:   fmt.Sprintf("%d", c.ShutdownTimeout.Value),
			Message: fmt.Sprintf("invalid http.server.shutdown.timeout, must be between %d and %d", ValidHTTPServerMinShutdownTimeout, ValidHTTPServerMaxShutdownTimeout),
		}
	}

	if c.ReadHeaderTimeout.Value < ValidHTTPServerMinReadHeaderTimeout || c.ReadHeaderTimeout.Value > ValidHTTPServerMaxReadHeaderTimeout {
		return &InvalidConfigurationError{
			Field:   "http.server.read.header.timeout",
			Value:   c.ReadHeaderTimeout.Value.String(),
			Message: fmt.Sprintf("invalid http.server.read.header.timeout, must be between %v and %v; it cannot be disabled because it is the only bound on a Slowloris-style header trickle", ValidHTTPServerMinReadHeaderTimeout, ValidHTTPServerMaxReadHeaderTimeout),
		}
	}

	// 0 means disabled for both body-side timeouts, so only a non-zero value is
	// range checked. See the Default* comments for why disabled is the default.
	if c.ReadTimeout.Value != 0 && (c.ReadTimeout.Value < ValidHTTPServerMinReadTimeout || c.ReadTimeout.Value > ValidHTTPServerMaxReadTimeout) {
		return &InvalidConfigurationError{
			Field:   "http.server.read.timeout",
			Value:   c.ReadTimeout.Value.String(),
			Message: fmt.Sprintf("invalid http.server.read.timeout, must be 0 to disable or between %v and %v", ValidHTTPServerMinReadTimeout, ValidHTTPServerMaxReadTimeout),
		}
	}

	// A read timeout below the header timeout is contradictory: the whole
	// request would have to finish before the headers alone were allowed to.
	if c.ReadTimeout.Value != 0 && c.ReadTimeout.Value < c.ReadHeaderTimeout.Value {
		return &InvalidConfigurationError{
			Field:   "http.server.read.timeout",
			Value:   c.ReadTimeout.Value.String(),
			Message: fmt.Sprintf("invalid http.server.read.timeout, must be at least http.server.read.header.timeout (%v)", c.ReadHeaderTimeout.Value),
		}
	}

	if c.WriteTimeout.Value != 0 && (c.WriteTimeout.Value < ValidHTTPServerMinWriteTimeout || c.WriteTimeout.Value > ValidHTTPServerMaxWriteTimeout) {
		return &InvalidConfigurationError{
			Field:   "http.server.write.timeout",
			Value:   c.WriteTimeout.Value.String(),
			Message: fmt.Sprintf("invalid http.server.write.timeout, must be 0 to disable or between %v and %v", ValidHTTPServerMinWriteTimeout, ValidHTTPServerMaxWriteTimeout),
		}
	}

	if c.IdleTimeout.Value < ValidHTTPServerMinIdleTimeout || c.IdleTimeout.Value > ValidHTTPServerMaxIdleTimeout {
		return &InvalidConfigurationError{
			Field:   "http.server.idle.timeout",
			Value:   c.IdleTimeout.Value.String(),
			Message: fmt.Sprintf("invalid http.server.idle.timeout, must be between %v and %v", ValidHTTPServerMinIdleTimeout, ValidHTTPServerMaxIdleTimeout),
		}
	}

	if c.MaxHeaderBytes.Value < ValidHTTPServerMinMaxHeaderBytes || c.MaxHeaderBytes.Value > ValidHTTPServerMaxMaxHeaderBytes {
		return &InvalidConfigurationError{
			Field:   "http.server.max.header.bytes",
			Value:   fmt.Sprintf("%d", c.MaxHeaderBytes.Value),
			Message: fmt.Sprintf("invalid http.server.max.header.bytes, must be between %d and %d", ValidHTTPServerMinMaxHeaderBytes, ValidHTTPServerMaxMaxHeaderBytes),
		}
	}

	if c.MaxBodyBytes.Value < ValidHTTPServerMinMaxBodyBytes || c.MaxBodyBytes.Value > ValidHTTPServerMaxMaxBodyBytes {
		return &InvalidConfigurationError{
			Field:   "http.server.max.body.bytes",
			Value:   fmt.Sprintf("%d", c.MaxBodyBytes.Value),
			Message: fmt.Sprintf("invalid http.server.max.body.bytes, must be between %d and %d", ValidHTTPServerMinMaxBodyBytes, ValidHTTPServerMaxMaxBodyBytes),
		}
	}

	if c.MaxBodyBytesIngest.Value < c.MaxBodyBytes.Value || c.MaxBodyBytesIngest.Value > ValidHTTPServerMaxMaxBodyBytes {
		return &InvalidConfigurationError{
			Field:   "http.server.max.body.bytes.ingest",
			Value:   fmt.Sprintf("%d", c.MaxBodyBytesIngest.Value),
			Message: fmt.Sprintf("invalid http.server.max.body.bytes.ingest, must be between http.server.max.body.bytes (%d) and %d", c.MaxBodyBytes.Value, ValidHTTPServerMaxMaxBodyBytes),
		}
	}

	if c.HSTSMaxAge.Value < ValidHTTPServerMinHSTSMaxAge || c.HSTSMaxAge.Value > ValidHTTPServerMaxHSTSMaxAge {
		return &InvalidConfigurationError{
			Field:   "http.server.hsts.max.age",
			Value:   c.HSTSMaxAge.Value.String(),
			Message: fmt.Sprintf("invalid http.server.hsts.max.age, must be between %v and %v", ValidHTTPServerMinHSTSMaxAge, ValidHTTPServerMaxHSTSMaxAge),
		}
	}

	if c.PublicURL.Value != "" {
		u, err := url.Parse(c.PublicURL.Value)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
			return &InvalidConfigurationError{
				Field:   "http.server.public.url",
				Value:   c.PublicURL.Value,
				Message: "invalid http.server.public.url, must be an absolute http or https URL with no query or fragment",
			}
		}
	}

	if c.CorsEnabled.Value {
		if c.CorsAllowedOrigins.Value == "" {
			return &InvalidConfigurationError{
				Field:   "http.server.cors.allowed.origins",
				Value:   c.CorsAllowedOrigins.Value,
				Message: "invalid http.server.cors.allowed.origins, must be a non-empty string",
			}
		}

		for method := range strings.SplitSeq(c.CorsAllowedMethods.Value, ",") {
			if !slices.Contains(strings.Split(ValidHTTPServerCorsAllowedMethods, "|"), strings.Trim(method, " ")) {
				return &InvalidConfigurationError{
					Field:   "http.server.cors.allowed.methods",
					Value:   c.CorsAllowedMethods.Value,
					Message: fmt.Sprintf("invalid http.server.cors.allowed.methods, must be one of: %s", ValidHTTPServerCorsAllowedMethods),
				}
			}
		}

		if len(c.CorsAllowedHeaders.Value) < ValidHTTPServerCorsAllowedHeaders {
			return &InvalidConfigurationError{
				Field:   "http.server.cors.allowed.headers",
				Value:   c.CorsAllowedHeaders.Value,
				Message: fmt.Sprintf("invalid http.server.cors.allowed.headers, must be at least %d elements", ValidHTTPServerCorsAllowedHeaders),
			}
		}
	}

	if c.PprofEnabled.Value {
		if c.PprofPort.Value < ValidHTTPServerMinPprofPort || c.PprofPort.Value > ValidHTTPServerMaxPprofPort || c.Port.Value == c.PprofPort.Value {
			return &InvalidConfigurationError{
				Field:   "http.server.pprof.port",
				Value:   fmt.Sprintf("%d", c.PprofPort.Value),
				Message: fmt.Sprintf("invalid http.server.pprof.port, must be between %d and %d and not equal to http.server.port", ValidHTTPServerMinPprofPort, ValidHTTPServerMaxPprofPort),
			}
		}

		if c.PprofAddress.Value == "" || (c.PprofAddress.Value != "localhost" && net.ParseIP(c.PprofAddress.Value) == nil) {
			return &InvalidConfigurationError{
				Field:   "http.server.pprof.address",
				Value:   c.PprofAddress.Value,
				Message: "invalid http.server.pprof.address, must be a valid IP address or hostname",
			}
		}
	}

	// rate-limit bucket.
	for _, entry := range c.TrustedProxiesList() {
		if _, err := netip.ParsePrefix(entry); err == nil {
			continue
		}
		if _, err := netip.ParseAddr(entry); err == nil {
			continue
		}

		return &InvalidConfigurationError{
			Field:   "http.server.trusted.proxies",
			Value:   entry,
			Message: "invalid http.server.trusted.proxies entry, each must be an IP address or a CIDR block",
		}
	}

	return nil
}

// TrustedProxiesList splits the configured value into entries, dropping blanks
// so that a trailing comma or a value padded for readability is not an error.
func (c *HTTPServerConfig) TrustedProxiesList() []string {
	entries := make([]string, 0)

	for raw := range strings.SplitSeq(c.TrustedProxies.Value, ",") {
		if entry := strings.TrimSpace(raw); entry != "" {
			entries = append(entries, entry)
		}
	}

	return entries
}
