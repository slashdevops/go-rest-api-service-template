//go:build unit

package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdkTrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// chain builds the observability part of the real chain, in the real order.
func chain(t *testing.T, tracer trace.Tracer, handler http.Handler) http.Handler {
	t.Helper()

	return RequestID(
		OtelTextMapPropagation(
			Tracing(tracer, nil)(
				Logging(nil)(handler),
			),
		),
	)
}

func recorder(t *testing.T) (trace.Tracer, *tracetest.SpanRecorder) {
	t.Helper()

	sr := tracetest.NewSpanRecorder()
	tp := sdkTrace.NewTracerProvider(sdkTrace.WithSpanProcessor(sr))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	return tp.Tracer("test"), sr
}

// The point of moving the span above the handler: the request line is written
// INSIDE the span, so the bridge can stamp a trace id on it. Everything else
// in this change is detail.
func TestRequestLogIsWrittenInsideTheServerSpan(t *testing.T) {
	tracer, _ := recorder(t)

	var logged trace.SpanContext

	previous := slog.Default()
	slog.SetDefault(slog.New(spanCapturingHandler{seen: &logged}))
	t.Cleanup(func() { slog.SetDefault(previous) })

	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	chain(t, tracer, mux).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/users/42", nil))

	if !logged.IsValid() {
		t.Fatal("the request line carried no span context; logs and traces cannot be joined")
	}

	if !logged.TraceID().IsValid() {
		t.Error("the request line carried no trace id")
	}
}

// http.route must be the route and never the path, or a trace backend gets one
// span name per id.
func TestServerSpanIsNamedForTheRouteNotThePath(t *testing.T) {
	tracer, sr := recorder(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	chain(t, tracer, mux).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/users/019822af-b448-73fb-89a1-447e8f8d1cde", nil))

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}

	name := spans[0].Name()
	if strings.Contains(name, "019822af") {
		t.Errorf("span name %q contains the id; that is one span name per user", name)
	}

	if !strings.Contains(name, "{id}") {
		t.Errorf("span name = %q, want the route pattern", name)
	}
}

// A request nothing routes to still produces a span. Those are exactly the
// requests somebody goes looking for.
func TestUnroutedRequestStillProducesASpan(t *testing.T) {
	tracer, sr := recorder(t)

	mux := http.NewServeMux()

	rec := httptest.NewRecorder()
	chain(t, tracer, mux).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nothing-here", nil))

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1 even though nothing matched", len(spans))
	}

	if spans[0].Name() != "HTTP GET" {
		t.Errorf("span name = %q, want the placeholder: there is no route, because nothing matched", spans[0].Name())
	}
}

// 4xx is the server working correctly. Marking it Error makes the error rate in
// a trace backend measure how often clients send bad requests.
func TestOnlyServerErrorsMarkTheSpanAsFailed(t *testing.T) {
	for _, tt := range []struct {
		name    string
		status  int
		wantErr bool
	}{
		{"ok", http.StatusOK, false},
		{"bad_request", http.StatusBadRequest, false},
		{"unauthorized", http.StatusUnauthorized, false},
		{"too_many_requests", http.StatusTooManyRequests, false},
		{"internal_error", http.StatusInternalServerError, true},
		{"bad_gateway", http.StatusBadGateway, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tracer, sr := recorder(t)

			h := chain(t, tracer, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))

			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))

			spans := sr.Ended()
			if len(spans) != 1 {
				t.Fatalf("got %d spans, want 1", len(spans))
			}

			isErr := spans[0].Status().Code.String() == "Error"
			if isErr != tt.wantErr {
				t.Errorf("status %d gave span error=%v, want %v", tt.status, isErr, tt.wantErr)
			}
		})
	}
}

// spanCapturingHandler records the span context of the first record it sees.
type spanCapturingHandler struct {
	seen *trace.SpanContext
}

func (h spanCapturingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h spanCapturingHandler) Handle(ctx context.Context, _ slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		*h.seen = sc
	}

	return nil
}
func (h spanCapturingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h spanCapturingHandler) WithGroup(string) slog.Handler      { return h }

// attrCapturingHandler records the attributes of the first record it sees.
type attrCapturingHandler struct {
	attrs map[string]string
}

func (h attrCapturingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h attrCapturingHandler) Handle(_ context.Context, r slog.Record) error {
	r.Attrs(func(a slog.Attr) bool {
		h.attrs[a.Key] = a.Value.String()

		return true
	})

	return nil
}
func (h attrCapturingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h attrCapturingHandler) WithGroup(string) slog.Handler      { return h }

// The request line has to carry the ROUTE, not just the path, or "which
// endpoint is slow" cannot be answered by grouping.
//
// Whether it can is not obvious from reading the code: net/http's ServeMux
// assigns the matched pattern to the request it was HANDED, and a middleware
// above it holds that same request only if nothing in between cloned it. This
// test is the empirical answer, and it fails if a future middleware starts
// cloning.
func TestRequestLogCarriesTheRouteAndTheTimings(t *testing.T) {
	tracer, _ := recorder(t)

	captured := attrCapturingHandler{attrs: map[string]string{}}

	previous := slog.Default()
	slog.SetDefault(slog.New(captured))
	t.Cleanup(func() { slog.SetDefault(previous) })

	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)

		if _, err := w.Write([]byte(`{"ok":true}`)); err != nil {
			t.Errorf("write: %v", err)
		}
	})

	rec := httptest.NewRecorder()
	chain(t, tracer, mux).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/users/42", nil))

	if got := captured.attrs["route"]; got != "GET /users/{id}" {
		t.Errorf("route = %q, want the mux pattern; a path cannot be grouped", got)
	}

	if got := captured.attrs["path"]; got != "/users/42" {
		t.Errorf("path = %q, want the concrete path", got)
	}

	if got := captured.attrs["status"]; got != "200" {
		t.Errorf("status = %q", got)
	}

	if got := captured.attrs["bytes"]; got != "11" {
		t.Errorf("bytes = %q, want 11: the response body was counted", got)
	}

	if _, ok := captured.attrs["duration_ms"]; !ok {
		t.Error("the request line has no duration; that is the first question asked of an access log")
	}

	if captured.attrs["request_id"] == "" {
		t.Error("the request line has no request id")
	}
}

// A request nothing matched still gets a line, and the route says so rather
// than being empty.
func TestRequestLogSaysUnmatchedWhenNothingRouted(t *testing.T) {
	tracer, _ := recorder(t)

	captured := attrCapturingHandler{attrs: map[string]string{}}

	previous := slog.Default()
	slog.SetDefault(slog.New(captured))
	t.Cleanup(func() { slog.SetDefault(previous) })

	rec := httptest.NewRecorder()
	chain(t, tracer, http.NewServeMux()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nothing-here", nil))

	if got := captured.attrs["route"]; got != "unmatched" {
		t.Errorf("route = %q, want \"unmatched\"", got)
	}
}
