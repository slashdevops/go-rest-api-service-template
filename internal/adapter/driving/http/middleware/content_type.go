package middleware

import (
	"mime"
	"net/http"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/respond"
)

// UnsupportedMediaTypeMessage is the 415 body.
const UnsupportedMediaTypeMessage = "Content-Type must be application/json"

// RequireJSONBody refuses a request that carries a body without declaring
// it as JSON.
//
// Every route that reads a body decodes JSON, and none of them checked the
// declared type -- text/plain, a form, anything at all was decoded as JSON.
// That matters for two reasons. A browser sends a cross-origin form post
// without a preflight, and only because form content types are "simple";
// requiring application/json makes the preflight, and so the CORS policy,
// apply to every write. And a client that sends the wrong type is told so at
// the door, in one wording, instead of by whatever the decoder makes of it.
//
// It keys on the presence of a body, not on the method: a DELETE with a JSON
// body (bulk delete) is a body like any other, and a POST without one (logout)
// is not asked to describe what it did not send.
func RequireJSONBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hasBody(r) {
			mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || mediaType != "application/json" {
				respond.WriteJSONMessage(w, r, http.StatusUnsupportedMediaType, UnsupportedMediaTypeMessage)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// hasBody reports whether the request declares a body: a positive
// Content-Length, or an unknown one (chunked), which Go reports as -1.
func hasBody(r *http.Request) bool {
	return r.ContentLength > 0 || (r.ContentLength == -1 && len(r.TransferEncoding) > 0)
}
