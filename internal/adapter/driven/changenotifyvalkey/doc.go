// Package changenotifyvalkey is the Valkey pub/sub adapter for
// [changenotify.Notifier]: one replica publishes "something changed" on a
// channel, every other replica subscribed to it reloads.
//
// # What it carries
//
// This replica's instance id, and nothing else. Never the value: a receiver
// reloads from the store, so a lost message costs a delay and a duplicate
// costs a query, where a value in the payload would make delivery order
// load-bearing across a reconnect that offers no order.
//
// # One channel per mirrored thing
//
// [RateLimitRulesChannel] and [TokenLifetimesChannel]. Sharing one would make
// every rate-limit write reload the token lifetimes and vice versa -- harmless
// today, and exactly the kind of coupling that becomes a surprise later.
//
// # The client is dedicated, and shared between notifiers
//
// valkey-go puts a connection into subscribe mode, where it accepts nothing but
// further subscription commands, so the notifiers must not share the client the
// cache and the rate-limit counter use. They may share one with EACH OTHER:
// each Watch takes its own dedicated connection from the client.
//
// # Failure is a delay, never a wrong answer
//
// A failed publish, a dropped subscription, no cache at all: in every case the
// mirror's reload ticker still carries the change, and the only cost is
// waiting up to one interval. The first dropped subscription is logged;
// reconnection is quiet after that.
//
// # History
//
// This began as ratelimitvalkey.Notifier and moved here, unchanged in
// mechanism, when the token lifetimes needed the same signal on a channel of
// their own. The tests moved with it and still run against a real Valkey --
// a mock replaying this package's own command builder would assert nothing
// about pub/sub actually delivering.
package changenotifyvalkey
