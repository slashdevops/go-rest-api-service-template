package handler

import (
	"context"
	"encoding/json"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/middleware"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

// hangingHealthService stands in for a service whose dependencies are down.
//
// Both methods block until the test tears down, which is what a Postgres that
// has stopped answering looks like from inside the handler: not an error, a
// call that never comes back.
type hangingHealthService struct {
	called chan string
	block  chan struct{}
}

func (s *hangingHealthService) HealthCheck(ctx context.Context) (payload.Health, error) {
	s.called <- "HealthCheck"
	<-s.block
	return payload.Health{}, ctx.Err()
}

func (s *hangingHealthService) GetDetailedHealth(ctx context.Context) (payload.DetailedHealth, error) {
	s.called <- "GetDetailedHealth"
	<-s.block
	return payload.DetailedHealth{}, ctx.Err()
}

func newTestHealthHandler(t *testing.T, service HealthService) *HealthHandler {
	t.Helper()

	ctx := t.Context()

	ot := &o11y.OpenTelemetry{
		Traces:  o11y.NewOpenTelemetryTracer(ctx, &o11y.OpenTelemetryTracerConfig{Name: "test"}),
		Metrics: o11y.NewOpenTelemetryMeter(ctx, &o11y.OpenTelemetryMeterConfig{Name: "test"}),
	}

	handler, err := NewHealthHandler(HealthHandlerConf{Service: service, OT: ot})
	if err != nil {
		t.Fatalf("NewHealthHandler: %v", err)
	}

	return handler
}

// TestLivenessAnswersWithoutTouchingAnyDependency is the whole reason
// /health/live exists.
//
// A liveness probe decides whether the orchestrator KILLS this process, and
// restarting cannot fix a database that is down. /health/status pings the
// database inside a five second budget, so aiming a liveness probe at it turns
// a Postgres outage into a restart loop on top of a Postgres outage: every
// replica is killed for a fault none of them can do anything about, and the
// service stops serving the requests that never needed the database.
//
// So the property under test is not "returns 200" -- it is "returns 200 while
// the health service is hung". The fake here never returns from either method;
// if getLiveness ever consults it, this request never completes and the test
// fails on the deadline rather than on an assertion.
func TestLivenessAnswersWithoutTouchingAnyDependency(t *testing.T) {
	t.Parallel()

	service := &hangingHealthService{
		called: make(chan string, 2),
		block:  make(chan struct{}),
	}
	t.Cleanup(func() { close(service.block) })

	mux := http.NewServeMux()
	newTestHealthHandler(t, service).RegisterRoutes(mux, middleware.Chain(), middleware.Chain())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)

	done := make(chan struct{})
	go func() {
		defer close(done)
		mux.ServeHTTP(rec, req)
	}()

	select {
	case <-done:
	case name := <-service.called:
		t.Fatalf("liveness called %s: it must not consult any dependency, or a dependency outage becomes a restart loop", name)
	case <-time.After(5 * time.Second):
		t.Fatal("liveness did not answer while the health service was hung -- it is not usable as a liveness probe")
	}

	if got := rec.Code; got != http.StatusOK {
		t.Errorf("status = %d, want %d", got, http.StatusOK)
	}

	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}

	var body payload.HTTPMessage
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body.Message != "alive" {
		t.Errorf("message = %q, want %q", body.Message, "alive")
	}

	// Nothing may have been queued while the response was being written
	// either. A call recorded here would mean the handler consulted a
	// dependency and merely won the race against it.
	select {
	case name := <-service.called:
		t.Errorf("liveness called %s", name)
	default:
	}
}

// TestReadinessReflectsDependencies is the other half of the split: the
// readiness target must NOT answer from the process alone, or an instance
// whose database is unreachable keeps receiving traffic it cannot serve.
//
// Same hung service, opposite expectation -- /health/detailed is required to
// reach it.
func TestReadinessReflectsDependencies(t *testing.T) {
	t.Parallel()

	service := &hangingHealthService{
		called: make(chan string, 2),
		block:  make(chan struct{}),
	}
	t.Cleanup(func() { close(service.block) })

	mux := http.NewServeMux()
	newTestHealthHandler(t, service).RegisterRoutes(mux, middleware.Chain(), middleware.Chain())

	go mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health/detailed", nil))

	select {
	case name := <-service.called:
		if name != "GetDetailedHealth" {
			t.Errorf("readiness called %s, want GetDetailedHealth", name)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("readiness never consulted the health service -- it would report an instance with a dead database as ready")
	}
}
