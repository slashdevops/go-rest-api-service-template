package middleware

import (
	"fmt"
	"net/http"
	"time"
)

// SecurityHeadersOpts configures [SecurityHeaders].
type SecurityHeadersOpts struct {
	// IsDocument reports a request that serves a page rather than API JSON --
	// the swagger UI. A page keeps the headers that never break one and loses
	// the two that would: no-store, which would make its assets uncacheable,
	// and a CSP of "default-src 'none'", which would block its own scripts.
	IsDocument func(r *http.Request) bool
	// HSTSMaxAge is the Strict-Transport-Security max-age; ignored unless HSTS.
	HSTSMaxAge time.Duration
	// HSTS sends Strict-Transport-Security. True when TLS terminates in this
	// process, or when the operator says a proxy does.
	HSTS bool
}

// SecurityHeaders sets the response headers an API should carry on every
// answer, before the handler runs so that an early exit gets them too.
//
// There were none. The API sets Content-Type: application/json on every
// response, and that is all a browser was told. What each header closes:
//
//   - X-Content-Type-Options: nosniff -- a browser never second-guesses the
//     declared type, so a JSON body that happens to contain markup is not
//     rendered as a page.
//   - Cache-Control: no-store -- a login answer carries two tokens, a list
//     carries data the caller was authorised to see, and nothing here is
//     public. Without it a shared cache decides heuristically.
//   - Content-Security-Policy: default-src 'none'; frame-ancestors 'none' --
//     a JSON body has nothing to load and nothing to frame; the policy says
//     so, which is what makes an error page or a misconfigured proxy that
//     serves this response as HTML inert.
//   - X-Frame-Options: DENY -- the same as frame-ancestors, for the user
//     agents that predate CSP.
//   - Referrer-Policy: no-referrer -- a URL here can carry an id; it should
//     not travel to wherever a client goes next.
//   - Cross-Origin-Resource-Policy: same-origin -- a no-cors cross-origin read
//     (an <img> or <script> pointed at the API) gets nothing. A CORS request
//     is governed by the CORS middleware, not by this.
//   - Strict-Transport-Security -- only when TLS ends here or a proxy is said
//     to end it; sent over plain HTTP it is ignored, so the switch exists to
//     avoid claiming a posture the deployment does not have.
func SecurityHeaders(opts SecurityHeadersOpts) Middleware {
	hsts := fmt.Sprintf("max-age=%d; includeSubDomains", int64(opts.HSTSMaxAge.Seconds()))

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Cross-Origin-Resource-Policy", "same-origin")

			if opts.HSTS {
				h.Set("Strict-Transport-Security", hsts)
			}

			if opts.IsDocument == nil || !opts.IsDocument(r) {
				h.Set("Cache-Control", "no-store")
				h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
			}

			next.ServeHTTP(w, r)
		})
	}
}
