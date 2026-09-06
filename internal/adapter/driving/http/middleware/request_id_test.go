package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/respond"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

func TestRequestID(t *testing.T) {
	t.Parallel()

	var seen string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = respond.RequestIDFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/roles", nil)
	// A caller-supplied id is not trusted: a log full of chosen ids is worse
	// than one with only ours.
	req.Header.Set(RequestIDHeader, "attacker-chosen")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	got := rec.Header().Get(RequestIDHeader)
	if got == "" || got == "attacker-chosen" || got != seen {
		t.Fatalf("header %q, context %q; want one minted id in both", got, seen)
	}

	id, err := uuid.Parse(got)
	if err != nil || !domain.IsUUIDV7(id) {
		t.Fatalf("request id %q is not a v7 uuid: %v", got, err)
	}
}
