// Package changenotify is the port through which one replica tells the others
// that something they mirror in memory has changed.
//
// # What it carries
//
// A SIGNAL, never the data. A message says only "the thing on channel X
// changed"; the receiver then reloads from the store. That is what makes a lost
// message cost a delay and a duplicated message cost a query, rather than either
// one installing a wrong value. Shipping the value in the message would make
// delivery order load-bearing, and pub/sub offers no order across a reconnect.
//
// # What it is for
//
// Anything served from a per-replica mirror with a reload ticker: the rate-limit
// rule set and the token lifetimes today. Each mirror owns its own channel, so a
// write to one does not make every other mirror reload.
//
// # It is an optimisation, never the mechanism
//
// The mirror's reload ticker remains the floor. Everything here may fail -- the
// transport is optional (cache.enabled=false is supported), a publish can fail, a
// subscription can drop -- and the only consequence must be that a change takes
// up to one reload interval to appear, which is exactly the behaviour before any
// of this existed.
//
// It used to live inside the rate-limit port as ratelimit.ChangeNotifier. It
// moved here, unchanged, when a second mirror needed it; that name is kept as an
// alias so nothing that spoke the old one had to change.
package changenotify

import "context"

// Notifier announces changes on one channel and watches for them.
type Notifier interface {
	// Notify announces a change. A failure is not fatal to the write that
	// caused it: the write already succeeded, and the ticker will carry it.
	Notify(ctx context.Context) error

	// Watch calls onChange once per notification until ctx is done. It blocks,
	// and it is responsible for reconnecting: a dropped subscription that stays
	// dropped silently returns the service to ticker-only propagation with
	// nothing to say so.
	Watch(ctx context.Context, onChange func()) error

	// Close releases the subscription.
	Close() error
}
