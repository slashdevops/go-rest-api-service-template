package app

import (
	"flag"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"

	"github.com/slashdevops/go-rest-api-service-template/internal/config"
	"github.com/slashdevops/go-rest-api-service-template/internal/version"
	"github.com/slashdevops/go-rest-api-service-template/pkg/cslog"
)

// Configs contains all application configurations
type Configs struct {
	Log        *config.LogConfig
	HTTPServer *config.HTTPServerConfig
	HTTPClient *config.HTTPClientConfig
	Database   *config.DatabaseConfig
	Cache      *config.CacheConfig
	Telemetry  *config.OpenTelemetryConfig
	Authn      *config.AuthnConfig
	Mail       *config.MailConfig

	RateLimit       *config.RateLimitConfig
	ResourcesLimits *config.ResourcesLimitsConfig

	ShowVersion     bool
	ShowLongVersion bool
	ShowHelp        bool
}

// LoadConfigs loads all configuration from flags and environment variables
// newConfigs builds every configuration group with its defaults, registering
// nothing. It is separate from LoadConfigs so that the set of declared settings
// can be inspected without parsing a command line — which is what
// TestEveryConfigFieldHasAFlag does to catch a setting that was declared but
// never given a flag.
func newConfigs() *Configs {
	return &Configs{
		Log:        config.NewLogConfig(),
		HTTPServer: config.NewHTTPServerConfig(),
		HTTPClient: config.NewHTTPClientConfig(),
		Database:   config.NewDatabaseConfig(),
		Cache:      config.NewCacheConfig(),
		Telemetry:  config.NewOpenTelemetryConfig(appName, version.Version),
		Authn:      config.NewAuthConfig(),
		Mail:       config.NewMailConfig(),

		RateLimit:       config.NewRateLimitConfig(),
		ResourcesLimits: config.NewResourcesLimitsConfig(),
	}
}

// parseEnvVars reads every configuration group from the environment.
//
// The group list is hand-maintained, like the flag registrations and each
// group's own ParseEnvVars: a group left out of it silently keeps its defaults
// no matter what an operator sets. Extracted so that
// TestEveryConfigFieldIsReadFromTheEnvironment drives the same code path
// production does, which is what makes the omission detectable.
func parseEnvVars(configs *Configs) {
	config.ParseEnvVars(
		configs.Log,
		configs.HTTPServer,
		configs.HTTPClient,
		configs.Database,
		configs.Cache,
		configs.Telemetry,
		configs.Authn,
		configs.Mail,
		configs.RateLimit,
		configs.ResourcesLimits,
	)
}

func LoadConfigs() (*Configs, error) {
	configs := newConfigs()

	// Register flags
	setupFlags(configs)

	// Parse the command line arguments
	flag.Parse()

	// Handle special flags
	if err := handleSpecialFlags(configs); err != nil {
		return nil, err
	}

	// Load environment variables
	if err := config.SetEnvVarFromFile(); err != nil {
		slog.Error("failed to set environment variables from .env file", "error", err)
		return nil, err
	}

	parseEnvVars(configs)

	// Validate configuration
	if err := config.Validate(
		configs.Log,
		configs.HTTPServer,
		configs.HTTPClient,
		configs.Database,
		configs.Cache,
		configs.Telemetry,
		configs.Authn,
		configs.Mail,
		configs.RateLimit,
	); err != nil {
		return nil, fmt.Errorf("error validating configuration: %w", err)
	}

	// Setup logger based on configuration
	setupLogger(configs.Log)

	return configs, nil
}

// setupFlags configures command line flags for all application configurations
func setupFlags(configs *Configs) {
	// Version, Help and debug flags
	flag.BoolVar(&configs.ShowVersion, "version", false, "Show the version information")
	flag.BoolVar(&configs.ShowLongVersion, "version.long", false, "Show the long version information")
	flag.BoolVar(&configs.ShowHelp, "help", false, "Show this help message")

	// Log configuration values
	flag.StringVar(&configs.Log.Level.Value, configs.Log.Level.FlagName, config.DefaultLogLevel, configs.Log.Level.FlagDescription)
	flag.StringVar(&configs.Log.Format.Value, configs.Log.Format.FlagName, config.DefaultLogFormat, configs.Log.Format.FlagDescription)
	flag.Var(&configs.Log.Output.Value, configs.Log.Output.FlagName, configs.Log.Output.FlagDescription)
	flag.BoolVar(&configs.Log.Debug.Value, configs.Log.Debug.FlagName, config.DefaultLogDebug, configs.Log.Debug.FlagDescription)
	flag.BoolVar(&configs.Log.AddSource.Value, configs.Log.AddSource.FlagName, config.DefaultLogAddSource, configs.Log.AddSource.FlagDescription)

	// HTTP Server configuration values
	flag.StringVar(&configs.HTTPServer.Address.Value, configs.HTTPServer.Address.FlagName, config.DefaultHTTPServerAddress, configs.HTTPServer.Address.FlagDescription)
	flag.IntVar(&configs.HTTPServer.Port.Value, configs.HTTPServer.Port.FlagName, config.DefaultHTTPServerPort, configs.HTTPServer.Port.FlagDescription)
	flag.DurationVar(&configs.HTTPServer.ShutdownTimeout.Value, configs.HTTPServer.ShutdownTimeout.FlagName, config.DefaultHTTPServerShutdownTimeout, configs.HTTPServer.ShutdownTimeout.FlagDescription)
	flag.DurationVar(&configs.HTTPServer.ReadHeaderTimeout.Value, configs.HTTPServer.ReadHeaderTimeout.FlagName, config.DefaultHTTPServerReadHeaderTimeout, configs.HTTPServer.ReadHeaderTimeout.FlagDescription)
	flag.DurationVar(&configs.HTTPServer.ReadTimeout.Value, configs.HTTPServer.ReadTimeout.FlagName, config.DefaultHTTPServerReadTimeout, configs.HTTPServer.ReadTimeout.FlagDescription)
	flag.DurationVar(&configs.HTTPServer.WriteTimeout.Value, configs.HTTPServer.WriteTimeout.FlagName, config.DefaultHTTPServerWriteTimeout, configs.HTTPServer.WriteTimeout.FlagDescription)
	flag.DurationVar(&configs.HTTPServer.IdleTimeout.Value, configs.HTTPServer.IdleTimeout.FlagName, config.DefaultHTTPServerIdleTimeout, configs.HTTPServer.IdleTimeout.FlagDescription)
	flag.IntVar(&configs.HTTPServer.MaxHeaderBytes.Value, configs.HTTPServer.MaxHeaderBytes.FlagName, config.DefaultHTTPServerMaxHeaderBytes, configs.HTTPServer.MaxHeaderBytes.FlagDescription)
	flag.Var(&configs.HTTPServer.PrivateKeyFile.Value, configs.HTTPServer.PrivateKeyFile.FlagName, configs.HTTPServer.PrivateKeyFile.FlagDescription)
	flag.Var(&configs.HTTPServer.CertificateFile.Value, configs.HTTPServer.CertificateFile.FlagName, configs.HTTPServer.CertificateFile.FlagDescription)
	flag.BoolVar(&configs.HTTPServer.TLSEnabled.Value, configs.HTTPServer.TLSEnabled.FlagName, config.DefaultHTTPServerTLSEnabled, configs.HTTPServer.TLSEnabled.FlagDescription)
	flag.StringVar(&configs.HTTPServer.PprofAddress.Value, configs.HTTPServer.PprofAddress.FlagName, config.DefaultHTTPServerPprofAddress, configs.HTTPServer.PprofAddress.FlagDescription)
	flag.IntVar(&configs.HTTPServer.PprofPort.Value, configs.HTTPServer.PprofPort.FlagName, config.DefaultHTTPServerPprofPort, configs.HTTPServer.PprofPort.FlagDescription)
	flag.BoolVar(&configs.HTTPServer.PprofEnabled.Value, configs.HTTPServer.PprofEnabled.FlagName, config.DefaultHTTPServerPprofEnabled, configs.HTTPServer.PprofEnabled.FlagDescription)
	flag.BoolVar(&configs.HTTPServer.CorsEnabled.Value, configs.HTTPServer.CorsEnabled.FlagName, config.DefaultHTTPServerCorsEnabled, configs.HTTPServer.CorsEnabled.FlagDescription)
	flag.BoolVar(&configs.HTTPServer.CorsAllowCredentials.Value, configs.HTTPServer.CorsAllowCredentials.FlagName, config.DefaultHTTPServerCorsAllowCredentials, configs.HTTPServer.CorsAllowCredentials.FlagDescription)
	flag.StringVar(&configs.HTTPServer.CorsAllowedOrigins.Value, configs.HTTPServer.CorsAllowedOrigins.FlagName, config.DefaultHTTPServerCorsAllowedOrigins, configs.HTTPServer.CorsAllowedOrigins.FlagDescription)
	flag.StringVar(&configs.HTTPServer.CorsAllowedMethods.Value, configs.HTTPServer.CorsAllowedMethods.FlagName, config.DefaultHTTPServerCorsAllowedMethods, configs.HTTPServer.CorsAllowedMethods.FlagDescription)
	flag.StringVar(&configs.HTTPServer.CorsAllowedHeaders.Value, configs.HTTPServer.CorsAllowedHeaders.FlagName, config.DefaultHTTPServerCorsAllowedHeaders, configs.HTTPServer.CorsAllowedHeaders.FlagDescription)

	// HTTP Rate Limiter configuration
	flag.StringVar(&configs.HTTPServer.TrustedProxies.Value, configs.HTTPServer.TrustedProxies.FlagName, config.DefaultHTTPServerTrustedProxies, configs.HTTPServer.TrustedProxies.FlagDescription)

	// Rate-limit rules. One line per Field, by hand — a Field without a line
	// works through the environment while STOPPING THE PROCESS when passed as a
	// flag, which is the failure mode TestEveryConfigFieldHasAFlag exists for.
	flag.BoolVar(&configs.RateLimit.Enabled.Value, configs.RateLimit.Enabled.FlagName, config.DefaultRateLimitEnabled, configs.RateLimit.Enabled.FlagDescription)
	flag.DurationVar(&configs.RateLimit.ReloadInterval.Value, configs.RateLimit.ReloadInterval.FlagName, config.DefaultRateLimitReloadInterval, configs.RateLimit.ReloadInterval.FlagDescription)
	flag.DurationVar(&configs.RateLimit.StoreTimeout.Value, configs.RateLimit.StoreTimeout.FlagName, config.DefaultRateLimitStoreTimeout, configs.RateLimit.StoreTimeout.FlagDescription)
	flag.StringVar(&configs.RateLimit.StoreFailMode.Value, configs.RateLimit.StoreFailMode.FlagName, config.DefaultRateLimitStoreFailMode, configs.RateLimit.StoreFailMode.FlagDescription)
	flag.IntVar(&configs.RateLimit.StoreBreakerThreshold.Value, configs.RateLimit.StoreBreakerThreshold.FlagName, config.DefaultRateLimitStoreBreakerThreshold, configs.RateLimit.StoreBreakerThreshold.FlagDescription)
	flag.DurationVar(&configs.RateLimit.StoreBreakerCooldown.Value, configs.RateLimit.StoreBreakerCooldown.FlagName, config.DefaultRateLimitStoreBreakerCooldown, configs.RateLimit.StoreBreakerCooldown.FlagDescription)
	flag.DurationVar(&configs.RateLimit.BucketSweepInterval.Value, configs.RateLimit.BucketSweepInterval.FlagName, config.DefaultRateLimitBucketSweepInterval, configs.RateLimit.BucketSweepInterval.FlagDescription)
	flag.DurationVar(&configs.RateLimit.BucketIdleAfter.Value, configs.RateLimit.BucketIdleAfter.FlagName, config.DefaultRateLimitBucketIdleAfter, configs.RateLimit.BucketIdleAfter.FlagDescription)
	flag.Var(&configs.RateLimit.ExcludedIPs.Value, configs.RateLimit.ExcludedIPs.FlagName, configs.RateLimit.ExcludedIPs.FlagDescription)
	flag.Var(&configs.RateLimit.BypassPrefixes.Value, configs.RateLimit.BypassPrefixes.FlagName, configs.RateLimit.BypassPrefixes.FlagDescription)

	// HTTP Client configuration values
	flag.IntVar(&configs.HTTPClient.MaxIdleConns.Value, configs.HTTPClient.MaxIdleConns.FlagName, config.DefaultHTTPClientMaxIdleConns, configs.HTTPClient.MaxIdleConns.FlagDescription)
	flag.IntVar(&configs.HTTPClient.MaxIdleConnsPerHost.Value, configs.HTTPClient.MaxIdleConnsPerHost.FlagName, config.DefaultHTTPClientMaxIdleConnsPerHost, configs.HTTPClient.MaxIdleConnsPerHost.FlagDescription)
	flag.DurationVar(&configs.HTTPClient.IdleConnTimeout.Value, configs.HTTPClient.IdleConnTimeout.FlagName, config.DefaultHTTPClientIdleConnTimeout, configs.HTTPClient.IdleConnTimeout.FlagDescription)
	flag.DurationVar(&configs.HTTPClient.TLSHandshakeTimeout.Value, configs.HTTPClient.TLSHandshakeTimeout.FlagName, config.DefaultHTTPClientTLSHandshakeTimeout, configs.HTTPClient.TLSHandshakeTimeout.FlagDescription)
	flag.DurationVar(&configs.HTTPClient.ExpectContinueTimeout.Value, configs.HTTPClient.ExpectContinueTimeout.FlagName, config.DefaultHTTPClientExpectContinueTimeout, configs.HTTPClient.ExpectContinueTimeout.FlagDescription)
	flag.BoolVar(&configs.HTTPClient.DisableKeepAlives.Value, configs.HTTPClient.DisableKeepAlives.FlagName, config.DefaultHTTPClientDisableKeepAlives, configs.HTTPClient.DisableKeepAlives.FlagDescription)
	flag.DurationVar(&configs.HTTPClient.Timeout.Value, configs.HTTPClient.Timeout.FlagName, config.DefaultHTTPClientTimeout, configs.HTTPClient.Timeout.FlagDescription)
	flag.IntVar(&configs.HTTPClient.MaxRetries.Value, configs.HTTPClient.MaxRetries.FlagName, config.DefaultHTTPClientMaxRetries, configs.HTTPClient.MaxRetries.FlagDescription)
	flag.StringVar(&configs.HTTPClient.RetryStrategy.Value, configs.HTTPClient.RetryStrategy.FlagName, config.DefaultHTTPClientRetryStrategy, configs.HTTPClient.RetryStrategy.FlagDescription)

	// Database configuration values
	flag.StringVar(&configs.Database.Kind.Value, configs.Database.Kind.FlagName, config.DefaultDatabaseKind, configs.Database.Kind.FlagDescription)
	flag.StringVar(&configs.Database.Address.Value, configs.Database.Address.FlagName, config.DefaultDatabaseAddress, configs.Database.Address.FlagDescription)
	flag.IntVar(&configs.Database.Port.Value, configs.Database.Port.FlagName, config.DefaultDatabasePort, configs.Database.Port.FlagDescription)
	flag.StringVar(&configs.Database.Username.Value, configs.Database.Username.FlagName, config.DefaultDatabaseUsername, configs.Database.Username.FlagDescription)
	flag.StringVar(&configs.Database.Password.Value, configs.Database.Password.FlagName, config.DefaultDatabasePassword, configs.Database.Password.FlagDescription)
	flag.StringVar(&configs.Database.Name.Value, configs.Database.Name.FlagName, config.DefaultDatabaseName, configs.Database.Name.FlagDescription)
	flag.StringVar(&configs.Database.SSLMode.Value, configs.Database.SSLMode.FlagName, config.DefaultDatabaseSSLMode, configs.Database.SSLMode.FlagDescription)
	flag.StringVar(&configs.Database.SSLRootCertFile.Value, configs.Database.SSLRootCertFile.FlagName, "", configs.Database.SSLRootCertFile.FlagDescription)
	flag.StringVar(&configs.Database.SSLCertFile.Value, configs.Database.SSLCertFile.FlagName, "", configs.Database.SSLCertFile.FlagDescription)
	flag.StringVar(&configs.Database.SSLKeyFile.Value, configs.Database.SSLKeyFile.FlagName, "", configs.Database.SSLKeyFile.FlagDescription)
	flag.StringVar(&configs.Database.TimeZone.Value, configs.Database.TimeZone.FlagName, config.DefaultDatabaseTimeZone, configs.Database.TimeZone.FlagDescription)
	flag.DurationVar(&configs.Database.MaxPingTimeout.Value, configs.Database.MaxPingTimeout.FlagName, config.DefaultDatabaseMaxPingTimeout, configs.Database.MaxPingTimeout.FlagDescription)
	flag.DurationVar(&configs.Database.MaxQueryTimeout.Value, configs.Database.MaxQueryTimeout.FlagName, config.DefaultDatabaseMaxQueryTimeout, configs.Database.MaxQueryTimeout.FlagDescription)
	flag.DurationVar(&configs.Database.ConnMaxLifetime.Value, configs.Database.ConnMaxLifetime.FlagName, config.DefaultDatabaseConnMaxLifetime, configs.Database.ConnMaxLifetime.FlagDescription)
	flag.DurationVar(&configs.Database.ConnMaxIdleTime.Value, configs.Database.ConnMaxIdleTime.FlagName, config.DefaultDatabaseConnMaxIdleTime, configs.Database.ConnMaxIdleTime.FlagDescription)
	flag.IntVar(&configs.Database.MaxConns.Value, configs.Database.MaxConns.FlagName, config.DefaultDatabaseMaxConns, configs.Database.MaxConns.FlagDescription)
	flag.IntVar(&configs.Database.MinConns.Value, configs.Database.MinConns.FlagName, config.DefaultDatabaseMinConns, configs.Database.MinConns.FlagDescription)
	flag.BoolVar(&configs.Database.MigrationEnable.Value, configs.Database.MigrationEnable.FlagName, config.DefaultDatabaseMigrationEnable, configs.Database.MigrationEnable.FlagDescription)

	// Resource limits configuration values
	flag.BoolVar(&configs.ResourcesLimits.ReconcileOnStart.Value, configs.ResourcesLimits.ReconcileOnStart.FlagName, config.DefaultResourcesLimitsReconcileOnStart, configs.ResourcesLimits.ReconcileOnStart.FlagDescription)
	flag.Var(&configs.ResourcesLimits.SigningPrivateKeyFile.Value, configs.ResourcesLimits.SigningPrivateKeyFile.FlagName, configs.ResourcesLimits.SigningPrivateKeyFile.FlagDescription)
	flag.Var(&configs.ResourcesLimits.SigningPublicKeyFile.Value, configs.ResourcesLimits.SigningPublicKeyFile.FlagName, configs.ResourcesLimits.SigningPublicKeyFile.FlagDescription)

	// Cache configuration values
	flag.DurationVar(&configs.Cache.InvalidateTimeout.Value, configs.Cache.InvalidateTimeout.FlagName, config.DefaultCacheInvalidateTimeout, configs.Cache.InvalidateTimeout.FlagDescription)
	flag.StringVar(&configs.Cache.ServerKind.Value, configs.Cache.ServerKind.FlagName, config.DefaultCacheServerKind, configs.Cache.ServerKind.FlagDescription)
	flag.Var(&configs.Cache.ServerAddresses.Value, configs.Cache.ServerAddresses.FlagName, configs.Cache.ServerAddresses.FlagDescription)
	flag.StringVar(&configs.Cache.ServerUsername.Value, configs.Cache.ServerUsername.FlagName, config.DefaultCacheServerUsername, configs.Cache.ServerUsername.FlagDescription)
	flag.StringVar(&configs.Cache.ServerPassword.Value, configs.Cache.ServerPassword.FlagName, config.DefaultCacheServerPassword, configs.Cache.ServerPassword.FlagDescription)
	flag.IntVar(&configs.Cache.ServerDB.Value, configs.Cache.ServerDB.FlagName, config.DefaultCacheServerDB, configs.Cache.ServerDB.FlagDescription)
	flag.DurationVar(&configs.Cache.MaxQueryTimeout.Value, configs.Cache.MaxQueryTimeout.FlagName, config.DefaultCacheMaxQueryTimeout, configs.Cache.MaxQueryTimeout.FlagDescription)
	flag.DurationVar(&configs.Cache.EntitiesHardTTL.Value, configs.Cache.EntitiesHardTTL.FlagName, config.DefaultCacheHardEntitiesTTL, configs.Cache.EntitiesHardTTL.FlagDescription)
	flag.DurationVar(&configs.Cache.EntitiesSoftTTL.Value, configs.Cache.EntitiesSoftTTL.FlagName, config.DefaultCacheSoftEntitiesTTL, configs.Cache.EntitiesSoftTTL.FlagDescription)
	flag.Float64Var(&configs.Cache.TTLJitterPercent.Value, configs.Cache.TTLJitterPercent.FlagName, config.DefaultCacheTTLJitterPercent, configs.Cache.TTLJitterPercent.FlagDescription)
	flag.BoolVar(&configs.Cache.EnableOnClient.Value, configs.Cache.EnableOnClient.FlagName, config.DefaultCacheEnableOnClient, configs.Cache.EnableOnClient.FlagDescription)
	flag.BoolVar(&configs.Cache.GeneratedAnswers.Value, configs.Cache.GeneratedAnswers.FlagName, config.DefaultCacheGeneratedAnswersEnabled, configs.Cache.GeneratedAnswers.FlagDescription)
	flag.BoolVar(&configs.Cache.TLSEnabled.Value, configs.Cache.TLSEnabled.FlagName, config.DefaultCacheTLSEnabled, configs.Cache.TLSEnabled.FlagDescription)
	flag.StringVar(&configs.Cache.TLSCAFile.Value, configs.Cache.TLSCAFile.FlagName, "", configs.Cache.TLSCAFile.FlagDescription)
	flag.StringVar(&configs.Cache.TLSCertFile.Value, configs.Cache.TLSCertFile.FlagName, "", configs.Cache.TLSCertFile.FlagDescription)
	flag.StringVar(&configs.Cache.TLSKeyFile.Value, configs.Cache.TLSKeyFile.FlagName, "", configs.Cache.TLSKeyFile.FlagDescription)
	flag.BoolVar(&configs.Cache.TLSInsecureSkipVerify.Value, configs.Cache.TLSInsecureSkipVerify.FlagName, config.DefaultCacheTLSInsecureSkipVerify, configs.Cache.TLSInsecureSkipVerify.FlagDescription)
	flag.StringVar(&configs.Cache.EncoderType.Value, configs.Cache.EncoderType.FlagName, config.DefaultCacheEncoderType.String(), configs.Cache.EncoderType.FlagDescription)
	flag.BoolVar(&configs.Cache.Enabled.Value, configs.Cache.Enabled.FlagName, config.DefaultCacheEnabled, configs.Cache.Enabled.FlagDescription)

	// OpenTelemetry configuration values
	flag.StringVar(&configs.Telemetry.TraceEndpoint.Value, configs.Telemetry.TraceEndpoint.FlagName, config.DefaultTraceEndpoint, configs.Telemetry.TraceEndpoint.FlagDescription)
	flag.IntVar(&configs.Telemetry.TracePort.Value, configs.Telemetry.TracePort.FlagName, config.DefaultTracePort, configs.Telemetry.TracePort.FlagDescription)
	flag.StringVar(&configs.Telemetry.TraceExporter.Value, configs.Telemetry.TraceExporter.FlagName, config.DefaultTraceExporter, configs.Telemetry.TraceExporter.FlagDescription)
	flag.DurationVar(&configs.Telemetry.TraceExporterBatchTimeout.Value, configs.Telemetry.TraceExporterBatchTimeout.FlagName, config.DefaultTraceExporterBatchTimeout, configs.Telemetry.TraceExporterBatchTimeout.FlagDescription)
	flag.IntVar(&configs.Telemetry.TraceSampling.Value, configs.Telemetry.TraceSampling.FlagName, config.DefaultTraceSampling, configs.Telemetry.TraceSampling.FlagDescription)
	flag.StringVar(&configs.Telemetry.MetricEndpoint.Value, configs.Telemetry.MetricEndpoint.FlagName, config.DefaultMetricEndpoint, configs.Telemetry.MetricEndpoint.FlagDescription)
	flag.IntVar(&configs.Telemetry.MetricPort.Value, configs.Telemetry.MetricPort.FlagName, config.DefaultMetricPort, configs.Telemetry.MetricPort.FlagDescription)
	flag.StringVar(&configs.Telemetry.MetricExporter.Value, configs.Telemetry.MetricExporter.FlagName, config.DefaultMetricExporter, configs.Telemetry.MetricExporter.FlagDescription)
	flag.DurationVar(&configs.Telemetry.MetricInterval.Value, configs.Telemetry.MetricInterval.FlagName, config.DefaultMetricInterval, configs.Telemetry.MetricInterval.FlagDescription)

	// Authentication configuration values
	flag.StringVar(&configs.Authn.Issuer.Value, configs.Authn.Issuer.FlagName, config.DefaultAuthnIssuer, configs.Authn.Issuer.FlagDescription)
	flag.Var(&configs.Authn.PrivateKeyFile.Value, configs.Authn.PrivateKeyFile.FlagName, configs.Authn.PrivateKeyFile.FlagDescription)
	flag.Var(&configs.Authn.PublicKeyFile.Value, configs.Authn.PublicKeyFile.FlagName, configs.Authn.PublicKeyFile.FlagDescription)
	flag.StringVar(&configs.Authn.AdditionalPublicKeyFiles.Value, configs.Authn.AdditionalPublicKeyFiles.FlagName, "", configs.Authn.AdditionalPublicKeyFiles.FlagDescription)
	flag.Var(&configs.Authn.SymmetricKeyFile.Value, configs.Authn.SymmetricKeyFile.FlagName, configs.Authn.SymmetricKeyFile.FlagDescription)
	flag.BoolVar(&configs.Authn.LoginThrottleEnabled.Value, configs.Authn.LoginThrottleEnabled.FlagName, config.DefaultAuthnLoginThrottleEnabled, configs.Authn.LoginThrottleEnabled.FlagDescription)
	flag.IntVar(&configs.Authn.LoginThrottleMaxAttempts.Value, configs.Authn.LoginThrottleMaxAttempts.FlagName, config.DefaultAuthnLoginThrottleMaxAttempts, configs.Authn.LoginThrottleMaxAttempts.FlagDescription)
	flag.DurationVar(&configs.Authn.LoginThrottleWindow.Value, configs.Authn.LoginThrottleWindow.FlagName, config.DefaultAuthnLoginThrottleWindow, configs.Authn.LoginThrottleWindow.FlagDescription)
	flag.DurationVar(&configs.Authn.LoginThrottleIdleAfter.Value, configs.Authn.LoginThrottleIdleAfter.FlagName, config.DefaultAuthnLoginThrottleIdleAfter, configs.Authn.LoginThrottleIdleAfter.FlagDescription)
	flag.DurationVar(&configs.Authn.TokenLifetimesReloadInterval.Value, configs.Authn.TokenLifetimesReloadInterval.FlagName, config.DefaultAuthnTokenLifetimesReloadInterval, configs.Authn.TokenLifetimesReloadInterval.FlagDescription)
	flag.BoolVar(&configs.Authn.RefreshTokenRotationEnabled.Value, configs.Authn.RefreshTokenRotationEnabled.FlagName, config.DefaultAuthnRefreshTokenRotationEnabled, configs.Authn.RefreshTokenRotationEnabled.FlagDescription)
	flag.DurationVar(&configs.Authn.RefreshTokenRotationGrace.Value, configs.Authn.RefreshTokenRotationGrace.FlagName, config.DefaultAuthnRefreshTokenRotationGrace, configs.Authn.RefreshTokenRotationGrace.FlagDescription)
	flag.DurationVar(&configs.Authn.RevokedTokensSweepInterval.Value, configs.Authn.RevokedTokensSweepInterval.FlagName, config.DefaultAuthnRevokedTokensSweepInterval, configs.Authn.RevokedTokensSweepInterval.FlagDescription)
	flag.BoolVar(&configs.Authn.AccessTokenRevocationEnabled.Value, configs.Authn.AccessTokenRevocationEnabled.FlagName, config.DefaultAuthnAccessTokenRevocationEnabled, configs.Authn.AccessTokenRevocationEnabled.FlagDescription)
	flag.DurationVar(&configs.Authn.AccessTokenRevocationReloadInterval.Value, configs.Authn.AccessTokenRevocationReloadInterval.FlagName, config.DefaultAuthnAccessTokenRevocationReloadInterval, configs.Authn.AccessTokenRevocationReloadInterval.FlagDescription)
	flag.StringVar(&configs.Authn.UserVerificationWebEndpoint.Value, configs.Authn.UserVerificationWebEndpoint.FlagName, config.DefaultAuthnUserVerificationWebEndpoint, configs.Authn.UserVerificationWebEndpoint.FlagDescription)
	flag.DurationVar(&configs.Authn.UserVerificationTokenTTL.Value, configs.Authn.UserVerificationTokenTTL.FlagName, config.DefaultAuthnUserVerificationTokenTTL, configs.Authn.UserVerificationTokenTTL.FlagDescription)
	flag.StringVar(&configs.Authn.UserResetPasswordEndpoint.Value, configs.Authn.UserResetPasswordEndpoint.FlagName, config.DefaultAuthnUserResetPasswordEndpoint, configs.Authn.UserResetPasswordEndpoint.FlagDescription)
	flag.DurationVar(&configs.Authn.UserResetPasswordTokenTTL.Value, configs.Authn.UserResetPasswordTokenTTL.FlagName, config.DefaultAuthnUserResetPasswordTokenTTL, configs.Authn.UserResetPasswordTokenTTL.FlagDescription)

	// Mail configuration values
	flag.StringVar(&configs.Mail.SMTPHost.Value, configs.Mail.SMTPHost.FlagName, config.DefaultMailSMTPHost, configs.Mail.SMTPHost.FlagDescription)
	flag.IntVar(&configs.Mail.SMTPPort.Value, configs.Mail.SMTPPort.FlagName, config.DefaultMailSMTPPort, configs.Mail.SMTPPort.FlagDescription)
	flag.StringVar(&configs.Mail.SMTPUsername.Value, configs.Mail.SMTPUsername.FlagName, config.DefaultMailSMTPUsername, configs.Mail.SMTPUsername.FlagDescription)
	flag.StringVar(&configs.Mail.SMTPPassword.Value, configs.Mail.SMTPPassword.FlagName, config.DefaultMailSMTPPassword, configs.Mail.SMTPPassword.FlagDescription)
	flag.BoolVar(&configs.Mail.SMTPRequireTLS.Value, configs.Mail.SMTPRequireTLS.FlagName, config.DefaultMailSMTPRequireTLS, configs.Mail.SMTPRequireTLS.FlagDescription)
	flag.StringVar(&configs.Mail.SenderName.Value, configs.Mail.SenderName.FlagName, config.DefaultMailSenderName, configs.Mail.SenderName.FlagDescription)
	flag.StringVar(&configs.Mail.SenderAddress.Value, configs.Mail.SenderAddress.FlagName, config.DefaultMailSenderAddress, configs.Mail.SenderAddress.FlagDescription)
	flag.StringVar(&configs.Mail.APIURL.Value, configs.Mail.APIURL.FlagName, config.DefaultMailAPIURL, configs.Mail.APIURL.FlagDescription)
	flag.StringVar(&configs.Mail.APIKey.Value, configs.Mail.APIKey.FlagName, config.DefaultMailAPIKey, configs.Mail.APIKey.FlagDescription)
	flag.StringVar(&configs.Mail.MailSender.Value, configs.Mail.MailSender.FlagName, config.DefaultMailSender, configs.Mail.MailSender.FlagDescription)
	flag.IntVar(&configs.Mail.MailWorkerCount.Value, configs.Mail.MailWorkerCount.FlagName, config.DefaultMailWorkerCount, configs.Mail.MailWorkerCount.FlagDescription)
	flag.DurationVar(&configs.Mail.MailWorkerTimeout.Value, configs.Mail.MailWorkerTimeout.FlagName, config.DefaultMailWorkerTimeout, configs.Mail.MailWorkerTimeout.FlagDescription)
	flag.IntVar(&configs.Mail.MailQueueSize.Value, configs.Mail.MailQueueSize.FlagName, config.DefaultMailQueueSize, configs.Mail.MailQueueSize.FlagDescription)

}

// handleSpecialFlags handles flags that control application execution flow
// such as version display, help display, etc.
func handleSpecialFlags(configs *Configs) error {
	// Handle version flag
	if configs.ShowVersion {

		if version.Version == "0.0.0" {
			if info, ok := debug.ReadBuildInfo(); ok {
				fmt.Printf("Version: %s\n", info.Main.Version)
			} else {
				fmt.Printf("Version: %s\n", version.Version)
			}
		} else {
			fmt.Printf("Version: %s\n", version.Version)
		}

		return nil
	}

	// Handle long version flag
	if configs.ShowLongVersion {
		var sb strings.Builder

		if version.Version == "0.0.0" {
			if info, ok := debug.ReadBuildInfo(); ok {
				fmt.Fprintf(&sb, "%s version: %s, ", appName, info.Main.Version)
				fmt.Fprintf(&sb, "Git commit: %s, ", info.Main.Sum)
				fmt.Fprintf(&sb, "Go version: %s\n", info.GoVersion)
			} else {
				fmt.Fprintf(&sb, "%s version: %s, ", appName, version.Version)
				fmt.Fprintf(&sb, "Build date: %s, ", version.BuildDate)
				fmt.Fprintf(&sb, "Build user: %s, ", version.BuildUser)
				fmt.Fprintf(&sb, "Git commit: %s, ", version.GitCommit)
				fmt.Fprintf(&sb, "Git branch: %s, ", version.GitBranch)
				fmt.Fprintf(&sb, "Go version: %s\n", version.GoVersion)
			}
		} else {
			fmt.Fprintf(&sb, "%s version: %s, ", appName, version.Version)
			fmt.Fprintf(&sb, "Build date: %s, ", version.BuildDate)
			fmt.Fprintf(&sb, "Build user: %s, ", version.BuildUser)
			fmt.Fprintf(&sb, "Git commit: %s, ", version.GitCommit)
			fmt.Fprintf(&sb, "Git branch: %s, ", version.GitBranch)
			fmt.Fprintf(&sb, "Go version: %s\n", version.GoVersion)
		}

		fmt.Print(sb.String())

		return nil
	}

	// Handle help flag
	if configs.ShowHelp {
		flag.Usage()
		return fmt.Errorf("help displayed")
	}

	// Handle debug flag - set log level to debug if enabled
	if configs.Log.Debug.Value {
		configs.Log.Level.Value = "debug"
	}

	return nil
}

// setupLogger configures the global logger based on the given LogConfig
func setupLogger(logConfig *config.LogConfig) {
	var logLevel slog.Level

	switch logConfig.Level.Value {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	case cslog.LogLevelTrace.String():
		logLevel = cslog.LogLevelTrace
	case cslog.LogLevelFatal.String():
		logLevel = cslog.LogLevelFatal
	default:
		logLevel = slog.LevelInfo
	}

	// Create logger options
	opts := &slog.HandlerOptions{
		Level:     logLevel,
		AddSource: logConfig.AddSource.Value,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.LevelKey {
				level := a.Value.Any().(slog.Level)
				switch level {
				case cslog.LogLevelTrace:
					a.Value = slog.StringValue("TRACE")
				case cslog.LogLevelFatal:
					a.Value = slog.StringValue("FATAL")
				}
			}

			return a
		},
	}

	// Create handler based on format
	var handler slog.Handler
	switch logConfig.Format.Value {
	case "json":
		handler = slog.NewJSONHandler(logConfig.Output.Value, opts)
	case "text":
		handler = slog.NewTextHandler(logConfig.Output.Value, opts)
	default:
		handler = slog.NewTextHandler(logConfig.Output.Value, opts)
	}

	// Set the default logger
	logger := slog.New(handler)
	logger = logger.With(
		slog.String("application", appName),
		slog.String("version", version.Version),
	)

	slog.SetDefault(logger)
}
