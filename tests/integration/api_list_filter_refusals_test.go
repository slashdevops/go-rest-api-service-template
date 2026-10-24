//go:build integration

package integration

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

// filtered is a list endpoint with a filter in its query.
func filtered(path, filter string) *apiEndpoint {
	ep := newAPIEndpoint(http.MethodGet, path)
	ep.requestURL.RawQuery = url.Values{"filter": {filter}}.Encode()

	return ep
}

// TestAFilterValueOfTheWrongTypeIsRefused: a filter whose shape the parser
// accepts and whose value Postgres cannot take as the column's type.
//
// `GET /users?filter=id = 'nope'` was answered 500, on every list. The
// parser checks the expression and its field names; it does not know that
// `id` is a uuid, so the statement went to Postgres, which refused the cast
// (22P02), and the driver's error was answered as the service's own fault.
// It is the caller's: a 400, in the service's words.
func TestAFilterValueOfTheWrongTypeIsRefused(t *testing.T) {
	t.Parallel()

	admin := getAdminUserTokens(t)
	headers := map[string]string{"Authorization": "Bearer " + admin.AccessToken}

	t.Cleanup(func() { deleteUserByIDFromDB(t, admin.UserID) })

	for _, path := range []string{"/users", "/roles", "/policies", "/projects", "/products", "/resources", "/resources_limits", "/rate_limits", "/auth/idps", "/auth/idp_types"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			status, body := getRetryingTooManyRequests(t, filtered(path, "id = 'nope'"), headers)

			assert.Equal(t, http.StatusBadRequest, status, "GET %s?filter=id = 'nope' must be refused, not answered as a fault. Body: %s", path, body)
			assert.Contains(t, body, "a value does not fit the type of the field it is compared with")
			// Postgres's text quotes the caller's value and names the
			// statement's aliases: it is not ours to publish.
			assert.NotContains(t, body, "invalid input syntax", "Postgres's message reached the caller")
			assert.NotContains(t, body, "SQLSTATE")
		})
	}
}

// TestAFilterIsRefusedInTheServicesWords pins what a caller is told, on one
// list, for each kind of filter Postgres refuses, and that a filter Postgres
// accepts still lists.
func TestAFilterIsRefusedInTheServicesWords(t *testing.T) {
	t.Parallel()

	admin := getAdminUserTokens(t)
	headers := map[string]string{"Authorization": "Bearer " + admin.AccessToken}

	t.Cleanup(func() { deleteUserByIDFromDB(t, admin.UserID) })

	const value = "a value does not fit the type of the field it is compared with"

	for _, c := range []struct {
		name, filter, says string
		status             int
	}{
		{name: "text_for_a_uuid", filter: "id = 'nope'", status: http.StatusBadRequest, says: value},
		{name: "a_number_for_a_uuid", filter: "id = 1", status: http.StatusBadRequest, says: value},
		{name: "text_for_a_boolean", filter: "disabled = 'maybe'", status: http.StatusBadRequest, says: value},
		{name: "text_for_a_timestamp", filter: "created_at = 'nope'", status: http.StatusBadRequest, says: value},
		{name: "a_time_zone_that_does_not_exist", filter: "created_at = '2026-01-01 00:00 Foo/Bar'", status: http.StatusBadRequest, says: value},
		{name: "a_timestamp_that_is_no_date", filter: "created_at = '2026-02-30'", status: http.StatusBadRequest, says: value},
		{name: "a_text_field_compared_with_a_number", filter: "first_name = 1", status: http.StatusBadRequest, says: value},
		{name: "a_pattern_that_is_not_one", filter: "first_name ~ '('", status: http.StatusBadRequest, says: "a pattern is not a valid regular expression or LIKE pattern"},
		{name: "a_text_field_used_as_a_boolean", filter: "first_name IS TRUE", status: http.StatusBadRequest, says: "a value has the wrong type for its field"},
		{name: "a_value_of_the_right_type_lists", filter: "first_name = 'nobody-is-called-this'", status: http.StatusOK, says: `"items"`},
		// The ESCAPE clause, since qfv v1.1.0: PostgreSQL 18 runs it, and the
		// parser refused it until then. What PostgreSQL refuses of it, the
		// parser refuses too, in the service's words.
		{name: "an_escape_clause_lists", filter: "first_name LIKE 'nobody!_is!_called!_this' ESCAPE '!'", status: http.StatusOK, says: `"items"`},
		{name: "an_empty_escape_lists", filter: "first_name LIKE 'nobody%' ESCAPE ''", status: http.StatusOK, says: `"items"`},
		{name: "an_escape_of_two_characters", filter: "first_name LIKE 'x' ESCAPE '!!'", status: http.StatusBadRequest, says: "is not a valid filter expression"},
		{name: "an_escape_after_the_operator", filter: "first_name ~~ 'x' ESCAPE '!'", status: http.StatusBadRequest, says: "is not a valid filter expression"},
		{name: "a_well_formed_id_lists", filter: "id = '" + admin.UserID.String() + "'", status: http.StatusOK, says: admin.UserID.String()},
	} {
		t.Run(c.name, func(t *testing.T) {
			status, body := getRetryingTooManyRequests(t, filtered("/users", c.filter), headers)

			assert.Equal(t, c.status, status, "body: %s", body)
			assert.Contains(t, body, c.says)
			assert.NotContains(t, body, "does not exist\"", "Postgres's message reached the caller")
			assert.NotContains(t, body, "usrs.", "the statement's alias reached the caller")
		})
	}
}
