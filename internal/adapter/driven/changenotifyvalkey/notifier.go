package changenotifyvalkey

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/valkey-io/valkey-go"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/changenotify"
)

// NotifierConfig configures [NewNotifier].
type NotifierConfig struct {
	// Client publishes and subscribes. Required.
	//
	// A DEDICATED client, not the one the counter uses: valkey-go puts a
	// connection into subscribe mode, where it accepts nothing but further
	// subscription commands. Sharing would stall every INCR the limiter makes.
	Client valkey.Client

	// Channel is the pub/sub channel. Every replica of one deployment must use
	// the same one, and two deployments sharing a Valkey must not. Each
	// mirrored thing has its own -- see [RateLimitRulesChannel] and
	// [TokenLifetimesChannel] -- so a write to one does not reload the other.
	// Required.
	Channel string

	// Subject names what the channel carries, for log lines only: "rate-limit
	// rules", "token lifetimes". Required.
	Subject string

	// InstanceID identifies this replica, so it can recognise the echo of its
	// own message. It has already reloaded by then, so acting on it would be a
	// wasted query on every write.
	InstanceID string

	// RetryInterval is how long to wait before resubscribing after the
	// subscription drops.
	RetryInterval time.Duration
}

// Notifier is a [changenotify.Notifier] over Valkey pub/sub.
//
// One instance per mirrored thing. It started life inside ratelimitvalkey as
// the rate-limit rule notifier and moved here, unchanged in mechanism, when the
// token lifetimes needed the same signal on a channel of their own.
//
// # Why pub/sub and not a queue
//
// The message is a signal, so nothing needs to be durable: a replica that was
// down missed nothing it will not learn from its next scheduled reload. A queue
// would add delivery guarantees for a payload that does not need them, and a
// backlog to drain for changes already superseded.
type Notifier struct {
	client     valkey.Client
	channel    string
	subject    string
	instanceID string
	retry      time.Duration
}

var _ changenotify.Notifier = (*Notifier)(nil)

// The channels, one per mirrored thing.
const (
	// RateLimitRulesChannel carries "the rate_limits table changed".
	RateLimitRulesChannel = "go-rest-api-service-template:ratelimit:rules"

	// TokenLifetimesChannel carries "the authn_token_lifetimes row changed".
	TokenLifetimesChannel = "go-rest-api-service-template:authn:token_lifetimes"
)

// defaultRetryInterval bounds how fast a replica reconnects to a Valkey that is
// refusing. Short enough that recovery is not noticed, long enough that a
// hundred replicas do not become a reconnection storm.
const defaultRetryInterval = 2 * time.Second

// NewNotifier builds the notifier.
func NewNotifier(conf NotifierConfig) (*Notifier, error) {
	if conf.Client == nil {
		return nil, fmt.Errorf("change notifier: valkey client is nil")
	}

	if conf.Channel == "" {
		return nil, fmt.Errorf("change notifier: channel is required; two mirrors sharing one would reload each other")
	}

	if conf.Subject == "" {
		return nil, fmt.Errorf("change notifier: subject is required, it names the channel in every log line")
	}

	retry := conf.RetryInterval
	if retry <= 0 {
		retry = defaultRetryInterval
	}

	return &Notifier{
		client:     conf.Client,
		channel:    conf.Channel,
		subject:    conf.Subject,
		instanceID: conf.InstanceID,
		retry:      retry,
	}, nil
}

// Notify announces that the subject changed.
//
// The payload is this replica's id and nothing else. It is not the value, and
// it is not even what changed: a receiver reloads the whole thing, so a message
// lost in a reconnect costs a delay and a message delivered twice costs a query.
func (ref *Notifier) Notify(ctx context.Context) error {
	cmd := ref.client.B().Publish().Channel(ref.channel).Message(ref.instanceID).Build()

	if err := ref.client.Do(ctx, cmd).Error(); err != nil {
		return fmt.Errorf("%s notifier: publishing to %s: %w", ref.subject, ref.channel, err)
	}

	return nil
}

// Watch calls onChange for every notification until ctx is done.
//
// It reconnects on its own. A subscription that drops and stays dropped returns
// the service to ticker-only propagation with nothing on screen to say so, which
// is the failure mode this whole mechanism is meant to remove -- so the
// reconnection is loud the first time and quiet afterwards.
func (ref *Notifier) Watch(ctx context.Context, onChange func()) error {
	var announced bool

	for {
		if err := ctx.Err(); err != nil {
			return nil
		}

		// Receive blocks until the context ends or the connection fails.
		cmd := ref.client.B().Subscribe().Channel(ref.channel).Build()

		err := ref.client.Receive(ctx, cmd, func(msg valkey.PubSubMessage) {
			// Its own echo: this replica applied the change before publishing,
			// so reloading again would be a wasted query on every write.
			//
			// A payload that does not match is treated as "somebody changed
			// something", NOT parsed for meaning -- the message is a signal, and
			// trusting its content is what would let a malformed or replayed
			// message install a wrong value.
			if ref.instanceID != "" && msg.Message == ref.instanceID {
				return
			}

			onChange()
		})

		if ctx.Err() != nil {
			return nil
		}

		if err != nil && !errors.Is(err, context.Canceled) {
			if !announced {
				slog.WarnContext(
					ctx, ref.subject+" change notifications are not being received; falling back to the reload interval",
					"error", err,
					"channel", ref.channel,
					"consequence", "a change written on another replica takes up to the reload interval to apply here",
				)

				announced = true
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(ref.retry):
		}
	}
}

// Close releases the subscription client.
func (ref *Notifier) Close() error {
	ref.client.Close()

	return nil
}
