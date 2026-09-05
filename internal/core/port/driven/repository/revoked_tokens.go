package repository

import (
	"context"
	"time"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

//go:generate go tool mockgen -package=mocks -destination=../../../../../mocks/service/revoked_tokens.go -source=revoked_tokens.go RevokedTokens

// RevokedTokens is the driven persistence port for token revocation.
//
// # Why this is a repository and not a cache
//
// Revocation is the one lookup in the service that must not fail open. The
// cache layer's documented invariant is that a fault never fails a request,
// which for this question means answering "not revoked" when the truth is
// unknown — and `cache.enabled=false` is a supported configuration, where a
// cache-backed denylist would not exist at all and logout would go back to
// doing nothing. Postgres is a hard dependency of the service, so the denylist
// has the same availability as the service. A cache may sit in front of this,
// but only as an optimisation: a miss falls through to the truth.
//
// # A denylist, not a session store
//
// A token with no row here is valid. Absence means valid, which is what keeps
// this store from being able to lock anybody out: losing it forgets
// revocations, it does not invalidate sessions.
//
// # Two ways a token stops working, and the difference matters
//
// [RevokedTokens.Revoke] ends a token outright — a logout. [RevokedTokens.Rotate]
// records that a token was *spent*: consumed by a refresh that issued a
// successor in its place. Both refuse the token, but only the second says
// another token took over from it, and that is what makes a replay legible.
// Presenting a revoked token is an ordinary logged-out client; presenting a
// rotated one means two parties hold a credential only one of them should have.
type RevokedTokens interface {
	// Revoke records jti as revoked until expiresAt, which should be the
	// token's own exp: past that instant the token is refused for being
	// expired and the row is no longer load-bearing.
	//
	// Revoking an already-revoked token is not an error. Logging out twice, or
	// two tabs logging out at once, must both succeed.
	//
	// tokenType says what kind of token the row names. It is what lets the
	// access-token mirror select exactly its own rows without a horizon on
	// expires_at -- see [RevokedTokens.SelectUnexpiredJTIs] for why the horizon
	// stopped being safe.
	Revoke(ctx context.Context, jti, userID uuid.UUID, tokenType domain.TokenType, expiresAt time.Time) error

	// Rotate records that oldJTI was spent on a refresh which issued newJTI in
	// its place. The old token is refused from this point on, and the link to
	// its successor is what [RevokedTokens.RevokeChain] follows.
	//
	// Like Revoke, this must tolerate being called twice for the same oldJTI:
	// two requests racing on one token both rotate it, and the first successor
	// recorded is the one that counts. It must NOT overwrite an existing
	// successor — that link is the only record of where the chain went, and
	// losing it would strand the live token beyond the reach of the walk.
	Rotate(ctx context.Context, oldJTI, newJTI, userID uuid.UUID, expiresAt time.Time) error

	// Consume records jti as spent and reports whether THIS call is the one
	// that spent it. A second call for the same jti reports false, which is how
	// a single-use token detects its own replay.
	//
	// It is one statement rather than a read followed by a write, and that is
	// the point: two callbacks arriving together would both pass a separate
	// check and both proceed, which is exactly the replay the check exists to
	// stop. The insert itself is the check.
	//
	// userID may be uuid.Nil() for a token that names no account — an OAuth
	// state token's subject is the event that started the flow, not a user, and
	// during a registration flow no user exists yet.
	Consume(ctx context.Context, jti, userID uuid.UUID, tokenType domain.TokenType, expiresAt time.Time) (bool, error)

	// Get returns what is known about jti, or nil when the token has not been
	// revoked and has not been spent — the ordinary case.
	//
	// An error here must be treated as fatal by the caller, never as "not
	// revoked" — that is the fail-closed half of the contract.
	Get(ctx context.Context, jti uuid.UUID) (*domain.TokenRevocation, error)

	// RevokeChain follows the rotation chain from jti to its end and revokes
	// the token at the tip — the one link nobody has spent yet, and therefore
	// the only one still usable. It returns the tip it revoked, or uuid.Nil()
	// when the chain had already been fully revoked.
	//
	// This is the answer to a detected replay. It ends the session for the
	// legitimate holder as well as for whoever copied the token, because
	// nothing in the request distinguishes them.
	RevokeChain(ctx context.Context, jti, userID uuid.UUID, expiresAt time.Time) (uuid.UUID, error)

	// SelectUnexpiredJTIs returns every jti of the given type that is still
	// refused: revoked, and not yet past its own expiry. It is what the
	// access-token mirror rebuilds itself from.
	//
	// # Why it selects by TYPE and not by a horizon on expires_at
	//
	// It used to take a horizon equal to the access-token lifetime and return
	// every row expiring inside it. That was exact only while the lifetime was
	// a startup constant shorter than any refresh token: the table takes a row
	// per refresh rotation, and the short window is what kept those out. With
	// the lifetime a runtime setting that may be raised to 48h the trick fails
	// twice -- a raise leaves a horizon shorter than the tokens it must cover,
	// silently omitting revocations, and a 24h access lifetime admits every
	// rotation row, growing the set to "rotations in the last day". Naming the
	// type on the row removes the coupling: the mirror asks for exactly the
	// unexpired ACCESS rows, whatever the lifetime is or becomes.
	//
	// A credential that is neither -- a personal access token, up to a year --
	// is not covered by a mirror at all and is revoked by deleting its row,
	// checked directly.
	SelectUnexpiredJTIs(ctx context.Context, tokenType domain.TokenType) ([]uuid.UUID, error)

	// DeleteExpired removes rows whose token has expired on its own and returns
	// how many went. It is safe to call at any time and from anywhere.
	//
	// Rotation makes this load-bearing rather than tidy: the table takes a row
	// per refresh, so nothing calling this means unbounded growth.
	DeleteExpired(ctx context.Context) (int64, error)
}
