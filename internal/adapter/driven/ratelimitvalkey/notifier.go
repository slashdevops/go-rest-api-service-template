package ratelimitvalkey

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/valkey-io/valkey-go"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/ratelimit"
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
	// the same one, and two deployments sharing a Valkey must not.
	Channel string

	// InstanceID identifies this replica, so it can recognise the echo of its
	// own message. It has already reloaded by then, so acting on it would be a
	// wasted query on every write.
	InstanceID string

	// RetryInterval is how long to wait before resubscribing after the
	// subscription drops.
	RetryInterval time.Duration
}

// Notifier is a [ratelimit.ChangeNotifier] over Valkey pub/sub.
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
	instanceID string
	retry      time.Duration
}

var _ ratelimit.ChangeNotifier = (*Notifier)(nil)

// DefaultChannel is the pub/sub channel used when none is configured.
const DefaultChannel = "go-rest-api-service-template:ratelimit:rules"

// defaultRetryInterval bounds how fast a replica reconnects to a Valkey that is
// refusing. Short enough that recovery is not noticed, long enough that a
// hundred replicas do not become a reconnection storm.
const defaultRetryInterval = 2 * time.Second

// NewNotifier builds the notifier.
func NewNotifier(conf NotifierConfig) (*Notifier, error) {
	if conf.Client == nil {
		return nil, fmt.Errorf("rate-limit notifier: valkey client is nil")
	}

	channel := conf.Channel
	if channel == "" {
		channel = DefaultChannel
	}

	retry := conf.RetryInterval
	if retry <= 0 {
		retry = defaultRetryInterval
	}

	return &Notifier{
		client:     conf.Client,
		channel:    channel,
		instanceID: conf.InstanceID,
		retry:      retry,
	}, nil
}

// Notify announces that the rule set changed.
//
// The payload is this replica's id and nothing else. It is not the rules, and it
// is not even which rule: a receiver queries for the whole set, so a message
// lost in a reconnect costs a delay and a message delivered twice costs a query.
func (ref *Notifier) Notify(ctx context.Context) error {
	cmd := ref.client.B().Publish().Channel(ref.channel).Message(ref.instanceID).Build()

	if err := ref.client.Do(ctx, cmd).Error(); err != nil {
		return fmt.Errorf("rate-limit notifier: publishing to %s: %w", ref.channel, err)
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
			// message install a wrong rule set.
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
					ctx, "rate-limit rule notifications are not being received; falling back to the reload interval",
					"error", err,
					"channel", ref.channel,
					"consequence", "a rule written on another replica takes up to ratelimit.reload.interval to apply here",
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
