package handler

import (
	"context"
	"errors"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/middleware"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
)

// pgxConnectError is the real text this service handed to unauthenticated
// callers, captured from the running API with Postgres stopped.
const pgxConnectError = "failed to connect to `user=username database=go-rest-api-service-template`:\n" +
	"\t[::1]:5432 (localhost): tls error: EOF\n" +
	"\t127.0.0.1:5432 (localhost): tls error: EOF"

// secrets are the substrings of that error a caller must never be able to read
// back. Each one is a separate fact about the deployment: who the service
// connects as, what the database is called, and where it lives.
var secrets = []string{"user=username", "database=go-rest-api-service-template", "5432", "tls error"}

type failingHealthService struct{ err error }

func (s *failingHealthService) HealthCheck(context.Context) (payload.Health, error) {
	return payload.Health{}, s.err
}

func (s *failingHealthService) GetDetailedHealth(context.Context) (payload.DetailedHealth, error) {
	return payload.DetailedHealth{}, s.err
}

// TestHealthFailureDoesNotDiscloseTheReason pins that a failing health check
// says one thing.
//
// The health endpoints are registered before the authentication chain -- a
// probe cannot hold a token -- so whatever they write is world-readable to
// anyone who can reach the port. They used to write the error verbatim, which
// with Postgres down published the database user, the database name and the
// addresses the pool tried. Measured against the running service:
//
//	GET /health/status -> 500
//	{"message":"failed to connect to `user=username database=go-rest-api-service-template`: ..."}
//
// The reason belongs on the span and in an ERROR log, which is where an
// operator reads it from anyway.
func TestHealthFailureDoesNotDiscloseTheReason(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	newTestHealthHandler(t, &failingHealthService{err: errors.New(pgxConnectError)}).RegisterRoutes(mux, middleware.Chain(), middleware.Chain())

	for _, path := range []string{"/health/status", "/health/detailed"} {
		t.Run(strings.TrimPrefix(path, "/health/"), func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
			}

			body := rec.Body.String()
			for _, secret := range secrets {
				if strings.Contains(body, secret) {
					t.Errorf("response discloses %q to an unauthenticated caller:\n%s", secret, body)
				}
			}

			if !strings.Contains(body, healthCheckFailedMessage) {
				t.Errorf("response does not carry the fixed message %q:\n%s", healthCheckFailedMessage, body)
			}
		})
	}
}
