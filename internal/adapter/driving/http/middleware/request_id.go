package middleware

import (
	"net/http"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/respond"
)

// RequestIDHeader carries the id on every response.
const RequestIDHeader = "X-Request-ID"

// RequestID mints one id per request, sends it back in X-Request-ID, and
// puts it on the context for the log line and the error body.
//
// It is minted here rather than taken from the request: an inbound
// X-Request-ID is whatever the caller wrote, and a log an attacker can
// fill with chosen ids is worse than one with only ours. The id is a v7
// uuid, so it sorts by time in a log.
//
// This is the outermost middleware, above Recovery: a recovered panic is
// exactly the 500 an operator has to find, and its id must exist by then.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := uuid.NewV7().String()
		w.Header().Set(RequestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(respond.WithRequestID(r.Context(), id)))
	})
}
