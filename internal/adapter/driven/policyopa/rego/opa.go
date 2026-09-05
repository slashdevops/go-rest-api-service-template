// Package rego embeds and serves the OPA Rego policy bundle that the
// policyopa adapter evaluates for authorization checks.
package rego

import _ "embed"

//go:embed bundle/authorization/policy.rego
var RegoPolicy string

// RegoQuery is the query used to check if a user is allowed to perform an action.
// this is the variable defined in the policy.rego file
// used to check if the user is allowed to perform the action
var RegoQuery = `data.authorization.allow`
