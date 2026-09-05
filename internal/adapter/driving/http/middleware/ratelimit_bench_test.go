//go:build unit

package middleware_test

import (
	"net/http"
	"testing"
	"time"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driven/ratelimitmemory"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/middleware"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// PLAN.md section 10 bounds "router lookup per request adds latency" with
// "benchmark before merge". It was never written -- the repo had no Benchmark at
// all -- so the cost of the pre-auth stage was asserted rather than measured.
//
// It is worth measuring specifically because of WHERE it sits. The pre-auth
// stage runs on every request, including the flood a rate limiter exists to
// shed, and it cannot read r.Pattern (the API mux is a subtree mount, so
// r.Pattern is already "/api/v1/" there) -- so it asks the inner mux for the
// route on every request. If that lookup were expensive, the limiter's cost
// would scale with the traffic it is meant to bound.
//
// Run: go test -tags=unit -bench=RateLimit -benchmem ./internal/adapter/driving/http/middleware/
func benchRouter() *http.ServeMux {
	mux := http.NewServeMux()

	// A spread of the shapes the real API registers, so the segment walk has
	// something to walk: literals, one wildcard, and two.
	for _, p := range []string{
		"GET /models", "POST /models", "GET /models/{model_id}",
		"GET /projects", "POST /projects", "GET /projects/{project_id}",
		"POST /projects/{project_id}/generate",
		"GET /projects/{project_id}/embeddings/{embedding_id}",
		"GET /users", "GET /users/{user_id}", "GET /rate_limits",
		"GET /rate_limits/effective", "GET /rate_limits/{rate_limit_id}",
	} {
		mux.Handle(p, http.NotFoundHandler())
	}

	return mux
}

func benchRules(n int) staticRules {
	rules := make([]domain.RateLimit, 0, n+1)
	rules = append(rules, mwRule("global", domain.RateLimitScopeIP))

	// Endpoint rules that do NOT match the benchmarked path, so resolution walks
	// the whole set rather than short-circuiting on the first hit.
	for i := range n {
		r := mwRule("endpoint-"+uuid.NewV7().String(), domain.RateLimitScopeIP)
		r.TargetKind = domain.RateLimitTargetKindEndpoint
		r.Target = "/projects/{project_id}"
		r.Windows = []domain.RateLimitWindow{
			{ID: uuid.NewV7(), Requests: 1000000, Period: time.Hour, Burst: 1000000},
		}
		rules = append(rules, r)

		_ = i
	}

	return staticRules{known: true, rules: rules}
}

// The whole pre-auth stage: route lookup, resolution, and one charge against the
// in-process limiter. That is the per-request cost the risk was about.
func BenchmarkRateLimitPreAuthStage(b *testing.B) {
	for _, rules := range []int{1, 10, 50} {
		b.Run("rules="+itoa(rules), func(b *testing.B) {
			local := ratelimitmemory.New()
			b.Cleanup(func() { _ = local.Close() })

			resolver, err := middleware.NewClientIPResolver(nil)
			if err != nil {
				b.Fatalf("NewClientIPResolver: %v", err)
			}

			mdw := middleware.RateLimit(middleware.RateLimitConfig{
				Rules:    benchRules(rules),
				Local:    local,
				Resolver: resolver,
				Router:   benchRouter(),
				Stage:    middleware.RateLimitStagePreAuth,
			})

			h := mdw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
			req := rlRequest(http.MethodGet, "/models")
			w := discardWriter{}

			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				h.ServeHTTP(w, req)
			}
		})
	}
}

// The route lookup alone, so the two halves of the cost can be told apart. This
// is the part PLAN.md called "a segment walk under an RLock".
func BenchmarkRateLimitRouteLookup(b *testing.B) {
	mux := benchRouter()
	req := rlRequest(http.MethodGet, "/projects/abc/embeddings/def")

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_, pattern := mux.Handler(req)
		if pattern == "" {
			b.Fatal("no pattern matched; the benchmark is measuring the miss path")
		}
	}
}

// discardWriter is an http.ResponseWriter that keeps no state, so the benchmark
// measures the middleware rather than a recorder's allocations.
type discardWriter struct{}

func (discardWriter) Header() http.Header         { return http.Header{} }
func (discardWriter) Write(b []byte) (int, error) { return len(b), nil }
func (discardWriter) WriteHeader(int)             {}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	var buf [20]byte

	i := len(buf)

	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}

	return string(buf[i:])
}
