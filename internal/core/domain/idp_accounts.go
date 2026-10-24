package domain

import "time"

// IDPAccount is an account linked to an identity provider, as the
// provider's identities list answers it: the account, when it was linked,
// and whether the provider is its only way in.
//
// OnlyWayIn is the provider's delete's own rule (no password, and no
// identity at another provider): the accounts a 409 on the delete is about,
// which it tells the administrator to delete or make local first. The list
// names them, which nothing did; the subject is not shown.
type IDPAccount struct {
	LinkedAt  time.Time
	User      User
	OnlyWayIn bool
}

// SelectIDPAccountsOutput is a page of a provider's accounts.
type SelectIDPAccountsOutput struct {
	Items     []IDPAccount
	Paginator Paginator
}
