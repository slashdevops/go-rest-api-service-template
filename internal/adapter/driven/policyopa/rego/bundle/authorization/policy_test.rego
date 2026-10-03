package authorization_test

import data.authorization

# Every case here has a twin in the Go table test (adapter_test.go), which
# drives the same fixtures through policy.Engine. Change both.

uid := "01a02b03-0000-7000-8000-000000000001"

other := "01a02b03-0000-7000-8000-000000000002"

# request is the input document for one decision: user asks to perform action
# on resource, holding perms. It only builds the document; each test applies
# it with `with input as`, so the helper reads nothing but its arguments.
request(user, perms, action, resource) := {
	"user_id": user,
	"action": action,
	"resource": resource,
	"permissions": {"users": {user: perms}},
}

# --- exact paths ---

test_exact_allow if {
	authorization.allow with input as request(uid, {"/users": ["GET", "PUT"]}, "GET", "/users")
}

test_exact_deny_method if {
	not authorization.allow with input as request(uid, {"/users": ["GET", "PUT"]}, "DELETE", "/users")
}

test_exact_deny_other_path if {
	not authorization.allow with input as request(uid, {"/users": ["GET"]}, "GET", "/roles")
}

test_exact_is_not_a_prefix if {
	path := "/users/01a02b03-0000-7000-8000-000000000009"
	not authorization.allow with input as request(uid, {"/users": ["GET"]}, "GET", path)
}

test_trailing_slash_is_a_different_path if {
	not authorization.allow with input as request(uid, {"/users": ["GET"]}, "GET", "/users/")
}

# --- "*" in a path means one uuid segment ---

test_wildcard_allows_a_uuid if {
	path := "/users/01a02b03-0000-7000-8000-000000000009"
	authorization.allow with input as request(uid, {"/users/*": ["GET"]}, "GET", path)
}

test_wildcard_refuses_a_literal_segment if {
	not authorization.allow with input as request(uid, {"/users/*": ["GET"]}, "GET", "/users/me")
}

test_wildcard_refuses_a_deeper_path if {
	path := "/roles/01a02b03-0000-7000-8000-000000000009/users"
	not authorization.allow with input as request(uid, {"/roles/*": ["GET"]}, "GET", path)
}

test_wildcard_refuses_an_uppercase_uuid if {
	path := "/users/01A02B03-0000-7000-8000-000000000009"
	not authorization.allow with input as request(uid, {"/users/*": ["GET"]}, "GET", path)
}

test_wildcard_honours_the_method if {
	path := "/users/01a02b03-0000-7000-8000-000000000009"
	not authorization.allow with input as request(uid, {"/users/*": ["GET"]}, "DELETE", path)
}

test_two_wildcards if {
	grant := {"/projects/*/products/*": ["PUT"]}
	path := "/projects/01a02b03-0000-7000-8000-000000000009/products/01a02b03-0000-7000-8000-000000000008"
	authorization.allow with input as request(uid, grant, "PUT", path)
}

test_two_wildcards_refuse_a_partial_path if {
	grant := {"/projects/*/products/*": ["PUT"]}
	path := "/projects/01a02b03-0000-7000-8000-000000000009/products"
	not authorization.allow with input as request(uid, grant, "PUT", path)
}

test_literal_uuid_in_a_grant_is_exact if {
	grant := {"/projects/01a02b03-0000-7000-8000-000000000009/products/*": ["GET"]}
	path := "/projects/01a02b03-0000-7000-8000-000000000009/products/01a02b03-0000-7000-8000-000000000008"
	authorization.allow with input as request(uid, grant, "GET", path)
}

test_literal_uuid_in_a_grant_refuses_another if {
	grant := {"/projects/01a02b03-0000-7000-8000-000000000009/products/*": ["GET"]}
	path := "/projects/01a02b03-0000-7000-8000-000000000007/products/01a02b03-0000-7000-8000-000000000008"
	not authorization.allow with input as request(uid, grant, "GET", path)
}

# --- "*" as an action means every method, wherever it appears ---

test_star_action_on_a_path if {
	authorization.allow with input as request(uid, {"/roles": ["*"]}, "DELETE", "/roles")
}

test_star_action_on_a_wildcard_path if {
	path := "/roles/01a02b03-0000-7000-8000-000000000009"
	authorization.allow with input as request(uid, {"/roles/*": ["*"]}, "PUT", path)
}

test_star_action_does_not_widen_the_path if {
	not authorization.allow with input as request(uid, {"/roles": ["*"]}, "GET", "/users")
}

# --- the global resource ---

test_administrator if {
	authorization.allow with input as request(uid, {"*": ["*"]}, "DELETE", "/anything/at/all")
}

test_administrator_survives_a_second_global_policy if {
	authorization.allow with input as request(uid, {"*": ["*", "GET"]}, "DELETE", "/roles")
}

test_global_method_grant if {
	authorization.allow with input as request(uid, {"*": ["GET"]}, "GET", "/roles")
}

test_global_method_grant_refuses_other_methods if {
	not authorization.allow with input as request(uid, {"*": ["GET"]}, "POST", "/roles")
}

# --- nothing ---

test_unknown_user if not authorization.allow with input as {
	"user_id": other,
	"action": "GET",
	"resource": "/users",
	"permissions": {"users": {uid: {"*": ["*"]}}},
}

test_empty_grants if {
	not authorization.allow with input as request(uid, {}, "GET", "/users")
}

test_no_permissions_document if {
	not authorization.allow with input as {"user_id": uid, "action": "GET", "resource": "/users"}
}
