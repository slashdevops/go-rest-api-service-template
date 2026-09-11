package middleware

import (
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/respond"
)

// Tracing starts the span that represents the whole request.
//
// # Why the server span belongs here and not in the handler
//
// It used to start inside the handler, in o11y.SetupTraceHTTP. Everything above
// the handler therefore ran outside any span, and two things followed:
//
//   - The request log line could not carry a trace id. Logging runs above the
//     handler and a context does not flow back UP from next.ServeHTTP, so by
//     the time the line is written the handler's span is gone. That is the
//     whole reason logs and traces could not be joined.
//   - A request refused before the handler -- by the rate limiter, by the body
//     limit, by authentication -- produced no span at all. Those are precisely
//     the requests somebody is looking for in a trace view.
//
// Starting it here fixes both: everything below is inside the span, and
// everything above it (RequestID, Recovery, SecurityHeaders) is deliberately
// outside, because a panic must be recovered whether or not tracing is
// working.
//
// # The span is named twice, on purpose
//
// http.route must be the ROUTE and never the path: `/users/{id}` is one span
// name, `/users/019822af-...` is one per user and turns a trace backend into a
// cardinality incident.
//
// The route is not known on the way in. Go's ServeMux assigns Request.Pattern
// while it routes, which happens below here, so the span opens under a
// placeholder and is renamed on the way out, once the mux has decided. The
// rename reads the pattern off the request this middleware PASSED DOWN rather
// than the one it received -- the mux sets it on the request it was handed, and
// this middleware hands down a context-carrying copy.
//
// A request that never reaches a handler keeps the placeholder, and that is
// correct: there is no route, because nothing matched or nothing was allowed
// to. TestRequestLogCarriesTheRouteAndTheTimings fails if a middleware added
// in between starts cloning the request, which would break the rename
// silently.
func Tracing(tracer trace.Tracer, clientIP *ClientIPResolver) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			ctx, span := tracer.Start(r.Context(),
				"HTTP "+r.Method,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					semconv.HTTPRequestMethodKey.String(r.Method),
					semconv.URLPath(r.URL.Path),
					semconv.UserAgentOriginal(r.UserAgent()),
				),
			)
			defer span.End()

			// The resolved client address, not RemoteAddr: behind a proxy the
			// peer is the proxy, and a trace that says so for every request is
			// telling you nothing you did not already know.
			if clientIP != nil {
				span.SetAttributes(semconv.ClientAddress(clientIP.ClientIP(r)))
			}

			if id := respond.RequestIDFrom(ctx); id != "" {
				span.SetAttributes(attribute.String("request.id", id))
			}

			wrapped := newWrappedResponseWriter(w)

			routed := r.WithContext(ctx)

			next.ServeHTTP(wrapped, routed)

			// Now the mux has chosen, so the span can say what it served.
			if route := routed.Pattern; route != "" {
				span.SetName(route)
				span.SetAttributes(semconv.HTTPRoute(route))
			}

			span.SetAttributes(
				semconv.HTTPResponseStatusCode(wrapped.status),
				attribute.Int64("http.response.body.size", wrapped.written),
				attribute.Int64("http.server.request.duration_ms", time.Since(start).Milliseconds()),
			)

			// Only 5xx marks the span as an error. A 4xx is the server working
			// correctly and telling a caller so; marking those Error makes the
			// error rate in a trace backend a measure of how often clients send
			// bad requests, which is not what anybody reads it as.
			if wrapped.status >= http.StatusInternalServerError {
				span.SetStatus(codes.Error, http.StatusText(wrapped.status))
			} else {
				span.SetStatus(codes.Ok, "")
			}
		})
	}
}
