package middleware

import (
	"errors"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/respond"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMaxBody(t *testing.T) {
	t.Parallel()

	limits := BodyLimits{
		Default: 16,
		Large:   64,
		IsLarge: func(r *http.Request) bool { return strings.HasSuffix(r.URL.Path, "/ingest") },
	}

	// readAll reports how the handler saw the body: bytes read, and the error.
	var seen struct {
		n   int
		err error
	}

	handler := MaxBody(limits)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		seen.n, seen.err = len(b), err
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("declared_length_over_the_bound_is_413_before_any_read", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/roles", strings.NewReader(strings.Repeat("x", 100)))
		rec := httptest.NewRecorder()
		seen.n = -1
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status = %d, want 413", rec.Code)
		}

		if seen.n != -1 {
			t.Error("the handler ran; the body must be refused before it is read")
		}

		if !strings.Contains(rec.Body.String(), respond.BodyTooLargeMessage) {
			t.Errorf("body = %s, want the fixed message", rec.Body.String())
		}
	})

	t.Run("undeclared_length_is_cut_at_the_bound", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/roles", strings.NewReader(strings.Repeat("x", 100)))
		req.ContentLength = -1 // chunked: the server does not know the size up front
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if _, ok := errors.AsType[*http.MaxBytesError](seen.err); !ok {
			t.Fatalf("handler read %d bytes with err %v; want a MaxBytesError at the bound", seen.n, seen.err)
		}
	})

	t.Run("ingest_gets_the_large_bound", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/projects/x/embeddings/ingest", strings.NewReader(strings.Repeat("x", 40)))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK || seen.n != 40 {
			t.Fatalf("status = %d, read %d; a 40-byte ingest body is under the 64-byte large bound", rec.Code, seen.n)
		}
	})

	t.Run("no_body_passes", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/roles", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
	})
}
