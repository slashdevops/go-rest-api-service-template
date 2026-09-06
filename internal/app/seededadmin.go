package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
)

// seededAdminEmail and seededAdminHash are the row migration 005 inserts. The
// plaintext is in that file's comment, in the Swagger login example and in
// every integration test; anyone who has read the repository can sign in as
// this administrator on any deployment that kept it.
const (
	seededAdminEmail = "admin@qu3ry.me"
	seededAdminHash  = "$2a$10$IqIoI8R.vDCRQw5Pceq6w..qKdeklXJYCR5U0nJSvN4jTIaXzm8Gm"
)

// checkSeededAdmin refuses to start while the seeded administrator still has
// the seeded password, unless authn.seed.admin.password.allowed says this is a
// development stack. The comparison is on the stored hash, so no password is
// handled here.
//
// Refusing rather than warning is the point: the service warns about a dozen
// weak postures at startup already, and a warning is the posture that ships.
func (a *App) checkSeededAdmin(ctx context.Context) error {
	var hash string

	err := a.dbPool.QueryRow(ctx,
		`SELECT password_hash FROM users WHERE email = $1 AND admin = TRUE AND disabled = FALSE`,
		seededAdminEmail,
	).Scan(&hash)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil // removed or disabled: nothing to refuse
	case err != nil:
		return fmt.Errorf("checking the seeded administrator: %w", err)
	}

	if hash != seededAdminHash {
		return nil
	}

	if a.configs.Authn.SeedAdminPasswordAllowed.Value {
		slog.Warn("the seeded administrator still has the seeded password; authn.seed.admin.password.allowed keeps the service running. Never outside development",
			"email", seededAdminEmail)

		return nil
	}

	return fmt.Errorf("refusing to start: the administrator %q still has the password seeded by the migrations, which is public. Change it, or set authn.seed.admin.password.allowed=true on a development stack", seededAdminEmail)
}
