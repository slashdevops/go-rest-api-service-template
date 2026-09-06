package middleware

import (
	"net/http"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/respond"
)

// BodyLimits configures [MaxBody].
type BodyLimits struct {
	// IsLarge picks the requests that get the Large bound: bulk ingest.
	IsLarge func(r *http.Request) bool
	// Default bounds every other request body, in bytes.
	Default int64
	// Large bounds the bodies IsLarge selects, in bytes.
	Large int64
}

// MaxBody bounds what a handler can read from the request body.
//
// There was no bound. Every handler decodes its body with encoding/json
// straight from r.Body, and ReadTimeout is off by design (bulk ingest over a
// slow link is legitimate), so one request could allocate whatever it sent:
// measured 2026-09-06, a 200 MiB body to /auth/login took the process from
// 52 MiB to 674 MiB RSS before the 400.
//
// Two mechanisms, because clients come in two shapes. A request that declares
// Content-Length above the bound is refused with 413 before a byte of body is
// read. A request that does not declare it (chunked) is wrapped in
// [http.MaxBytesReader], which stops the read at the bound and makes the
// decoder fail; that failure surfaces as the handler's decode error, and the
// connection is closed so the client cannot keep sending. Handlers that want
// to answer 413 for that case match [http.MaxBytesError].
func MaxBody(limits BodyLimits) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body == nil || r.Body == http.NoBody {
				next.ServeHTTP(w, r)
				return
			}

			limit := limits.Default
			if limits.IsLarge != nil && limits.IsLarge(r) {
				limit = limits.Large
			}

			if r.ContentLength > limit {
				respond.WriteJSONMessage(w, r, http.StatusRequestEntityTooLarge, respond.BodyTooLargeMessage)
				return
			}

			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}
