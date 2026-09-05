package authorization_test

import data.authorization

# --- Test Data ---

permissions_user1_exact := {"users": {"user1": {"/users": [
	"PUT",
	"DELETE",
	"GET",
]}}}

permissions_user2_wildcard := {"users": {"user2": {"/users/*": [
	"PUT",
	"DELETE",
	"GET",
	"POST",
]}}}

permissions_user3_nested_wildcard := {"users": {"user3": {"/users/*/posts/*": [
	"PUT",
	"DELETE",
	"GET",
]}}}

permissions_user4_multi_wildcard := {"users": {"user4": {
	"/users/user4_id/posts/*": ["PUT", "DELETE", "GET"],
	"/groups/*/posts/*": ["PUT", "DELETE", "GET"],
}}}

permissions_user5_super_admin := {"users": {"user5": {"*": ["*"]}}}

permissions_user6_global_get := {"users": {"user6": {"*": ["GET"]}}}

permissions_empty := {"users": {}}

permissions_user7_mixed_wildcard := {"users": {"user7": {
	"/projects/a3a19444-2632-4f1c-8907-016b2260828b/items/*": ["GET", "POST"],
	"/users/*/settings/5a4c4cad-7652-450f-b5e0-97905ad1d074": ["PUT"],
}}}

# --- Test Cases ---

# Test 1: Exact resource match
test_exact_resource_allow if {
	methods := ["GET", "PUT", "DELETE"]
	every method in methods {
		authorization.allow with input as {
			"user_id": "user1",
			"action": method,
			"resource": "/users",
		}
			with data.permissions as permissions_user1_exact
	}
}

test_exact_resource_deny_method if {
	methods := ["POST", "PATCH", "OPTIONS", "HEAD"]
	every method in methods {
		not authorization.allow with input as {
			"user_id": "user1",
			"action": method,
			"resource": "/users",
		}
			with data.permissions as permissions_user1_exact
	}
}

test_exact_resource_deny_path if {
	not authorization.allow with input as {
		"user_id": "user1",
		"action": "GET",
		"resource": "/users/other", # Path doesn't match exactly
	}
		with data.permissions as permissions_user1_exact
}

# Test 2: Single wildcard resource match
test_wildcard_resource_allow if {
	methods := ["GET", "PUT", "DELETE", "POST"]
	every method in methods {
		authorization.allow with input as {
			"user_id": "user2",
			"action": method,
			"resource": "/users/4359b3cc-2d8d-4c4f-a2e6-63b11215c92f",
		}
			with data.permissions as permissions_user2_wildcard
	}
}

test_wildcard_resource_deny_method if {
	methods := ["PATCH", "OPTIONS", "HEAD"]
	every method in methods {
		not authorization.allow with input as {
			"user_id": "user2",
			"action": method,
			"resource": "/users/4359b3cc-2d8d-4c4f-a2e6-63b11215c92f",
		}
			with data.permissions as permissions_user2_wildcard
	}
}

test_wildcard_resource_deny_path_non_match if {
	not authorization.allow with input as {
		"user_id": "user2",
		"action": "GET",
		"resource": "/projects/0d92baa3-07d8-4423-8d1c-46d723845bb0", # Path doesn't match pattern
	}
		with data.permissions as permissions_user2_wildcard
}

# Test 3: Nested wildcard resource match
test_nested_wildcard_resource_allow if {
	methods := ["GET", "PUT", "DELETE"]
	every method in methods {
		authorization.allow with input as {
			"user_id": "user3",
			"action": method,
			"resource": "/users/c7560cfb-56d6-4996-b330-c3e4afb5d071/posts/5a4c4cad-7652-450f-b5e0-97905ad1d074",
		}
			with data.permissions as permissions_user3_nested_wildcard
	}
}

test_nested_wildcard_resource_deny_method if {
	methods := ["POST", "PATCH", "OPTIONS", "HEAD"]
	every method in methods {
		not authorization.allow with input as {
			"user_id": "user3",
			"action": method,
			"resource": "/users/c7560cfb-56d6-4996-b330-c3e4afb5d071/posts/5a4c4cad-7652-450f-b5e0-97905ad1d074",
		}
			with data.permissions as permissions_user3_nested_wildcard
	}
}

test_nested_wildcard_resource_deny_path_partial_match if {
	not authorization.allow with input as {
		"user_id": "user3",
		"action": "GET",
		"resource": "/users/c7560cfb-56d6-4996-b330-c3e4afb5d071/comments/519b4cec-f42c-4281-956d-81fac04ba6c1",
	}
		with data.permissions as permissions_user3_nested_wildcard
}

# Test 4: Multiple wildcard resource definitions
test_multi_wildcard_resource_allow_first if {
	methods := ["GET", "PUT", "DELETE"]
	every method in methods {
		authorization.allow with input as {
			"user_id": "user4",
			"action": method,
			"resource": "/users/user4_id/posts/16d62901-58a2-4e67-94cd-255592cbdfd8",
		}
			with data.permissions as permissions_user4_multi_wildcard
	}
}

test_multi_wildcard_resource_allow_second if {
	methods := ["GET", "PUT", "DELETE"]
	every method in methods {
		authorization.allow with input as {
			"user_id": "user4",
			"action": method,
			"resource": "/groups/71d2d2d5-7b08-40a2-9242-25dc768b5c0e/posts/519b4cec-f42c-4281-956d-81fac04ba6c1",
		}
			with data.permissions as permissions_user4_multi_wildcard
	}
}

test_multi_wildcard_resource_deny_method if {
	methods := ["POST", "PATCH", "OPTIONS", "HEAD"]
	every method in methods {
		not authorization.allow with input as {
			"user_id": "user4",
			"action": method,
			"resource": "/users/user4_id/posts/16d62901-58a2-4e67-94cd-255592cbdfd8", # Check against first pattern
		}
			with data.permissions as permissions_user4_multi_wildcard
		not authorization.allow with input as {
			"user_id": "user4",
			"action": method,
			"resource": "/groups/71d2d2d5-7b08-40a2-9242-25dc768b5c0e/posts/519b4cec-f42c-4281-956d-81fac04ba6c1",
		}
			with data.permissions as permissions_user4_multi_wildcard
	}
}

test_multi_wildcard_resource_deny_path if {
	not authorization.allow with input as {
		"user_id": "user4",
		"action": "GET",
		"resource": "/users/another_user/posts/4359b3cc-2d8d-4c4f-a2e6-63b11215c92f", # Doesn't match any pattern
	}
		with data.permissions as permissions_user4_multi_wildcard
}

# Test 5: Super Admin ("*": ["*"])
test_super_admin_allow_any_action_any_resource if {
	methods := ["GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD", "ANYTHING"]
	resources := [
		"/users",
		"/users/4359b3cc-2d8d-4c4f-a2e6-63b11215c92f",
		"/projects/c7560cfb-56d6-4996-b330-c3e4afb5d071/items/5a4c4cad-7652-450f-b5e0-97905ad1d074",
		"/",
		"*",
	]
	every method in methods {
		every resource in resources {
			authorization.allow with input as {
				"user_id": "user5",
				"action": method,
				"resource": resource,
			}
				with data.permissions as permissions_user5_super_admin
		}
	}
}

# Test 6: Global Action ("*": [actions])
test_global_action_allow_get_any_resource if {
	resources := [
		"/users",
		"/users/4359b3cc-2d8d-4c4f-a2e6-63b11215c92f",
		"/projects/c7560cfb-56d6-4996-b330-c3e4afb5d071/items/5a4c4cad-7652-450f-b5e0-97905ad1d074",
		"/",
		"*",
	]
	every resource in resources {
		authorization.allow with input as {
			"user_id": "user6",
			"action": "GET",
			"resource": resource,
		}
			with data.permissions as permissions_user6_global_get
	}
}

test_global_action_deny_other_actions if {
	methods := ["POST", "PUT", "DELETE", "PATCH"]
	resources := [
		"/users/4359b3cc-2d8d-4c4f-a2e6-63b11215c92f",
		"/projects/c7560cfb-56d6-4996-b330-c3e4afb5d071",
	]
	every method in methods {
		every resource in resources {
			not authorization.allow with input as {
				"user_id": "user6",
				"action": method,
				"resource": resource,
			}
				with data.permissions as permissions_user6_global_get
		}
	}
}

# Test 7: User not found or no permissions
test_deny_if_user_not_in_permissions if {
	not authorization.allow with input as {
		"user_id": "unknown_user",
		"action": "GET",
		"resource": "/users",
	} # Use any non-empty map
		with data.permissions as permissions_user1_exact
}

test_deny_if_permissions_empty if {
	not authorization.allow with input as {
		"user_id": "any_user",
		"action": "GET",
		"resource": "/any/resource",
	}
		with data.permissions as permissions_empty
}

# Test 8: Deny if resource does not match any rule (exact or wildcard)
test_deny_if_no_rule_matches_resource if {
	not authorization.allow with input as {
		"user_id": "user1", # Has rule for /users exactly
		"action": "GET",
		"resource": "/projects", # No rule for /projects
	}
		with data.permissions as permissions_user1_exact

	not authorization.allow with input as {
		"user_id": "user2", # Has rule for /users/*
		"action": "GET",
		"resource": "/groups/71d2d2d5-7b08-40a2-9242-25dc768b5c0e", # No rule for /groups/*
	}
		with data.permissions as permissions_user2_wildcard
}

# Test 9: Mixed UUID and Wildcard in Resource Path
test_mixed_wildcard_project_items_allow if {
	methods := ["GET", "POST"]
	every method in methods {
		authorization.allow with input as {
			"user_id": "user7",
			"action": method,
			"resource": "/projects/a3a19444-2632-4f1c-8907-016b2260828b/items/357256d7-c071-4f7f-acc7-3cdf299034d3",
		}
			with data.permissions as permissions_user7_mixed_wildcard
	}
}

test_mixed_wildcard_project_items_deny_method if {
	methods := ["PUT", "DELETE", "PATCH"]
	every method in methods {
		not authorization.allow with input as {
			"user_id": "user7",
			"action": method,
			"resource": "/projects/a3a19444-2632-4f1c-8907-016b2260828b/items/357256d7-c071-4f7f-acc7-3cdf299034d3",
		}
			with data.permissions as permissions_user7_mixed_wildcard
	}
}

test_mixed_wildcard_project_items_deny_path if {
	not authorization.allow with input as {
		"user_id": "user7",
		"action": "GET",
		"resource": "/projects/a3a19444-2632-4f1c-8907-016b2260828b/other/357256d7-c071-4f7f-acc7-3cdf299034d3",
	}
		with data.permissions as permissions_user7_mixed_wildcard
	not authorization.allow with input as {
		"user_id": "user7",
		"action": "GET",
		"resource": "/projects/11111111-1111-1111-1111-111111111111/items/357256d7-c071-4f7f-acc7-3cdf299034d3",
	}
		with data.permissions as permissions_user7_mixed_wildcard
}

test_mixed_wildcard_user_settings_allow if {
	authorization.allow with input as {
		"user_id": "user7",
		"action": "PUT",
		"resource": "/users/5063fae0-e3f1-49ba-bbde-377bc99c8cad/settings/5a4c4cad-7652-450f-b5e0-97905ad1d074",
	}
		with data.permissions as permissions_user7_mixed_wildcard
}

test_mixed_wildcard_user_settings_deny_method if {
	methods := ["GET", "POST", "DELETE", "PATCH"]
	every method in methods {
		not authorization.allow with input as {
			"user_id": "user7",
			"action": method,
			"resource": "/users/5063fae0-e3f1-49ba-bbde-377bc99c8cad/settings/5a4c4cad-7652-450f-b5e0-97905ad1d074",
		}
			with data.permissions as permissions_user7_mixed_wildcard
	}
}

test_mixed_wildcard_user_settings_deny_path if {
	not authorization.allow with input as {
		"user_id": "user7",
		"action": "PUT",
		"resource": "/users/5063fae0-e3f1-49ba-bbde-377bc99c8cad/profile/5a4c4cad-7652-450f-b5e0-97905ad1d074",
	}
		with data.permissions as permissions_user7_mixed_wildcard
	not authorization.allow with input as {
		"user_id": "user7",
		"action": "PUT",
		"resource": "/users/5063fae0-e3f1-49ba-bbde-377bc99c8cad/settings/519b4cec-f42c-4281-956d-81fac04ba6c1",
	}
		with data.permissions as permissions_user7_mixed_wildcard
}
