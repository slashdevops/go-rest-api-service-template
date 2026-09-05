package usecase

import (
	"testing"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// TestWithoutCredentials guards the boundary that keeps bcrypt hashes out of
// the cache. Everything GetByEmail and GetByID return is written to Valkey — a
// store outside the database, with a twelve hour TTL and, unless TLS is
// enabled, a cleartext connection.
func TestWithoutCredentials(t *testing.T) {
	t.Parallel()

	t.Run("clears both credential fields", func(t *testing.T) {
		t.Parallel()

		got := withoutCredentials(&domain.User{
			Email:        "ada@example.com",
			Password:     "hunter2",
			PasswordHash: "$2a$10$abcdefghijklmnopqrstuv",
		})

		if got.PasswordHash != "" {
			t.Errorf("PasswordHash survived as %q", got.PasswordHash)
		}

		if got.Password != "" {
			t.Errorf("Password survived as %q", got.Password)
		}
	})

	t.Run("keeps everything else", func(t *testing.T) {
		t.Parallel()

		yes := true
		in := &domain.User{
			Email: "ada@example.com", FirstName: "Ada", LastName: "Lovelace",
			Admin: &yes, PasswordHash: "$2a$10$abc",
		}

		got := withoutCredentials(in)

		if got.Email != in.Email || got.FirstName != in.FirstName || got.LastName != in.LastName {
			t.Errorf("identity fields changed: %+v", got)
		}

		if got.Admin == nil || !*got.Admin {
			t.Error("Admin was lost; the cached user still has to answer the questions it is read for")
		}
	})

	t.Run("does not mutate the original", func(t *testing.T) {
		t.Parallel()

		// The value belongs to whoever called the repository. Clearing in place
		// would surprise anyone still holding a reference — including the
		// caller that needs the hash.
		in := &domain.User{PasswordHash: "$2a$10$abc", Password: "hunter2"}

		_ = withoutCredentials(in)

		if in.PasswordHash == "" || in.Password == "" {
			t.Error("the original was cleared in place")
		}
	})

	t.Run("nil is safe", func(t *testing.T) {
		t.Parallel()

		if withoutCredentials(nil) != nil {
			t.Error("nil should stay nil")
		}
	})
}
