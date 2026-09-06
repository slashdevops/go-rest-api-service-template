package authorization_test

import data.authorization

# Every case here has a twin in the Go table test (adapter_test.go), which
# drives the same fixtures through policy.Engine. Change both.

uid := "01a02b03-0000-7000-8000-000000000001"

other := "01a02b03-0000-7000-8000-000000000002"

decide(perms, action, resource) if {
	authorization.allow with input as {
		"user_id": uid,
		"action": action,
		"resource": resource,
		"permissions": {"users": {uid: perms}},
	}
}

# --- exact paths ---

test_exact_allow if decide({"/users": ["GET", "PUT"]}, "GET", "/users")

test_exact_deny_method if not decide({"/users": ["GET", "PUT"]}, "DELETE", "/users")

test_exact_deny_other_path if not decide({"/users": ["GET"]}, "GET", "/roles")

test_exact_is_not_a_prefix if not decide({"/users": ["GET"]}, "GET", "/users/01a02b03-0000-7000-8000-000000000009")

test_trailing_slash_is_a_different_path if not decide({"/users": ["GET"]}, "GET", "/users/")

# --- "*" in a path means one uuid segment ---

test_wildcard_allows_a_uuid if decide({"/users/*": ["GET"]}, "GET", "/users/01a02b03-0000-7000-8000-000000000009")

test_wildcard_refuses_a_literal_segment if not decide({"/users/*": ["GET"]}, "GET", "/users/me")

test_wildcard_refuses_a_deeper_path if not decide({"/roles/*": ["GET"]}, "GET", "/roles/01a02b03-0000-7000-8000-000000000009/users")

test_wildcard_refuses_an_uppercase_uuid if not decide({"/users/*": ["GET"]}, "GET", "/users/01A02B03-0000-7000-8000-000000000009")

test_wildcard_honours_the_method if not decide({"/users/*": ["GET"]}, "DELETE", "/users/01a02b03-0000-7000-8000-000000000009")

test_two_wildcards if decide({"/projects/*/products/*": ["PUT"]}, "PUT", "/projects/01a02b03-0000-7000-8000-000000000009/products/01a02b03-0000-7000-8000-000000000008")

test_two_wildcards_refuse_a_partial_path if not decide({"/projects/*/products/*": ["PUT"]}, "PUT", "/projects/01a02b03-0000-7000-8000-000000000009/products")

test_literal_uuid_in_a_grant_is_exact if decide({"/projects/01a02b03-0000-7000-8000-000000000009/products/*": ["GET"]}, "GET", "/projects/01a02b03-0000-7000-8000-000000000009/products/01a02b03-0000-7000-8000-000000000008")

test_literal_uuid_in_a_grant_refuses_another if not decide({"/projects/01a02b03-0000-7000-8000-000000000009/products/*": ["GET"]}, "GET", "/projects/01a02b03-0000-7000-8000-000000000007/products/01a02b03-0000-7000-8000-000000000008")

# --- "*" as an action means every method, wherever it appears ---

test_star_action_on_a_path if decide({"/roles": ["*"]}, "DELETE", "/roles")

test_star_action_on_a_wildcard_path if decide({"/roles/*": ["*"]}, "PUT", "/roles/01a02b03-0000-7000-8000-000000000009")

test_star_action_does_not_widen_the_path if not decide({"/roles": ["*"]}, "GET", "/users")

# --- the global resource ---

test_administrator if decide({"*": ["*"]}, "DELETE", "/anything/at/all")

test_administrator_survives_a_second_global_policy if decide({"*": ["*", "GET"]}, "DELETE", "/roles")

test_global_method_grant if decide({"*": ["GET"]}, "GET", "/roles")

test_global_method_grant_refuses_other_methods if not decide({"*": ["GET"]}, "POST", "/roles")

# --- nothing ---

test_unknown_user if not authorization.allow with input as {
	"user_id": other,
	"action": "GET",
	"resource": "/users",
	"permissions": {"users": {uid: {"*": ["*"]}}},
}

test_empty_grants if not decide({}, "GET", "/users")

test_no_permissions_document if not authorization.allow with input as {"user_id": uid, "action": "GET", "resource": "/users"}
