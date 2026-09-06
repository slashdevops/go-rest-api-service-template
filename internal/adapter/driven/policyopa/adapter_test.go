package policyopa

import (
	"context"
	"testing"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driven/policyopa/rego"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/policy"
)

const (
	uid   = "01a02b03-0000-7000-8000-000000000001"
	other = "01a02b03-0000-7000-8000-000000000002"
	id9   = "01a02b03-0000-7000-8000-000000000009"
	id8   = "01a02b03-0000-7000-8000-000000000008"
)

// perms builds the map exactly as UsersRepository.SelectAuthz returns it and
// the cache round-trips it: the top-level "permissions" wrapper included. The
// first version of this test pre-unwrapped it and passed while every live
// request was refused; the fixture is the wire shape from now on.
func perms(user string, grants map[string]any) map[string]any {
	return map[string]any{"permissions": map[string]any{"users": map[string]any{user: grants}}}
}

// The permission map reaches the engine after a JSON round trip through the
// cache, so action lists are []any, not []string. The fixtures say so.
func actions(a ...string) []any {
	out := make([]any, len(a))
	for i, s := range a {
		out[i] = s
	}
	return out
}

// TestEngineDecisions is the twin of policy_test.rego: the same cases,
// driven through the port the middleware uses. Change both.
func TestEngineDecisions(t *testing.T) {
	t.Parallel()

	engine, err := New(rego.RegoQuery, rego.RegoPolicy)
	if err != nil {
		t.Fatalf("the shipped policy does not compile: %v", err)
	}

	cases := []struct {
		name     string
		user     string
		grants   map[string]any
		action   string
		resource string
		want     bool
	}{
		{"exact_allow", uid, map[string]any{"/users": actions("GET", "PUT")}, "GET", "/users", true},
		{"exact_deny_method", uid, map[string]any{"/users": actions("GET", "PUT")}, "DELETE", "/users", false},
		{"exact_deny_other_path", uid, map[string]any{"/users": actions("GET")}, "GET", "/roles", false},
		{"exact_is_not_a_prefix", uid, map[string]any{"/users": actions("GET")}, "GET", "/users/" + id9, false},
		{"trailing_slash_is_a_different_path", uid, map[string]any{"/users": actions("GET")}, "GET", "/users/", false},
		{"wildcard_allows_a_uuid", uid, map[string]any{"/users/*": actions("GET")}, "GET", "/users/" + id9, true},
		{"wildcard_refuses_a_literal_segment", uid, map[string]any{"/users/*": actions("GET")}, "GET", "/users/me", false},
		{"wildcard_refuses_a_deeper_path", uid, map[string]any{"/roles/*": actions("GET")}, "GET", "/roles/" + id9 + "/users", false},
		{"wildcard_refuses_an_uppercase_uuid", uid, map[string]any{"/users/*": actions("GET")}, "GET", "/users/01A02B03-0000-7000-8000-000000000009", false},
		{"wildcard_honours_the_method", uid, map[string]any{"/users/*": actions("GET")}, "DELETE", "/users/" + id9, false},
		{"two_wildcards", uid, map[string]any{"/projects/*/products/*": actions("PUT")}, "PUT", "/projects/" + id9 + "/products/" + id8, true},
		{"two_wildcards_refuse_a_partial_path", uid, map[string]any{"/projects/*/products/*": actions("PUT")}, "PUT", "/projects/" + id9 + "/products", false},
		{"literal_uuid_in_a_grant_is_exact", uid, map[string]any{"/projects/" + id9 + "/products/*": actions("GET")}, "GET", "/projects/" + id9 + "/products/" + id8, true},
		{"literal_uuid_in_a_grant_refuses_another", uid, map[string]any{"/projects/" + id9 + "/products/*": actions("GET")}, "GET", "/projects/01a02b03-0000-7000-8000-000000000007/products/" + id8, false},
		{"star_action_on_a_path", uid, map[string]any{"/roles": actions("*")}, "DELETE", "/roles", true},
		{"star_action_on_a_wildcard_path", uid, map[string]any{"/roles/*": actions("*")}, "PUT", "/roles/" + id9, true},
		{"star_action_does_not_widen_the_path", uid, map[string]any{"/roles": actions("*")}, "GET", "/users", false},
		{"administrator", uid, map[string]any{"*": actions("*")}, "DELETE", "/anything/at/all", true},
		{"administrator_survives_a_second_global_policy", uid, map[string]any{"*": actions("*", "GET")}, "DELETE", "/roles", true},
		{"global_method_grant", uid, map[string]any{"*": actions("GET")}, "GET", "/roles", true},
		{"global_method_grant_refuses_other_methods", uid, map[string]any{"*": actions("GET")}, "POST", "/roles", false},
		{"unknown_user", other, map[string]any{"*": actions("*")}, "GET", "/users", false},
		{"empty_grants", uid, map[string]any{}, "GET", "/users", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// The fixture always describes uid; an "unknown_user" case asks
			// about someone else.
			p := perms(uid, tc.grants)
			got, err := engine.IsAllowed(context.Background(), policy.Decision{
				UserID:      tc.user,
				Action:      tc.action,
				Resource:    tc.resource,
				Permissions: p,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tc.want {
				t.Fatalf("%s %s with %v: got %v, want %v", tc.action, tc.resource, tc.grants, got, tc.want)
			}
		})
	}
}

func TestEngineFailsClosed(t *testing.T) {
	t.Parallel()

	t.Run("an_unwrapped_map_is_accepted_too", func(t *testing.T) {
		t.Parallel()
		engine, _ := New(rego.RegoQuery, rego.RegoPolicy)
		got, err := engine.IsAllowed(context.Background(), policy.Decision{UserID: uid, Action: "GET", Resource: "/users",
			Permissions: map[string]any{"users": map[string]any{uid: map[string]any{"/users": actions("GET")}}}})
		if err != nil || !got {
			t.Fatalf("got %v, %v; want true", got, err)
		}
	})

	t.Run("nil_permissions_is_a_deny_not_an_error", func(t *testing.T) {
		t.Parallel()
		engine, _ := New(rego.RegoQuery, rego.RegoPolicy)
		got, err := engine.IsAllowed(context.Background(), policy.Decision{UserID: uid, Action: "GET", Resource: "/users"})
		if err != nil || got {
			t.Fatalf("got %v, %v; want false, nil", got, err)
		}
	})

	t.Run("a_module_that_does_not_compile_is_refused_at_construction", func(t *testing.T) {
		t.Parallel()
		if _, err := New(rego.RegoQuery, "package authorization\nallow if { input. }"); err == nil {
			t.Fatal("a broken policy must fail New, not the first request")
		}
	})

	t.Run("a_query_that_is_not_a_boolean_is_an_error", func(t *testing.T) {
		t.Parallel()
		engine, err := New("data.authorization.grants", rego.RegoPolicy)
		if err != nil {
			t.Fatal(err)
		}
		p := perms(uid, map[string]any{"/users": actions("GET")})
		got, err := engine.IsAllowed(context.Background(), policy.Decision{UserID: uid, Action: "GET", Resource: "/users", Permissions: p})
		if err == nil || got {
			t.Fatalf("a non-boolean answer must be an error and a deny; got %v, %v", got, err)
		}
	})
}
