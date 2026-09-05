//go:build unit

package changenotifyvalkey

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
)

// newTestClient connects to a real Valkey, or SKIPS. A skipped test reports
// ok, so the Makefile sets VALKEY_TEST_CA from the dev stack and CI provides a
// plaintext service container -- see ratelimitvalkey/adapter_test.go, whose
// helper this mirrors, for why neither is optional.
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

// waitFor polls until cond holds or the budget runs out, so a test does not
// depend on how fast a real Valkey delivers.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for %s", what)
}

func newTestNotifier(t *testing.T, channel, instanceID string) *Notifier {
	t.Helper()

	n, err := NewNotifier(NotifierConfig{
		Client:     newTestClient(t),
		Channel:    channel,
		Subject:    "test",
		InstanceID: instanceID,
	})
	if err != nil {
		t.Fatalf("NewNotifier: %v", err)
	}

	t.Cleanup(func() { _ = n.Close() })

	return n
}

// The property the whole mechanism exists for: a write on one replica reaches
// the others without waiting out the reload interval.
//
// Against a real Valkey, not a mock. A mock replaying this package's own
// command builder would assert nothing about pub/sub actually delivering, which
// is the entire thing under test.
func TestANotificationReachesAnotherReplica(t *testing.T) {
	t.Parallel()

	channel := "test:ratelimit:" + t.Name()

	var got atomic.Int64

	// Two notifiers with DIFFERENT instance ids: one publisher, one subscriber,
	// which is the two-replica case.
	sub := newTestNotifier(t, channel, "replica-b")
	pub := newTestNotifier(t, channel, "replica-a")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	go func() { _ = sub.Watch(ctx, func() { got.Add(1) }) }()

	// Give the subscription a moment to establish; without it the publish races
	// the SUBSCRIBE and the message is genuinely lost, which would make this
	// test flaky for a reason that is not a bug.
	waitForSubscription(t, pub, channel)

	if err := pub.Notify(ctx); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	waitFor(t, "the notification to arrive", func() bool { return got.Load() >= 1 })
}

// A replica must ignore the echo of its own message: it applied the change
// before publishing, so acting on it is a wasted query on every write.
func TestAReplicaIgnoresItsOwnEcho(t *testing.T) {
	t.Parallel()

	channel := "test:ratelimit:" + t.Name()

	var own atomic.Int64

	self := newTestNotifier(t, channel, "replica-a")
	peer := newTestNotifier(t, channel, "replica-b")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	go func() { _ = self.Watch(ctx, func() { own.Add(1) }) }()

	waitForSubscription(t, peer, channel)

	// First establish that delivery WORKS on this channel, so a zero later is
	// suppression and not a message that never arrived.
	if err := peer.Notify(ctx); err != nil {
		t.Fatalf("peer Notify: %v", err)
	}

	waitFor(t, "the peer's notification", func() bool { return own.Load() == 1 })

	// Now its own. The settle window is what makes this deterministic: an
	// earlier version waited for `own >= 1`, which was already satisfied, so it
	// passed even with the echo filter removed. A local Valkey delivers in
	// single-digit milliseconds, so 500ms is a wide margin for "it would have
	// arrived by now".
	if err := self.Notify(ctx); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	if got := own.Load(); got != 1 {
		t.Fatalf("the watcher fired %d times; it must ignore the echo of its own message, "+
			"having already applied that change before publishing it", got)
	}

	// And it is still listening -- suppression must not mean the subscription
	// died on the message it ignored.
	if err := peer.Notify(ctx); err != nil {
		t.Fatalf("peer Notify: %v", err)
	}

	waitFor(t, "the peer's second notification", func() bool { return own.Load() == 2 })
}

// Watch returns when the context ends, rather than leaking a goroutine for the
// life of the process.
func TestWatchStopsWithItsContext(t *testing.T) {
	t.Parallel()

	n := newTestNotifier(t, "test:ratelimit:"+t.Name(), "replica-a")

	ctx, cancel := context.WithCancel(t.Context())

	done := make(chan struct{})

	go func() {
		_ = n.Watch(ctx, func() {})
		close(done)
	}()

	waitForSubscription(t, n, "test:ratelimit:"+t.Name())
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Watch did not return when its context was cancelled")
	}
}

// waitForSubscription blocks until the channel has at least one subscriber, so
// a publish cannot race the SUBSCRIBE that was meant to receive it.
func waitForSubscription(t *testing.T, n *Notifier, channel string) {
	t.Helper()

	waitFor(t, "the subscription to establish", func() bool {
		res := n.client.Do(t.Context(), n.client.B().PubsubNumsub().Channel(channel).Build())

		m, err := res.AsIntMap()
		if err != nil {
			return false
		}

		return m[channel] > 0
	})
}

func TestNewNotifierRejectsANilClient(t *testing.T) {
	t.Parallel()

	if _, err := NewNotifier(NotifierConfig{Channel: "x", Subject: "x"}); err == nil {
		t.Fatal("a nil client must be refused")
	}
}

// Two mirrors on one channel would reload each other on every write, and a
// notifier with no subject cannot say which one is failing. Both are refused
// at construction rather than discovered in a log line that names nothing.
func TestNewNotifierRequiresAChannelAndASubject(t *testing.T) {
	t.Parallel()

	client := newTestClient(t)

	if _, err := NewNotifier(NotifierConfig{Client: client, Subject: "x"}); err == nil {
		t.Fatal("a missing channel must be refused")
	}

	if _, err := NewNotifier(NotifierConfig{Client: client, Channel: "x"}); err == nil {
		t.Fatal("a missing subject must be refused")
	}

	if RateLimitRulesChannel == TokenLifetimesChannel {
		t.Fatal("the two mirrors must not share a channel")
	}
}
