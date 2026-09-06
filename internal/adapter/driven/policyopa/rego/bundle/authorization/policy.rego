package authorization

# The API's one authorization decision: may input.user_id perform
# input.action (an HTTP method) on input.resource (a request path)?
#
# input.permissions is that user's effective grant set, materialised by the
# service from users -> roles -> policies and handed in with every decision:
#
#   {"users": {"<user id>": {"<allowed_resource>": ["<allowed_action>", ...]}}}
#
# It travels in input rather than data because it is request-scoped: OPA's
# data document is for what is true for every evaluation, and this is true
# for one caller. That is also what lets the query be compiled once.
#
# An allowed_resource is either "*", a literal path, or a path with "*" in
# place of an id. A "*" expands to a strict UUID pattern, never to "any
# segment": /users/* admits /users/<uuid> and refuses /users/me, /embeddings/*
# refuses /embeddings/bulk. An allowed_action is a method or "*", and "*"
# means every method wherever it appears -- on the global resource and on a
# concrete path alike. It used to mean that only on the global resource, so a
# policy granting "*" on /roles validated, inserted and admitted nothing.

default allow := false

# Global grants: the "*" resource.
allow if permits(grants["*"], input.action)

# A literal path.
allow if permits(grants[input.resource], input.action)

# A path with ids in it.
allow if {
	some resource, actions in grants
	contains(resource, "*")
	regex.match(replace_placeholders(resource), input.resource)
	permits(actions, input.action)
}

grants := input.permissions.users[input.user_id]

# permits: the action is listed, or the list says every action.
#
# Membership, never list equality: the administrator's ["*"] used to be
# compared with == ["*"], so attaching one more "*"-resource policy turned the
# list into ["*", "GET"] and locked every administrator out of everything
# but GET, including the call that would have undone it.
permits(actions, action) if action in actions

permits(actions, _) if "*" in actions

# replace_placeholders turns an allowed_resource into an anchored regex, with
# every "*" standing for exactly one lowercase UUID segment.
# e.g. /users/* -> ^/users/[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$
replace_placeholders(resource) := concat(
	"",
	[
		"^",
		regex.replace(resource, `\*`, `[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`),
		"$",
	],
)
