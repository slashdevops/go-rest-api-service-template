package authorization

default allow := false

allow if {
	data.permissions.users[input.user_id]["*"] == ["*"]
}

allow if {
	actions := data.permissions.users[input.user_id]["*"]
	input.action in actions
}

allow if {
	actions := data.permissions.users[input.user_id][input.resource]
	input.action in actions
}

allow if {
	resources_and_actions := data.permissions.users[input.user_id]

	some resource, actions in resources_and_actions

	# Ensure resource contains a wildcard before attempting regex conversion
	contains(resource, "*")
	resource_regex := replace_placeholders(resource)
	regex.match(resource_regex, input.resource)
	input.action in actions
}

# replace_placeholders takes a resource string and replaces the wildcard (*) with a
# regex pattern that matches any sequence of characters except a slash.
# e.g. /users/* -> ^/users/[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$
replace_placeholders(resource) := concat(
	"",
	[
		"^",
		regex.replace(resource, `\*`, `[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`),
		"$",
	],
)
