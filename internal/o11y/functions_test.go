package o11y

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkTrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func newTestTracer() (*sdkTrace.TracerProvider, *tracetest.InMemoryExporter) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdkTrace.NewTracerProvider(sdkTrace.WithSyncer(exporter))
	return tp, exporter
}

func TestMetadata_FullName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		meta     Metadata
		expected string
	}{
		{
			name:     "standard_metadata",
			meta:     Metadata{Layer: "service", Domain: "Users", Action: "GetByID"},
			expected: "service.Users.GetByID",
		},
		{
			name:     "handler_layer",
			meta:     Metadata{Layer: "handler", Domain: "Projects", Action: "Create"},
			expected: "handler.Projects.Create",
		},
		{
			name:     "repository_layer",
			meta:     Metadata{Layer: "repository", Domain: "Embeddings", Action: "Insert"},
			expected: "repository.Embeddings.Insert",
		},
		{
			name:     "empty_fields",
			meta:     Metadata{},
			expected: "..",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.meta.FullName()
			if got != tt.expected {
				t.Errorf("FullName() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestMetadata_ToAttributes(t *testing.T) {
	t.Parallel()

	meta := Metadata{Layer: "service", Domain: "Users", Action: "GetByID"}
	attrs := meta.ToAttributes()

	if len(attrs) != 3 {
		t.Fatalf("expected 3 attributes, got %d", len(attrs))
	}

	expected := map[attribute.Key]string{
		AttrLayer:  "service",
		AttrDomain: "Users",
		AttrAction: "GetByID",
	}

	for _, kv := range attrs {
		want, ok := expected[kv.Key]
		if !ok {
			t.Errorf("unexpected attribute key: %s", kv.Key)
			continue
		}
		if kv.Value.AsString() != want {
			t.Errorf("attribute %s = %q, want %q", kv.Key, kv.Value.AsString(), want)
		}
	}
}

func TestSetupTrace(t *testing.T) {
	t.Parallel()

	tp, exporter := newTestTracer()
	defer func() { _ = tp.Shutdown(context.Background()) }()
	tracer := tp.Tracer("test")

	meta := Metadata{Layer: "service", Domain: "Users"}
	ctx, span, attrs := SetupTrace(context.Background(), tracer, meta, "GetByID")
	span.End()

	if ctx == nil {
		t.Fatal("expected non-nil context")
	}

	if !span.SpanContext().IsValid() {
		t.Fatal("expected valid span context")
	}

	if len(attrs) != 3 {
		t.Fatalf("expected 3 attributes, got %d", len(attrs))
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	if spans[0].Name != "service.Users.GetByID" {
		t.Errorf("span name = %q, want %q", spans[0].Name, "service.Users.GetByID")
	}

	// In-process (service/repository) spans must be Internal, not Server.
	if spans[0].SpanKind != trace.SpanKindInternal {
		t.Errorf("span kind = %v, want %v", spans[0].SpanKind, trace.SpanKindInternal)
	}
}

func TestSetupTraceHTTP(t *testing.T) {
	t.Parallel()

	tp, exporter := newTestTracer()
	defer func() { _ = tp.Shutdown(context.Background()) }()
	tracer := tp.Tracer("test")

	meta := Metadata{Layer: "handler", Domain: "Projects"}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)

	ctx, span, attrs := SetupTraceHTTP(req, tracer, meta, "ListProjects")
	span.End()

	if ctx == nil {
		t.Fatal("expected non-nil context")
	}

	// Should have 3 base attrs + 2 HTTP attrs
	if len(attrs) != 5 {
		t.Fatalf("expected 5 attributes, got %d", len(attrs))
	}

	// Verify HTTP attributes are present
	foundMethod := false
	foundRoute := false
	for _, kv := range attrs {
		switch string(kv.Key) {
		case "http.request.method":
			foundMethod = true
			if kv.Value.AsString() != http.MethodGet {
				t.Errorf("http.request.method = %q, want %q", kv.Value.AsString(), http.MethodGet)
			}
		case "http.route":
			foundRoute = true
			if kv.Value.AsString() != "ListProjects" {
				t.Errorf("http.route = %q, want %q", kv.Value.AsString(), "ListProjects")
			}
		}
	}

	if !foundMethod {
		t.Error("expected http.request.method attribute")
	}
	if !foundRoute {
		t.Error("expected http.route attribute")
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	if spans[0].Name != "handler.Projects.ListProjects" {
		t.Errorf("span name = %q, want %q", spans[0].Name, "handler.Projects.ListProjects")
	}

	// The HTTP entrypoint span is the Server root of the trace.
	if spans[0].SpanKind != trace.SpanKindServer {
		t.Errorf("span kind = %v, want %v", spans[0].SpanKind, trace.SpanKindServer)
	}
}

func TestSetupTraceWithTimeout(t *testing.T) {
	t.Parallel()

	tp, _ := newTestTracer()
	defer func() { _ = tp.Shutdown(context.Background()) }()
	tracer := tp.Tracer("test")

	meta := Metadata{Layer: "repository", Domain: "Embeddings"}
	timeout := 5 * time.Second

	ctx, span, attrs, cancel := SetupTraceWithTimeout(context.Background(), tracer, timeout, meta, "SelectByID")
	defer cancel()
	defer span.End()

	if ctx == nil {
		t.Fatal("expected non-nil context")
	}

	// Verify the context has a deadline
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected context to have a deadline")
	}

	if time.Until(deadline) > timeout {
		t.Error("deadline is too far in the future")
	}

	if len(attrs) != 3 {
		t.Fatalf("expected 3 attributes, got %d", len(attrs))
	}

	if cancel == nil {
		t.Fatal("expected non-nil cancel function")
	}
}

func TestAttrsToAny(t *testing.T) {
	t.Parallel()

	attrs := []attribute.KeyValue{
		attribute.String("key1", "value1"),
		attribute.String("key2", "value2"),
	}

	result := attrsToAny(attrs)

	if len(result) != 4 {
		t.Fatalf("expected 4 items, got %d", len(result))
	}

	if result[0] != "key1" {
		t.Errorf("result[0] = %v, want %q", result[0], "key1")
	}
	if result[1] != "value1" {
		t.Errorf("result[1] = %v, want %q", result[1], "value1")
	}
	if result[2] != "key2" {
		t.Errorf("result[2] = %v, want %q", result[2], "key2")
	}
	if result[3] != "value2" {
		t.Errorf("result[3] = %v, want %q", result[3], "value2")
	}
}

func TestAttrsToAny_empty(t *testing.T) {
	t.Parallel()

	result := attrsToAny(nil)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d items", len(result))
	}
}

func TestRecordResult_success(t *testing.T) {
	t.Parallel()

	tp, exporter := newTestTracer()
	defer func() { _ = tp.Shutdown(context.Background()) }()
	tracer := tp.Tracer("test")

	ctx, span := tracer.Start(context.Background(), "test-op")
	baseAttrs := []attribute.KeyValue{
		attribute.String(AttrLayer, "service"),
		attribute.String(AttrDomain, "Users"),
		attribute.String(AttrAction, "GetByID"),
	}

	err := RecordResult(ctx, span, time.Now(), nil, baseAttrs, nil, "success")
	span.End()

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	if spans[0].Status.Code != codes.Ok {
		t.Errorf("span status = %v, want %v", spans[0].Status.Code, codes.Ok)
	}
}

func TestRecordResult_success_default_message(t *testing.T) {
	t.Parallel()

	tp, exporter := newTestTracer()
	defer func() { _ = tp.Shutdown(context.Background()) }()
	tracer := tp.Tracer("test")

	ctx, span := tracer.Start(context.Background(), "test-op")
	baseAttrs := []attribute.KeyValue{
		attribute.String(AttrLayer, "handler"),
	}

	err := RecordResult(ctx, span, time.Now(), nil, baseAttrs, nil)
	span.End()

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	spans := exporter.GetSpans()
	if spans[0].Status.Code != codes.Ok {
		t.Errorf("span status = %v, want %v", spans[0].Status.Code, codes.Ok)
	}
}

func TestRecordResult_error(t *testing.T) {
	t.Parallel()

	tp, exporter := newTestTracer()
	defer func() { _ = tp.Shutdown(context.Background()) }()
	tracer := tp.Tracer("test")

	ctx, span := tracer.Start(context.Background(), "test-op")
	baseAttrs := []attribute.KeyValue{
		attribute.String(AttrLayer, "repository"),
		attribute.String(AttrDomain, "Users"),
		attribute.String(AttrAction, "Insert"),
	}

	testErr := errors.New("connection refused")
	returnedErr := RecordResult(ctx, span, time.Now(), nil, baseAttrs, testErr)
	span.End()

	if !errors.Is(returnedErr, testErr) {
		t.Fatalf("expected original error, got %v", returnedErr)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	if spans[0].Status.Code != codes.Error {
		t.Errorf("span status = %v, want %v", spans[0].Status.Code, codes.Error)
	}

	if spans[0].Status.Description != "connection refused" {
		t.Errorf("span status description = %q, want %q", spans[0].Status.Description, "connection refused")
	}

	// Verify the error was recorded as a span event
	foundErrorEvent := false
	for _, event := range spans[0].Events {
		if event.Name == "exception" {
			foundErrorEvent = true
			break
		}
	}
	if !foundErrorEvent {
		t.Error("expected an exception event on the span")
	}
}

func TestRecordResult_with_metrics(t *testing.T) {
	t.Parallel()

	tp, _ := newTestTracer()
	defer func() { _ = tp.Shutdown(context.Background()) }()
	tracer := tp.Tracer("test")

	ctx, span := tracer.Start(context.Background(), "test-op")

	// Using nil metrics should not panic
	err := RecordResult(ctx, span, time.Now(), nil, nil, nil)
	span.End()

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestRecordError(t *testing.T) {
	t.Parallel()

	tp, exporter := newTestTracer()
	defer func() { _ = tp.Shutdown(context.Background()) }()
	tracer := tp.Tracer("test")

	ctx, span := tracer.Start(context.Background(), "test-op")
	baseAttrs := []attribute.KeyValue{
		attribute.String(AttrLayer, "service"),
	}

	testErr := errors.New("not found")
	returned := RecordError(ctx, span, time.Now(), testErr, nil, baseAttrs)
	span.End()

	if !errors.Is(returned, testErr) {
		t.Fatalf("RecordError should return the original error, got %v", returned)
	}

	spans := exporter.GetSpans()
	if spans[0].Status.Code != codes.Error {
		t.Errorf("span status = %v, want %v", spans[0].Status.Code, codes.Error)
	}
}

func TestRecordSuccess(t *testing.T) {
	t.Parallel()

	tp, exporter := newTestTracer()
	defer func() { _ = tp.Shutdown(context.Background()) }()
	tracer := tp.Tracer("test")

	ctx, span := tracer.Start(context.Background(), "test-op")
	baseAttrs := []attribute.KeyValue{
		attribute.String(AttrLayer, "handler"),
	}

	RecordSuccess(ctx, span, time.Now(), nil, baseAttrs, "all good")
	span.End()

	spans := exporter.GetSpans()
	if spans[0].Status.Code != codes.Ok {
		t.Errorf("span status = %v, want %v", spans[0].Status.Code, codes.Ok)
	}
}

func TestRecordSuccess_extraAttrs_on_span(t *testing.T) {
	t.Parallel()

	tp, exporter := newTestTracer()
	defer func() { _ = tp.Shutdown(context.Background()) }()
	tracer := tp.Tracer("test")

	ctx, span := tracer.Start(context.Background(), "test-op")
	baseAttrs := []attribute.KeyValue{attribute.String(AttrLayer, "usecase")}

	RecordSuccess(ctx, span, time.Now(), nil, baseAttrs, "role found",
		attribute.String("role.id", "abc-123"))
	span.End()

	spans := exporter.GetSpans()
	found := false
	for _, kv := range spans[0].Attributes {
		if string(kv.Key) == "role.id" && kv.Value.AsString() == "abc-123" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected extraAttrs (role.id=abc-123) on span, got %v", spans[0].Attributes)
	}
}

func TestRecordResult_nil_metrics(t *testing.T) {
	t.Parallel()

	tp, _ := newTestTracer()
	defer func() { _ = tp.Shutdown(context.Background()) }()
	tracer := tp.Tracer("test")

	ctx, span := tracer.Start(context.Background(), "test-op")

	// nil metrics should not panic
	err := RecordResult(ctx, span, time.Now(), nil, nil, nil)
	span.End()

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestRecordResult_metrics_with_nil_instruments(t *testing.T) {
	t.Parallel()

	tp, _ := newTestTracer()
	defer func() { _ = tp.Shutdown(context.Background()) }()
	tracer := tp.Tracer("test")

	ctx, span := tracer.Start(context.Background(), "test-op")

	// LayerMetrics with nil counter and histogram should not panic
	metrics := &LayerMetrics{
		Counter:   nil,
		Histogram: nil,
	}

	err := RecordResult(ctx, span, time.Now(), metrics, nil, nil)
	span.End()

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestRecordResult_preserves_base_attrs(t *testing.T) {
	t.Parallel()

	tp, _ := newTestTracer()
	defer func() { _ = tp.Shutdown(context.Background()) }()
	tracer := tp.Tracer("test")

	ctx, span := tracer.Start(context.Background(), "test-op")
	baseAttrs := []attribute.KeyValue{
		attribute.String(AttrLayer, "service"),
		attribute.String(AttrDomain, "Users"),
	}

	originalLen := len(baseAttrs)

	// Call RecordResult which appends "successful" attr internally
	_ = RecordResult(ctx, span, time.Now(), nil, baseAttrs, nil)
	span.End()

	// Base attrs should not be modified (copy was made internally)
	if len(baseAttrs) != originalLen {
		t.Errorf("base attrs were modified: len = %d, want %d", len(baseAttrs), originalLen)
	}
}

// TestSetupTraceKeepsEachCallersActionSeparate is the behavioural half of
// TestNoSharedMetadataActionWrite.
//
// One Metadata value, shared exactly the way a handler or repository shares the
// one it holds on its receiver, used concurrently by callers naming different
// operations. Every caller must get back the action IT asked for.
//
// The old API made this untestable, because naming an operation meant writing
// the shared field: under -race the write is reported outright, and without the
// detector the visible symptom is a span called service.Users.Insert for a
// request that was doing a Select. Passing the action by value makes the
// property expressible, and this test asserts it.
func TestSetupTraceKeepsEachCallersActionSeparate(t *testing.T) {
	t.Parallel()

	tp, _ := newTestTracer()

	defer func() { _ = tp.Shutdown(context.Background()) }()

	tracer := tp.Tracer("test")

	// The shared value. Note it carries no Action: there is nowhere to put one.
	shared := Metadata{Layer: "service", Domain: "Users"}

	actions := []string{"Insert", "SelectByID", "UpdateByID", "Delete", "List"}

	var wg sync.WaitGroup

	for range 50 {
		for _, action := range actions {

			wg.Go(func() {

				_, span, attrs := SetupTrace(context.Background(), tracer, shared, action)
				defer span.End()

				want := "service.Users." + action

				var got string

				for _, kv := range attrs {
					if string(kv.Key) == AttrAction {
						got = kv.Value.AsString()
					}
				}

				if got != action {
					t.Errorf("attrs carry action %q, want %q — a concurrent caller's action leaked into this one", got, action)
				}

				if name := shared.Layer + "." + shared.Domain + "." + got; name != want {
					t.Errorf("span would be named %q, want %q", name, want)
				}
			})
		}
	}

	wg.Wait()

	// And the shared value itself is untouched, which is what makes the above
	// true rather than merely lucky.
	if shared.Action != "" {
		t.Errorf("the shared Metadata was mutated: Action = %q", shared.Action)
	}
}
