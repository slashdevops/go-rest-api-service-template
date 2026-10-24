package repositorypg

import (
	"strings"
	"testing"
)

// A provider's accounts list says which of them are the 409's of the
// provider's delete: only_way_in is the delete's own rule (queryDeleteIDP),
// an account with no password and no identity at another provider. The two
// are held to one another here, spaces apart, so that a change to either
// shows in the pull request gate rather than as a list that names the wrong
// accounts.
func TestAProvidersAccountsAreMarkedByTheDeletesRule(t *testing.T) {
	t.Parallel()

	flat := func(s string) string { return strings.Join(strings.Fields(s), " ") }

	const want = ", ui.linked_at, (usrs.local_account IS NOT TRUE AND NOT EXISTS ( SELECT 1 FROM users_identities AS other " +
		"WHERE other.users_id = usrs.id AND other.idps_id <> ui.idps_id )) AS only_way_in"
	if got := flat(idpAccountColumns); got != want {
		t.Errorf("the accounts' columns changed:\n got: %s\nwant: %s", got, want)
	}

	// The delete's rule, as the list writes it: the same two terms.
	for _, term := range []string{
		"users.local_account IS NOT TRUE",
		"SELECT 1 FROM users_identities AS other WHERE other.users_id = only_way.users_id AND other.idps_id <> $1",
	} {
		if !strings.Contains(flat(queryDeleteIDP), term) {
			t.Errorf("the delete's rule no longer has %q: only_way_in says what it no longer does", term)
		}
	}
}
