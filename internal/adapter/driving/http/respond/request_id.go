package respond

import "context"

type requestIDKey struct{}

// WithRequestID returns a context carrying the request id. Set once per
// request by the RequestID middleware.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestIDFrom returns the request id set by the middleware, or "".
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}
