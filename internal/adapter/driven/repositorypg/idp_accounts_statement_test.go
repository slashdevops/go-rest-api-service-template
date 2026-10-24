package repositorypg

import (
	"strings"
	"testing"
)

// A provider's accounts list says which of them are the 409's of the
// provider's delete: only_way_in is the delete's own rule (queryDeleteIDP),
// an account with no password and no identity at another provider. The
// delete's predicate is taken out of its statement whole, its aliases
// renamed to the list's, and compared with the list's column: a condition
// added to either, or a change to the delete's shape around it, fails here
// rather than as a list that names the wrong accounts.
func TestAProvidersAccountsAreMarkedByTheDeletesRule(t *testing.T) {
	t.Parallel()

	flat := func(s string) string { return strings.Join(strings.Fields(s), " ") }

	const (
		deleteHead = "DELETE FROM idps WHERE id = $1 AND NOT EXISTS ( SELECT 1 FROM users_identities AS only_way " +
			"JOIN users ON users.id = only_way.users_id WHERE only_way.idps_id = $1 AND "
		deleteTail = " );"
		listHead   = ", ui.linked_at, ("
		listTail   = ") AS only_way_in"
	)

	statement := flat(queryDeleteIDP)
	if !strings.HasPrefix(statement, deleteHead) || !strings.HasSuffix(statement, deleteTail) {
		t.Fatalf("the delete's statement changed around its rule; only_way_in must be read against it again:\n%s", statement)
	}

	column := flat(idpAccountColumns)
	if !strings.HasPrefix(column, listHead) || !strings.HasSuffix(column, listTail) {
		t.Fatalf("the accounts' columns changed shape:\n%s", column)
	}

	// The delete's rule, as the list writes it: the account is usrs, the
	// provider the identity's.
	deletes := strings.NewReplacer(
		"users.local_account", "usrs.local_account",
		"only_way.users_id", "usrs.id",
		"$1", "ui.idps_id",
	).Replace(strings.TrimSuffix(strings.TrimPrefix(statement, deleteHead), deleteTail))

	if lists := strings.TrimSuffix(strings.TrimPrefix(column, listHead), listTail); lists != deletes {
		t.Errorf("only_way_in is not the delete's rule:\n list: %s\n delete: %s", lists, deletes)
	}
}
