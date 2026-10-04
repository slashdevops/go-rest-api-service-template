//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// listFields is what a list route validates `?filter=` and `?sort=` against.
type listFields struct {
	filter []string
	sort   []string
}

// listFieldsByRoute is the allow-lists of every list route of the contract.
// A list that is added adds its row: the test fails on a filterable route of
// docs/api/swagger.json that is not here.
var listFieldsByRoute = map[string]listFields{
	"/auth/idp_types":                 {domain.IDPTypesFilterFields, domain.IDPTypesSortFields},
	"/auth/idps":                      {domain.IDPsFilterFields, domain.IDPsSortFields},
	"/policies":                       {domain.PoliciesFilterFields, domain.PoliciesSortFields},
	"/policies/{policy_id}/roles":     {domain.RolesFilterFields, domain.RolesSortFields},
	"/products":                       {domain.ProductsFilterFields, domain.ProductsSortFields},
	"/projects":                       {domain.ProjectFilterFields, domain.ProjectSortFields},
	"/projects/{project_id}/products": {domain.ProductsFilterFields, domain.ProductsSortFields},
	"/projects/{project_id}/users":    {domain.UsersFilterFields, domain.UsersSortFields},
	"/rate_limits":                    {domain.RateLimitsFilterFields, domain.RateLimitsSortFields},
	"/resources":                      {domain.ResourcesFilterFields, domain.ResourcesSortFields},
	"/resources_limits":               {domain.ResourcesLimitsFilterFields, domain.ResourcesLimitsSortFields},
	"/roles":                          {domain.RolesFilterFields, domain.RolesSortFields},
	"/roles/{role_id}/policies":       {domain.PoliciesFilterFields, domain.PoliciesSortFields},
	"/roles/{role_id}/users":          {domain.UsersFilterFields, domain.UsersSortFields},
	"/users":                          {domain.UsersFilterFields, domain.UsersSortFields},
	"/users/{user_id}/projects":       {domain.ProjectFilterFields, domain.ProjectSortFields},
	"/users/{user_id}/roles":          {domain.RolesFilterFields, domain.RolesSortFields},
}

// TestEveryListFieldCanBeFilteredAndSortedBy holds every list's allow-lists to
// what its statement can serve: a field a list accepts in `?filter=` or
// `?sort=` must be one Postgres can evaluate there.
//
// Two lists accepted a field and were then refused by the database for "a
// field that does not exist". `GET /resources_limits?filter=soft_limit = 1`:
// the repository prefixed the field with the listed relation's alias, and the
// column belongs to the joined one. `GET /auth/idps?filter=system = true` and
// `?sort=system`: identity providers have no such column. Nothing held an
// allow-list to the statement it is spliced into.
//
// Each route is asked for the rows where any of its filter fields is not
// null -- one request that names every field and can only be answered 200 --
// and then sorted by each of its sort fields. A row of the table above that
// names the wrong allow-list shows as a 400 too ("not a filterable field"),
// so the table checks itself.
//
// Not parallel: the suite shares one per-address rate-limit budget, and a
// sweep beside the parallel tests costs one of them a 429.
func TestEveryListFieldCanBeFilteredAndSortedBy(t *testing.T) {
	headers := map[string]string{"Authorization": "Bearer " + getAdminUserTokens(t).AccessToken}

	routes := filterableListRoutes(t)
	require.GreaterOrEqual(t, len(routes), 10, "the contract lost its filterable lists; this test would pass by testing nothing")

	// A project, so that the project-scoped lists have something to be asked
	// about on a database that was just created.
	project := createProjectInDB(t, generateRandomName(t, "list-fields"), "a project for the list fields sweep")
	t.Cleanup(func() { deleteProjectByIDFromDB(t, project.ID) })

	ids := map[string]string{"{project_id}": project.ID.String()}
	for slug, list := range map[string]string{"{policy_id}": "/policies", "{role_id}": "/roles", "{user_id}": "/users"} {
		ids[slug] = firstID(t, list, headers)
	}

	for _, route := range routes {
		fields, known := listFieldsByRoute[route]
		require.True(t, known, "%s takes a filter and has no row in listFieldsByRoute: add it with its allow-lists", route)
		require.NotEmpty(t, fields.filter, "%s has an empty filter allow-list", route)
		require.NotEmpty(t, fields.sort, "%s has an empty sort allow-list", route)

		path := route
		for slug, id := range ids {
			path = strings.ReplaceAll(path, slug, id)
		}

		name := strings.Trim(strings.NewReplacer("/", "_", "{", "", "}", "").Replace(route), "_")

		terms := make([]string, len(fields.filter))
		for i, field := range fields.filter {
			terms[i] = field + " IS NOT NULL"
		}

		expression := strings.Join(terms, " OR ")
		require.LessOrEqual(t, len(expression), domain.MaxFilterExpressionLength, "%s: the allow-list no longer fits one filter", route)

		t.Run(name+"/filter", func(t *testing.T) {
			endpoint := newAPIEndpoint(http.MethodGet, path)
			endpoint.SetQueryParam("filter", expression)

			status, body := getRetryingTooManyRequests(t, endpoint, headers)
			assert.Equal(t, http.StatusOK, status, "GET %s?filter=%q: a field this list allows cannot be filtered by. Body: %s", route, expression, body)
		})

		for _, field := range fields.sort {
			t.Run(name+"/sort_"+field, func(t *testing.T) {
				endpoint := newAPIEndpoint(http.MethodGet, path)
				endpoint.SetQueryParam("sort", field+" ASC")

				status, body := getRetryingTooManyRequests(t, endpoint, headers)
				assert.Equal(t, http.StatusOK, status, "GET %s?sort=%s ASC: a field this list allows cannot be sorted by. Body: %s", route, field, body)
			})
		}
	}

	for route := range listFieldsByRoute {
		assert.True(t, slices.Contains(routes, route), "%s is in listFieldsByRoute and is not a filterable list route of the contract", route)
	}
}

// TestAResourceLimitIsFilteredByItsLimits: the two limit fields are shown
// from the joined limits relation, -1 when a scope has no limit of its own.
// A filter on them matches what the row shows.
func TestAResourceLimitIsFilteredByItsLimits(t *testing.T) {
	headers := map[string]string{"Authorization": "Bearer " + getAdminUserTokens(t).AccessToken}

	for _, tt := range []struct {
		name   string
		filter string
		holds  func(soft, hard int64) bool
	}{
		{name: "a_soft_limit_at_or_above_zero", filter: "soft_limit >= 0", holds: func(soft, _ int64) bool { return soft >= 0 }},
		{name: "no_soft_limit", filter: "soft_limit < 0", holds: func(soft, _ int64) bool { return soft == -1 }},
		{name: "a_hard_limit_at_or_above_zero", filter: "hard_limit >= 0", holds: func(_, hard int64) bool { return hard >= 0 }},
		{name: "no_hard_limit", filter: "hard_limit < 0", holds: func(_, hard int64) bool { return hard == -1 }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			endpoint := newAPIEndpoint(http.MethodGet, "/resources_limits")
			endpoint.SetQueryParam("filter", tt.filter)
			endpoint.SetQueryParam("limit", "100")

			status, body := getRetryingTooManyRequests(t, endpoint, headers)
			require.Equal(t, http.StatusOK, status, "GET /resources_limits?filter=%q. Body: %s", tt.filter, body)

			var page struct {
				Items []struct {
					SoftLimit int64 `json:"soft_limit"`
					HardLimit int64 `json:"hard_limit"`
				} `json:"items"`
			}
			require.NoError(t, json.Unmarshal([]byte(body), &page), "body: %s", body)

			for _, item := range page.Items {
				assert.True(t, tt.holds(item.SoftLimit, item.HardLimit),
					"filter %q returned a row it does not hold for: soft_limit=%d hard_limit=%d", tt.filter, item.SoftLimit, item.HardLimit)
			}
		})
	}
}

// TestAFieldNoColumnServesIsNotFilterable: a field the list has no column
// for is refused by the validator, in its words, and not by the database.
func TestAFieldNoColumnServesIsNotFilterable(t *testing.T) {
	headers := map[string]string{"Authorization": "Bearer " + getAdminUserTokens(t).AccessToken}

	for _, param := range []struct{ key, value string }{
		{key: "filter", value: "system = true"},
		{key: "sort", value: "system ASC"},
	} {
		t.Run("idps_"+param.key, func(t *testing.T) {
			endpoint := newAPIEndpoint(http.MethodGet, "/auth/idps")
			endpoint.SetQueryParam(param.key, param.value)

			status, body := getRetryingTooManyRequests(t, endpoint, headers)
			assert.Equal(t, http.StatusBadRequest, status, "body: %s", body)
			assert.NotContains(t, body, "does not exist", "the database refused it, not the validator. Body: %s", body)
		})
	}
}

// filterableListRoutes is every GET route of the contract that takes a
// `filter` parameter, read from the generated swagger.json.
func filterableListRoutes(t *testing.T) []string {
	t.Helper()

	raw, err := os.ReadFile("../../docs/api/swagger.json")
	require.NoError(t, err)

	var contract struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name string `json:"name"`
				In   string `json:"in"`
			} `json:"parameters"`
		} `json:"paths"`
	}
	require.NoError(t, json.Unmarshal(raw, &contract))

	var routes []string

	for path, operations := range contract.Paths {
		get, ok := operations["get"]
		if !ok {
			continue
		}

		for _, parameter := range get.Parameters {
			if parameter.Name == "filter" && parameter.In == "query" {
				routes = append(routes, path)
			}
		}
	}

	slices.Sort(routes)

	return routes
}

// firstID is the id of the first row of a list.
func firstID(t *testing.T, list string, headers map[string]string) string {
	t.Helper()

	endpoint := newAPIEndpoint(http.MethodGet, list)
	endpoint.SetQueryParam("limit", "1")

	status, body := getRetryingTooManyRequests(t, endpoint, headers)
	require.Equal(t, http.StatusOK, status, "GET %s. Body: %s", list, body)

	var page struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &page), "body: %s", body)
	require.NotEmpty(t, page.Items, "%s is empty", list)

	return page.Items[0].ID
}

// getRetryingTooManyRequests sends a GET and tries again while the answer is
// a 429: the suite shares one rate-limit budget per address, and a sweep is
// not a test of it.
func getRetryingTooManyRequests(t *testing.T, endpoint *apiEndpoint, headers map[string]string) (int, string) {
	t.Helper()

	for attempt := 0; ; attempt++ {
		resp, err := sendHTTPRequest(t, t.Context(), endpoint, nil, headers)
		require.NoError(t, err)

		body := readResponseBody(t, resp)

		if resp.StatusCode != http.StatusTooManyRequests || attempt == 20 {
			return resp.StatusCode, body
		}

		time.Sleep(250 * time.Millisecond)
	}
}
