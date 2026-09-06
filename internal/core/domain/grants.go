package domain

// Grant is one thing a policy allows: an action on a resource pattern.
type Grant struct {
	Action   string
	Resource string
}

func (g Grant) String() string { return g.Action + " " + g.Resource }

// GrantNotHeldError is the refusal of a grant the caller does not hold
// themselves. It is the guard against privilege escalation: with the four
// grants POST /policies, POST /roles, POST /roles/*/policies and
// POST /users/*/roles a regular account minted an allow-all policy, a role,
// attached both to itself and became an administrator (measured 2026-09-06).
// A caller may hand out only what they already hold.
type GrantNotHeldError struct {
	Grant Grant
}

func (e *GrantNotHeldError) Error() string {
	return "grant refused: the caller does not hold " + e.Grant.String()
}
