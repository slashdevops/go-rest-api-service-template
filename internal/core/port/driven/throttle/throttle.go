// Package throttle defines the driven port use-cases consume to bound repeated
// failed attempts against a single subject — today, login attempts against one
// account.
//
// # Why this exists next to the IP rate limiter
//
// The per-IP limiter in the HTTP middleware bounds how fast one *source* can
// call the API. It does nothing about a distributed attack: spread the same
// guesses across enough addresses and each one stays under the limit while the
// account underneath is hammered. Password guessing has to be bounded per
// account as well as per source, and the two limits are independent — neither
// substitutes for the other.
//
// # Only failures cost, but the mechanism is consume-then-refund
//
// [Throttle.Attempt] always spends one unit; [Throttle.Succeed] gives the whole
// budget back. The net effect is that only failures accumulate — someone who
// signs in normally never approaches the limit however often they do it, and
// someone who mistypes twice before getting it right starts clean.
//
// It is expressed that way round because a non-consuming check is not
// expressible on a token bucket. Reserving a token and cancelling it looks like
// one, but golang.org/x/time/rate refuses to return a token whose time to act
// has already passed — which is every immediately-available token — so the
// "check" silently spends. Refunding on success is the honest version of the
// same intent.
//
// # What callers must key on
//
// The key has to be derived from what the *caller supplied*, not from what was
// found. Throttling only accounts that exist would answer "does this address
// have an account?" through the difference in behaviour, which is the question
// an attacker enumerates for. A failed attempt against an unknown address must
// consume budget exactly like a failed attempt against a real one.
//
// # This delays, it does not lock
//
// Anyone who knows an address can spend its budget and slow its owner down.
// That is inherent to per-account throttling and is the reason the port is
// specified as a refilling budget rather than a lockout: the ceiling recovers
// on its own, so the worst an attacker achieves is a delay, never an account
// they can keep shut.
//
// # Availability
//
// A throttle is defence in depth, not an authorization decision. An
// implementation backed by something that can fail should fail *open* and say
// so in its own documentation — refusing logins because a throttle store is
// unreachable turns a hardening measure into an outage. This is the opposite of
// how a token-revocation check must behave.
package throttle

import "time"

//go:generate go tool mockgen -package=mocks -destination=../../../../../mocks/service/throttle.go -source=throttle.go Throttle

// Throttle bounds repeated failed attempts against a single key.
//
// Implementations must be safe for concurrent use.
type Throttle interface {
	// Attempt records an attempt against key and reports whether it may
	// proceed, and if not, how long until one can.
	//
	// It ALWAYS consumes one unit of budget, including on the attempt it
	// allows. Callers give it back with Succeed.
	Attempt(key string) (retryAfter time.Duration, allowed bool)

	// Succeed restores key's full budget. Callers invoke it after an attempt
	// that succeeded, so that only failures accumulate.
	Succeed(key string)
}
