package middleware

import "net/http"

// Thanks to:
// - https://github.com/denpeshkov/greenlight/blob/c68f5a2111adcd5b1a65a06595acc93a02b6380e/internal/http/middleware.go#L16-L71
// - https://github.com/golang/go/issues/65648
type wrappedResponseWriter struct {
	http.ResponseWriter
	written int64
	status  int
}

// newWrappedResponseWriter creates a new statusResponseWriter.
//
// An already-wrapped writer is returned as it is rather than wrapped again.
// Two middlewares in the chain want the status and the size -- Tracing, to put
// them on the server span, and Logging, to put them in the request line -- and
// wrapping twice would work but would add a layer to every Write, and would
// give each of them its own byte count of the same response.
func newWrappedResponseWriter(w http.ResponseWriter) *wrappedResponseWriter {
	if wrapped, ok := w.(*wrappedResponseWriter); ok {
		return wrapped
	}

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

// Write counts what the handler produced.
//
// The size is worth having on the request line and the span: a slow endpoint
// that returns a megabyte is a different problem from a slow endpoint that
// returns nothing, and the two are indistinguishable from a duration alone.
func (w *wrappedResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.written += int64(n)

	return n, err
}

// Unwrap is used by a [http.ResponseController].
func (w *wrappedResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
