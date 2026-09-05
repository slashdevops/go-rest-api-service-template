//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"uuid"

	"go.uber.org/mock/gomock"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/middleware"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
	mocks "github.com/slashdevops/go-rest-api-service-template/mocks/handler"
)

func newTokenLifetimesHandler(t *testing.T) (*TokenLifetimesHandler, *mocks.MockTokenLifetimes) {
	t.Helper()

	ctrl := gomock.NewController(t)
	svc := mocks.NewMockTokenLifetimes(ctrl)

	ctx := t.Context()
	ot := &o11y.OpenTelemetry{
		Traces:  o11y.NewOpenTelemetryTracer(ctx, &o11y.OpenTelemetryTracerConfig{Name: "test"}),
		Metrics: o11y.NewOpenTelemetryMeter(ctx, &o11y.OpenTelemetryMeterConfig{Name: "test"}),
	}

	h, err := NewTokenLifetimesHandler(TokenLifetimesHandlerConf{Service: svc, OT: ot})
	if err != nil {
		t.Fatalf("NewTokenLifetimesHandler: %v", err)
	}

	return h, svc
}

func withCaller(r *http.Request, id uuid.UUID) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), middleware.JwtClaims, map[string]any{"sub": id.String()}))
}

func storedLifetimes() *domain.TokenLifetimes {
	return &domain.TokenLifetimes{
		AccessTokenDuration:  5 * time.Minute,
		RefreshTokenDuration: 24 * time.Hour,
		UpdatedAt:            time.Date(2026, 9, 5, 10, 12, 0, 0, time.UTC),
	}
}

// GET carries the bounds and defaults beside the value, so a client never
// hardcodes a number the server validates against.
func TestTokenLifetimesGetCarriesBoundsAndDefaults(t *testing.T) {
	t.Parallel()

	h, svc := newTokenLifetimesHandler(t)
	svc.EXPECT().Get(gomock.Any()).Return(storedLifetimes(), nil)

	rec := httptest.NewRecorder()
	h.get(rec, httptest.NewRequest(http.MethodGet, "/auth/token_lifetimes", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var body payload.TokenLifetimesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body.AccessTokenDuration != "5m0s" || body.RefreshTokenDuration != "24h0m0s" {
		t.Fatalf("durations = %s / %s, want Go duration strings", body.AccessTokenDuration, body.RefreshTokenDuration)
	}

	if body.Bounds.AccessTokenDuration.Min != "2m0s" || body.Bounds.AccessTokenDuration.Max != "48h0m0s" {
		t.Fatalf("access bounds = %+v", body.Bounds.AccessTokenDuration)
	}

	if body.Bounds.RefreshTokenDuration.Min != "12h0m0s" || body.Bounds.RefreshTokenDuration.Max != "168h0m0s" {
		t.Fatalf("refresh bounds = %+v", body.Bounds.RefreshTokenDuration)
	}

	if body.Defaults.AccessTokenDuration != "5m0s" || body.Defaults.RefreshTokenDuration != "24h0m0s" {
		t.Fatalf("defaults = %+v", body.Defaults)
	}

	if body.UpdatedBy != nil {
		t.Fatal("the seeded row has no updated_by, and the field must be absent rather than the nil uuid")
	}
}

// A missing row is a 500, never a 404: the migration seeds it and the service
// refuses to start without it.
func TestTokenLifetimesGetMissingRowIs500(t *testing.T) {
	t.Parallel()

	h, svc := newTokenLifetimesHandler(t)
	svc.EXPECT().Get(gomock.Any()).Return(nil, &domain.TokenLifetimesNotFoundError{})

	rec := httptest.NewRecorder()
	h.get(rec, httptest.NewRequest(http.MethodGet, "/auth/token_lifetimes", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

// PUT attributes the change to the verified caller, never to anything in the
// body, and answers with the same shape GET does.
func TestTokenLifetimesUpdateAttributesToTheVerifiedCaller(t *testing.T) {
	t.Parallel()

	h, svc := newTokenLifetimesHandler(t)
	caller := uuid.NewV7()

	svc.EXPECT().Update(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *domain.UpdateTokenLifetimesInput) (*domain.TokenLifetimes, error) {
			if in.UpdatedBy != caller {
				t.Errorf("UpdatedBy = %s, want the token's subject %s", in.UpdatedBy, caller)
			}

			if in.AccessTokenDuration != 10*time.Minute || in.RefreshTokenDuration != 72*time.Hour {
				t.Errorf("parsed durations = %v / %v", in.AccessTokenDuration, in.RefreshTokenDuration)
			}

			return &domain.TokenLifetimes{
				AccessTokenDuration: in.AccessTokenDuration, RefreshTokenDuration: in.RefreshTokenDuration,
				UpdatedBy: in.UpdatedBy, UpdatedAt: time.Now(),
			}, nil
		})

	body := `{"access_token_duration":"10m","refresh_token_duration":"72h"}`
	req := withCaller(httptest.NewRequest(http.MethodPut, "/auth/token_lifetimes", strings.NewReader(body)), caller)

	rec := httptest.NewRecorder()
	h.update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var out payload.TokenLifetimesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if out.UpdatedBy == nil || *out.UpdatedBy != caller {
		t.Fatalf("updated_by = %v, want %s", out.UpdatedBy, caller)
	}
}

// A duration that does not parse is a 400 in this package's own words -- not
// time.ParseDuration's, which would become part of the API contract.
func TestTokenLifetimesUpdateRejectsAnUnparseableDurationBeforeTheService(t *testing.T) {
	t.Parallel()

	h, _ := newTokenLifetimesHandler(t) // no EXPECT: the service must not be reached

	body := `{"access_token_duration":"ten minutes","refresh_token_duration":"72h"}`
	req := withCaller(httptest.NewRequest(http.MethodPut, "/auth/token_lifetimes", strings.NewReader(body)), uuid.NewV7())

	rec := httptest.NewRecorder()
	h.update(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}

	if strings.Contains(rec.Body.String(), "time: ") {
		t.Fatalf("the library's error text leaked into the response: %s", rec.Body.String())
	}

	if !strings.Contains(rec.Body.String(), domain.FieldAccessTokenDuration) {
		t.Fatalf("the response must name the field: %s", rec.Body.String())
	}
}

// The service's validation errors are 400s; anything else is a 500.
func TestTokenLifetimesUpdateMapsServiceErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want int
	}{
		{"validation", &domain.ValidationErrors{Errors: []domain.ValidationError{{Field: domain.FieldRefreshTokenDuration, Message: "x"}}}, http.StatusBadRequest},
		{"invalid_input", &domain.InvalidInputError{Message: "x"}, http.StatusBadRequest},
		{"store_fault", errors.New("database is away"), http.StatusInternalServerError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h, svc := newTokenLifetimesHandler(t)
			svc.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil, tc.err)

			body := `{"access_token_duration":"10m","refresh_token_duration":"72h"}`
			req := withCaller(httptest.NewRequest(http.MethodPut, "/auth/token_lifetimes", strings.NewReader(body)), uuid.NewV7())

			rec := httptest.NewRecorder()
			h.update(rec, req)

			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

// Without verified claims on the context there is nobody to attribute the
// change to. The middleware always supplies them, so this is a 500 -- a wiring
// fault -- not a 401.
func TestTokenLifetimesUpdateWithoutClaimsIs500(t *testing.T) {
	t.Parallel()

	h, _ := newTokenLifetimesHandler(t)

	body := `{"access_token_duration":"10m","refresh_token_duration":"72h"}`
	rec := httptest.NewRecorder()
	h.update(rec, httptest.NewRequest(http.MethodPut, "/auth/token_lifetimes", strings.NewReader(body)))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}
