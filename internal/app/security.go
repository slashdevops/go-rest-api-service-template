package app

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/middleware"
)

// securityHeadersOpts decides the response-header posture from configuration.
//
// HSTS is sent when TLS terminates in this process, and otherwise only when
// the operator says a proxy terminates it. Neither posture is visible from a
// request, so the choice is logged at startup.
func (a *App) securityHeadersOpts() middleware.SecurityHeadersOpts {
	cfg := a.configs.HTTPServer
	hsts := cfg.TLSEnabled.Value || cfg.HSTSEnabled.Value

	switch {
	case cfg.TLSEnabled.Value:
		slog.Info("Strict-Transport-Security is sent: TLS terminates in this process", "max_age", cfg.HSTSMaxAge.Value)
	case cfg.HSTSEnabled.Value:
		slog.Info("Strict-Transport-Security is sent: http.server.hsts.enabled says a proxy terminates TLS", "max_age", cfg.HSTSMaxAge.Value)
	default:
		slog.Warn("Strict-Transport-Security is not sent; set http.server.hsts.enabled when a proxy terminates TLS in front of this service")
	}

	return middleware.SecurityHeadersOpts{
		HSTS:       hsts,
		HSTSMaxAge: cfg.HSTSMaxAge.Value,
		// The swagger UI is the one page this server serves. The path is
		// seen after the API prefix has been stripped.
		IsDocument: func(r *http.Request) bool {
			return strings.HasPrefix(r.URL.Path, "/swagger/")
		},
	}
}

// bodyLimits decides the request body bounds from configuration. The large
// bound is reserved for a bulk route; the template has none, so it applies
// to nothing until a service adds one and names it here.
func (a *App) bodyLimits() middleware.BodyLimits {
	cfg := a.configs.HTTPServer

	return middleware.BodyLimits{
		Default: int64(cfg.MaxBodyBytes.Value),
		Large:   int64(cfg.MaxBodyBytesIngest.Value),
		IsLarge: func(r *http.Request) bool {
			return r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/embeddings/ingest")
		},
	}
}
