package middleware

import "net/http"

// Thanks to:
// - https://github.com/denpeshkov/greenlight/blob/c68f5a2111adcd5b1a65a06595acc93a02b6380e/internal/http/middleware.go#L16-L71
// - https://github.com/golang/go/issues/65648
type wrappedResponseWriter struct {
	http.ResponseWriter
	status int
}

// newWrappedResponseWriter creates a new statusResponseWriter.
func newWrappedResponseWriter(w http.ResponseWriter) *wrappedResponseWriter {
	// WriteHeader() is not called if our response implicitly returns 200 OK, so we default to that status code.
	return &wrappedResponseWriter{
		ResponseWriter: w,
		status:         http.StatusOK,
	}
}

func (w *wrappedResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// Unwrap is used by a [http.ResponseController].
func (w *wrappedResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
