// Package policyopa is the driven adapter that satisfies the
// policy.Engine port using Open Policy Agent's Rego evaluator. The
// adapter holds the prepared query string and policy module once at
// construction; each IsAllowed call builds a per-decision in-memory
// store and runs Eval.
package policyopa

import (
	"context"
	"fmt"

	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/storage/inmem"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/policy"
)

// Engine implements policy.Engine.
type Engine struct {
	query  string
	module string
}

// New constructs an Engine bound to the given Rego query expression
// and policy module. Both must be non-empty.
func New(query, module string) (*Engine, error) {
	if query == "" {
		return nil, &domain.InvalidRegoQueryError{Message: "rego query cannot be empty"}
	}
	if module == "" {
		return nil, &domain.InvalidRegoPolicyError{Message: "rego policy cannot be empty"}
	}
	return &Engine{query: query, module: module}, nil
}

// IsAllowed implements policy.Engine.
func (e *Engine) IsAllowed(ctx context.Context, decision policy.Decision) (bool, error) {
	perms := decision.Permissions
	if perms == nil {
		perms = map[string]any{}
	}

	input := map[string]any{
		"user_id":  decision.UserID,
		"action":   decision.Action,
		"resource": decision.Resource,
	}

	store := inmem.NewFromObject(perms)

	query, err := rego.New(
		rego.Query(e.query),
		rego.Module("policy.rego", e.module),
		rego.Input(input),
		rego.Store(store),
		rego.EnablePrintStatements(true),
	).PrepareForEval(ctx)
	if err != nil {
		return false, &domain.UnauthorizedError{Message: err.Error()}
	}

	results, err := query.Eval(ctx)
	if err != nil {
		return false, &domain.UnauthorizedError{Message: err.Error()}
	}
	if len(results) == 0 {
		return false, &domain.UnauthorizedError{Message: "unauthorized: no results found"}
	}
	if len(results[0].Expressions) == 0 {
		return false, &domain.UnauthorizedError{Message: "unauthorized: no expressions found"}
	}

	allowed, ok := results[0].Expressions[0].Value.(bool)
	if !ok {
		return false, &domain.UnauthorizedError{
			Message: fmt.Sprintf("unauthorized: expression value is not a bool: %T", results[0].Expressions[0].Value),
		}
	}
	return allowed, nil
}
