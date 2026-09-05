// Package policy defines the driven port that use-cases consume to
// answer "is this user allowed to do this action on this resource?".
// The implementation lives in internal/adapter/driven/policyopa
// which evaluates a Rego policy module via OPA.
//
// Use-cases stay free of OPA, Rego, and in-memory store details. The
// permissions blob is opaque from the port's perspective — the
// adapter decides how to feed it to the engine.
package policy

import "context"

//go:generate go tool mockgen -package=mocks -destination=../../../../../mocks/service/policy.go -source=policy.go Engine

// Decision is the input the policy engine evaluates.
type Decision struct {
	Permissions map[string]any
	UserID      string
	Action      string
	Resource    string
}

// Engine is the driven port consumed by use-cases.
type Engine interface {
	// IsAllowed returns true if the configured policy allows the
	// decision. A non-nil error indicates the policy could not be
	// evaluated (mis-configured query, malformed input, …) and the
	// caller should treat the request as denied.
	IsAllowed(ctx context.Context, decision Decision) (bool, error)
}
