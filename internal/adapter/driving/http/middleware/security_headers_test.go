package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSecurityHeaders(t *testing.T) {
	t.Parallel()

	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	t.Run("api_json_gets_every_header", func(t *testing.T) {
		t.Parallel()

		h := SecurityHeaders(SecurityHeadersOpts{HSTS: true, HSTSMaxAge: 2 * time.Hour})(ok)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/roles", nil))

		for name, want := range map[string]string{
			"X-Content-Type-Options":       "nosniff",
			"X-Frame-Options":              "DENY",
			"Referrer-Policy":              "no-referrer",
			"Cross-Origin-Resource-Policy": "same-origin",
			"Cache-Control":                "no-store",
			"Content-Security-Policy":      "default-src 'none'; frame-ancestors 'none'",
			"Strict-Transport-Security":    "max-age=7200; includeSubDomains",
		} {
			if got := rec.Header().Get(name); got != want {
				t.Errorf("%s = %q, want %q", name, got, want)
			}
		}
	})

	t.Run("no_hsts_unless_tls_or_proxy_says_so", func(t *testing.T) {
		t.Parallel()

		h := SecurityHeaders(SecurityHeadersOpts{})(ok)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/roles", nil))

		if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
			t.Errorf("HSTS sent over plain HTTP: %q", got)
		}
	})

	t.Run("a_document_keeps_the_safe_headers_and_loses_no_store_and_csp", func(t *testing.T) {
		t.Parallel()

		h := SecurityHeaders(SecurityHeadersOpts{
			IsDocument: func(r *http.Request) bool { return strings.HasPrefix(r.URL.Path, "/swagger/") },
		})(ok)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil))

		if rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("X-Frame-Options") != "DENY" {
			t.Error("a document lost the headers that never break a page")
		}

		if rec.Header().Get("Cache-Control") != "" || rec.Header().Get("Content-Security-Policy") != "" {
			t.Error("a document got no-store or the API CSP, which would break the swagger UI")
		}
	})

	t.Run("headers_are_present_on_an_early_exit", func(t *testing.T) {
		t.Parallel()

		// Set before the handler runs, so a handler that writes nothing but a
		// status -- or a middleware inside this one refusing the request --
		// still answers with them.
		h := SecurityHeaders(SecurityHeadersOpts{})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/roles", nil))

		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Error("a 429 went out without no-store")
		}
	})
}
