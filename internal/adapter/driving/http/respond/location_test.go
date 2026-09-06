package respond

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLocationFor(t *testing.T) {
	// Not parallel: the base URL is package state.
	t.Cleanup(func() { SetPublicBaseURL("") })

	req := httptest.NewRequest(http.MethodPost, "/api/v1/roles?x=1", nil)
	req.Header.Set("Origin", "http://evil.example")

	SetPublicBaseURL("")
	if got, want := LocationFor(req, "abc"), "/api/v1/roles/abc"; got != want {
		t.Errorf("path reference: got %q, want %q (the query must be dropped and Origin ignored)", got, want)
	}

	if got, want := LocationFor(req), "/api/v1/roles"; got != want {
		t.Errorf("self: got %q, want %q", got, want)
	}

	if got, want := LocationFor(req, "p1", "users"), "/api/v1/roles/p1/users"; got != want {
		t.Errorf("nested: got %q, want %q", got, want)
	}

	SetPublicBaseURL("https://api.example.com/")
	if got, want := LocationFor(req, "abc"), "https://api.example.com/api/v1/roles/abc"; got != want {
		t.Errorf("absolute: got %q, want %q", got, want)
	}

	rec := httptest.NewRecorder()
	SetLocation(rec, req, "abc")
	if got := rec.Header().Get("Location"); got != "https://api.example.com/api/v1/roles/abc" {
		t.Errorf("SetLocation wrote %q", got)
	}
}
