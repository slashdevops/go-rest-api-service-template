// Package policyopa is the driven adapter that satisfies the policy.Engine
// port with Open Policy Agent's Rego evaluator.
//
// The query is compiled once, in [New]: parsing and compiling a module is
// the expensive part of an evaluation, and it used to happen on every
// authenticated request. Each [Engine.IsAllowed] then evaluates the prepared
// query with the decision as input. The caller's permission set travels in
// input rather than in OPA's data document because it is request-scoped --
// data is for what is true for every evaluation -- and that is also what
// lets the query be prepared once with no store to rebuild.
package policyopa

import (
	"context"
	"fmt"

	"github.com/open-policy-agent/opa/v1/rego"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/policy"
)

// Engine implements policy.Engine.
type Engine struct {
	prepared rego.PreparedEvalQuery
}

// New compiles the query against the module once. A module that does not
// compile is a start-up failure, not a per-request 500.
func New(query, module string) (*Engine, error) {
	if query == "" {
		return nil, &domain.InvalidRegoQueryError{Message: "rego query cannot be empty"}
	}

	if module == "" {
		return nil, &domain.InvalidRegoPolicyError{Message: "rego policy cannot be empty"}
	}

	prepared, err := rego.New(
		rego.Query(query),
		rego.Module("policy.rego", module),
	).PrepareForEval(context.Background())
	if err != nil {
		return nil, &domain.InvalidRegoPolicyError{Message: fmt.Sprintf("compiling the authorization policy: %v", err)}
	}

	return &Engine{prepared: prepared}, nil
}

// IsAllowed implements policy.Engine.
//
// Every failure to reach a boolean is an error, and the port says an error
// means denied: an unevaluable policy must not admit anyone.
func (e *Engine) IsAllowed(ctx context.Context, decision policy.Decision) (bool, error) {
	// The use case hands over the map exactly as the repository builds it,
	// with its top-level "permissions" key -- the shape the old data-root
	// evaluation expected. The policy reads input.permissions.users, so
	// that wrapper is stripped here, once, rather than at every producer.
	perms := decision.Permissions
	if inner, ok := perms["permissions"].(map[string]any); ok {
		perms = inner
	}

	if perms == nil {
		perms = map[string]any{}
	}

	input := map[string]any{
		"user_id":     decision.UserID,
		"action":      decision.Action,
		"resource":    decision.Resource,
		"permissions": perms,
	}

	results, err := e.prepared.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return false, &domain.UnauthorizedError{Message: err.Error()}
	}

	if len(results) == 0 || len(results[0].Expressions) == 0 {
		return false, &domain.UnauthorizedError{Message: "unauthorized: the policy produced no result"}
	}

	allowed, ok := results[0].Expressions[0].Value.(bool)
	if !ok {
		return false, &domain.UnauthorizedError{
			Message: fmt.Sprintf("unauthorized: expression value is not a bool: %T", results[0].Expressions[0].Value),
		}
	}

	return allowed, nil
}
