package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driven/ratelimitmemory"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/middleware"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// ipLimiterHandler builds the real limiter with an ip-scoped rule of the given
// burst, keyed through the given resolver.
//
// These tests used to drive middleware.IPRateLimiter, the per-IP flag limiter,
// which no longer exists -- there is one limiter now and its budgets come from
// the database. The PROPERTY they protect is unchanged and is a security
// boundary, so they were repointed rather than deleted: it is the resolver, not
// a caller-supplied header, that chooses the bucket.
func ipLimiterHandler(t *testing.T, resolver *middleware.ClientIPResolver, burst int) http.Handler {
	t.Helper()

	rule := mwRule("ip", domain.RateLimitScopeIP,
		domain.RateLimitWindow{ID: uuid.NewV7(), Requests: burst, Period: time.Hour, Burst: burst})

	local := ratelimitmemory.New()
	t.Cleanup(func() { _ = local.Close() })

	return middleware.RateLimit(middleware.RateLimitConfig{
		Rules:    staticRules{known: true, rules: []domain.RateLimit{rule}},
		Local:    local,
		Resolver: resolver,
		Stage:    middleware.RateLimitStagePreAuth,
	})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
}

// countStatuses drives n requests through the middleware and tallies the
// responses. forwardedFor, when non-nil, supplies a header value per request.
func countStatuses(handler http.Handler, remoteAddr string, n int, forwardedFor func(i int) string) map[int]int {
	got := make(map[int]int, 2)

	for i := range n {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = remoteAddr

		if forwardedFor != nil {
			req.Header.Set("X-Forwarded-For", forwardedFor(i))
		}

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		got[rec.Code]++
	}

	return got
}

// TestIPRateLimiter_rotatingForwardedForDoesNotMultiplyTheBudget is the
// end-to-end regression test for the rate-limiter bypass, driven through the
// real middleware and a real token bucket rather than the resolver alone.
//
// Before the fix the limiter keyed on an unvalidated X-Forwarded-For, so a
// caller that varied the header drew a fresh budget on every request and the
// throttle never engaged. Reproduced against the running API, guessing
// passwords against a real account with the limiter at 5 req/s:
//
//	30 attempts, one source:   {401: 7,  429: 23}
//	30 attempts, rotating XFF: {401: 30}
//
// The property asserted here is that both columns now look alike.
func TestIPRateLimiter_rotatingForwardedForDoesNotMultiplyTheBudget(t *testing.T) {
	t.Parallel()

	const (
		burst    = 5
		requests = 30
	)

	// No trusted proxies: the dev and default posture, and the one a service
	// exposed directly should run with.
	resolver, err := middleware.NewClientIPResolver(nil)
	require.NoError(t, err)

	newHandler := func() http.Handler { return ipLimiterHandler(t, resolver, burst) }

	plain := countStatuses(newHandler(), "203.0.113.9:1234", requests, nil)
	rotating := countStatuses(newHandler(), "203.0.113.9:1234", requests, func(i int) string {
		return "198.51.100." + string(rune('1'+i%9))
	})

	require.Positive(t, plain[http.StatusTooManyRequests],
		"the limiter must reject something at %d requests against a burst of %d", requests, burst)

	assert.Equal(t, plain[http.StatusOK], rotating[http.StatusOK],
		"a rotating X-Forwarded-For let through %d requests versus %d without it — the header is choosing the bucket",
		rotating[http.StatusOK], plain[http.StatusOK])

	assert.LessOrEqual(t, rotating[http.StatusOK], burst,
		"no more than the burst may pass, whatever the header says")
}

// TestIPRateLimiter_trustedProxySeparatesRealClients is the other half: once a
// proxy is trusted, distinct clients behind it must get distinct buckets,
// otherwise the fix would throttle a whole deployment as one caller.
func TestIPRateLimiter_trustedProxySeparatesRealClients(t *testing.T) {
	t.Parallel()

	const burst = 3

	resolver, err := middleware.NewClientIPResolver([]string{"10.0.0.0/8"})
	require.NoError(t, err)

	handler := ipLimiterHandler(t, resolver, burst)

	// One client behind the proxy burns its own budget...
	exhausted := countStatuses(handler, "10.0.0.1:1234", burst+3, func(int) string { return "203.0.113.50" })
	require.Positive(t, exhausted[http.StatusTooManyRequests], "the first client must hit its limit")

	// ...and a different client behind the same proxy is unaffected.
	other := countStatuses(handler, "10.0.0.1:1234", burst, func(int) string { return "203.0.113.99" })

	assert.Equal(t, burst, other[http.StatusOK],
		"a second client behind a trusted proxy must have its own bucket, not inherit the first one's")
}

// The 429 body must never echo a caller-supplied header back.
//
// The flag limiter wrote "too many requests from ip address <addr>", so this
// test asserted the address was the resolved peer and not the X-Forwarded-For.
// The rule limiter names no address at all, which is strictly better: there is
// nothing for a caller to influence and nothing for an operator to mistake for
// a verified value. The assertion is kept, pointed at what still matters.
func TestARejectionDoesNotEchoTheCallersOwnHeader(t *testing.T) {
	t.Parallel()

	resolver, err := middleware.NewClientIPResolver(nil)
	require.NoError(t, err)

	handler := ipLimiterHandler(t, resolver, 1)

	var body string

	for range 5 {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "203.0.113.9:1234"
		req.Header.Set("X-Forwarded-For", "198.51.100.77")

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code == http.StatusTooManyRequests {
			body = rec.Body.String()

			break
		}
	}

	require.NotEmpty(t, body, "expected the limiter to reject a request")
	assert.NotContains(t, body, "198.51.100.77", "the rejection must not echo the caller's own header back")
}
