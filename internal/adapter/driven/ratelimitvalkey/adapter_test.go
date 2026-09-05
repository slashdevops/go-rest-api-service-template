//go:build unit

package ratelimitvalkey

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"os"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/ratelimit"
)

// newTestClient connects to the dev Valkey, or skips.
//
// A SKIP is honest about what was not verified. A mock that replays the same
// command builder this package uses would assert nothing about Valkey's actual
// INCR semantics, which is the entire thing under test -- the same rule the
// provider fixtures follow.
func newTestClient(t *testing.T) valkey.Client {
	t.Helper()

	addr := os.Getenv("VALKEY_TEST_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}

	opt := valkey.ClientOption{
		InitAddress:  []string{addr},
		DisableRetry: true,
	}

	if ca := os.Getenv("VALKEY_TEST_CA"); ca != "" {
		pem, err := os.ReadFile(ca)
		if err != nil {
			t.Skipf("valkey CA unreadable: %v", err)
		}

		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			t.Skip("valkey CA is not a PEM bundle")
		}

		opt.TLSConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	}

	client, err := valkey.NewClient(opt)
	if err != nil {
		t.Skipf("valkey unavailable at %s: %v", addr, err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	if err := client.Do(ctx, client.B().Ping().Build()).Error(); err != nil {
		client.Close()
		t.Skipf("valkey unreachable at %s: %v", addr, err)
	}

	t.Cleanup(client.Close)

	return client
}

func newTestAdapter(t *testing.T) *Adapter {
	t.Helper()

	a, err := New(Config{Client: newTestClient(t), Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return a
}

func uniqueKey(t *testing.T) string {
	t.Helper()

	return "test:" + t.Name() + ":" + time.Now().Format("150405.000000000")
}

func TestNewRejectsAnUnboundedTimeout(t *testing.T) {
	t.Parallel()

	// The HTTP server sets no ReadTimeout or WriteTimeout deliberately, so a
	// request context often carries no deadline. Without a timeout here an
	// unresponsive Valkey holds the request open for as long as the connection
	// allows -- which is the failure this rejects at construction.
	if _, err := New(Config{Client: nil, Timeout: time.Second}); err == nil {
		t.Fatal("a nil client must be rejected")
	}
}

func TestAllowsUpToTheLimitThenRefuses(t *testing.T) {
	a := newTestAdapter(t)
	key := uniqueKey(t)
	b := ratelimit.Budget{Requests: 3, Period: time.Minute, Strategy: "token_bucket"}

	for i := range 3 {
		d, err := a.Allow(t.Context(), key, b, 1)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}

		if !d.Allowed {
			t.Fatalf("request %d refused; the budget is 3", i)
		}

		if want := 2 - i; d.Remaining != want {
			t.Fatalf("request %d: Remaining = %d, want %d", i, d.Remaining, want)
		}
	}

	d, err := a.Allow(t.Context(), key, b, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if d.Allowed {
		t.Fatal("the fourth request should be refused")
	}

	if d.RetryAfter <= 0 || d.RetryAfter > time.Minute {
		t.Fatalf("Retry-After = %s, want a value inside the window", d.RetryAfter)
	}
}

// The property that makes INCR sufficient and a Lua script unnecessary: two
// independent adapters -- standing in for two replicas -- share one counter.
func TestTwoAdaptersShareTheCounter(t *testing.T) {
	replicaA := newTestAdapter(t)
	replicaB := newTestAdapter(t)

	key := uniqueKey(t)
	b := ratelimit.Budget{Requests: 2, Period: time.Minute, Strategy: "token_bucket"}

	if d, err := replicaA.Allow(t.Context(), key, b, 1); err != nil || !d.Allowed {
		t.Fatalf("replica A first request: %v", err)
	}

	if d, err := replicaB.Allow(t.Context(), key, b, 1); err != nil || !d.Allowed {
		t.Fatalf("replica B first request: %v", err)
	}

	// The budget is 2 in total, NOT 2 per replica. If this passes, the counter
	// is not shared and the whole point of the Valkey path is gone.
	d, err := replicaB.Allow(t.Context(), key, b, 1)
	if err != nil {
		t.Fatalf("replica B second request: %v", err)
	}

	if d.Allowed {
		t.Fatal("the budget is 2 across both replicas; a third request means each replica has its own counter")
	}
}

func TestDifferentKeysDoNotShareABucket(t *testing.T) {
	a := newTestAdapter(t)
	b := ratelimit.Budget{Requests: 1, Period: time.Minute, Strategy: "token_bucket"}

	alice := uniqueKey(t) + ":alice"
	bob := uniqueKey(t) + ":bob"

	if d, _ := a.Allow(t.Context(), alice, b, 1); !d.Allowed {
		t.Fatal("alice's first request should be allowed")
	}

	if d, _ := a.Allow(t.Context(), alice, b, 1); d.Allowed {
		t.Fatal("alice's second request should be refused")
	}

	if d, _ := a.Allow(t.Context(), bob, b, 1); !d.Allowed {
		t.Fatal("bob must have his own counter")
	}
}

// The window must actually roll. A key whose TTL is pushed forward on every
// request never expires under load, so the window never rolls and the limit
// becomes permanent -- silently.
func TestTheWindowRolls(t *testing.T) {
	a := newTestAdapter(t)
	key := uniqueKey(t)
	b := ratelimit.Budget{Requests: 1, Period: time.Second, Strategy: "token_bucket"}

	if d, _ := a.Allow(t.Context(), key, b, 1); !d.Allowed {
		t.Fatal("first request should be allowed")
	}

	if d, _ := a.Allow(t.Context(), key, b, 1); d.Allowed {
		t.Fatal("second request in the same window should be refused")
	}

	// Cross the boundary. A real sleep, because the window index is derived from
	// the wall clock -- that is the mechanism, and a fake clock would test
	// something else.
	time.Sleep(1100 * time.Millisecond)

	if d, err := a.Allow(t.Context(), key, b, 1); err != nil || !d.Allowed {
		t.Fatalf("the window did not roll: %v", err)
	}
}

// The TTL must count DOWN across requests in one window, not reset.
//
// The window rolls on the key changing, so a re-pushed TTL is not a correctness
// bug -- it is a residency one: every past window's key stays alive for as long
// as traffic continues. Asserting the roll instead (as this test first did)
// passes whether or not the guard is there, because the roll does not depend on
// it. Mutation testing is what surfaced that.
func TestTheTTLIsNotPushedForwardOnEveryRequest(t *testing.T) {
	client := newTestClient(t)

	a, err := New(Config{Client: client, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	key := uniqueKey(t)
	b := ratelimit.Budget{Requests: 100, Period: time.Minute, Strategy: "token_bucket"}

	if _, err := a.Allow(t.Context(), key, b, 1); err != nil {
		t.Fatalf("first request: %v", err)
	}

	full, _ := a.windowKey(key, b, time.Now())

	pttl := func() int64 {
		t.Helper()

		res := client.Do(t.Context(), client.B().Pttl().Key(full).Build())
		if err := res.Error(); err != nil {
			t.Fatalf("pttl: %v", err)
		}

		n, err := res.AsInt64()
		if err != nil {
			t.Fatalf("pttl result: %v", err)
		}

		return n
	}

	before := pttl()
	if before <= 0 {
		t.Fatalf("no TTL was set on the first increment (pttl=%d); the key would never be cleaned up", before)
	}

	time.Sleep(300 * time.Millisecond)

	if _, err := a.Allow(t.Context(), key, b, 1); err != nil {
		t.Fatalf("second request: %v", err)
	}

	after := pttl()
	if after >= before {
		t.Fatalf("the TTL was pushed forward on the second request (%d -> %d); under continuous load every past window's key stays resident", before, after)
	}
}

// A fault must be UNKNOWN, never "allowed". Reporting a closed client as an
// admitted request would remove the limit precisely when the system is least
// healthy -- the mistake the token denylist is written to avoid.
func TestAnUnreachableStoreIsAnErrorNotAnAllow(t *testing.T) {
	client := newTestClient(t)

	a, err := New(Config{Client: client, Timeout: 500 * time.Millisecond})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	client.Close()

	d, err := a.Allow(t.Context(), uniqueKey(t), ratelimit.Budget{Requests: 10, Period: time.Minute}, 1)
	if err == nil {
		t.Fatal("a closed client must produce an error")
	}

	if d.Allowed {
		t.Fatal("a fault reported as Allowed: true removes the limit exactly when the system is least healthy")
	}
}

func TestAnInvalidBudgetIsRejected(t *testing.T) {
	t.Parallel()

	a := &Adapter{timeout: time.Second}

	for _, b := range []ratelimit.Budget{
		{Requests: 0, Period: time.Minute},
		{Requests: 10, Period: 0},
	} {
		if _, err := a.Allow(context.Background(), "k", b, 1); err == nil {
			t.Fatalf("budget %+v should be rejected", b)
		}
	}
}

// Burst, not Requests, is what the shared counter admits. Using Requests when a
// rule sets a larger burst makes the shared counter stricter than the rule, so
// the configured burst is never reachable and nothing says why.
func TestCapacityUsesBurstWhenItIsLarger(t *testing.T) {
	t.Parallel()

	if got := capacity(ratelimit.Budget{Requests: 10, Burst: 30}); got != 30 {
		t.Fatalf("capacity = %d, want 30", got)
	}

	if got := capacity(ratelimit.Budget{Requests: 10, Burst: 0}); got != 10 {
		t.Fatalf("capacity = %d, want 10 — burst 0 means requests", got)
	}

	if got := capacity(ratelimit.Budget{Requests: 10, Burst: 3}); got != 10 {
		t.Fatalf("capacity = %d, want 10 — a burst smaller than requests must not shrink the window budget", got)
	}
}

func TestWindowKeyIsStableWithinAWindowAndChangesAcross(t *testing.T) {
	t.Parallel()

	a := &Adapter{timeout: time.Second}
	b := ratelimit.Budget{Requests: 1, Period: time.Minute}

	base := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)

	k1, _ := a.windowKey("x", b, base)
	k2, _ := a.windowKey("x", b, base.Add(30*time.Second))

	if k1 != k2 {
		// Two replicas a moment apart must compute the SAME key, or they each
		// count into their own window and the shared budget is not shared.
		t.Fatalf("keys differ inside one window: %s vs %s", k1, k2)
	}

	k3, _ := a.windowKey("x", b, base.Add(61*time.Second))
	if k1 == k3 {
		t.Fatal("the key must change across a window boundary, or the window never rolls")
	}

	_, resetsIn := a.windowKey("x", b, base.Add(30*time.Second))
	if resetsIn <= 0 || resetsIn > time.Minute {
		t.Fatalf("resetsIn = %s, want the remainder of the window", resetsIn)
	}
}
