//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// filterExample is the example a filter parameter's description gives, after
// "e.g." or "Example:", to the end of the description.
var filterExample = regexp.MustCompile(`(?:e\.g\.,?|Example:)\s*(.+)$`)

// filterWithoutExample are the lists whose filter description gives no
// example. Every other list's must give one the pattern above reads: an
// example phrased another way is not skipped, it fails here.
var filterWithoutExample = []string{"/auth/idp_types", "/rate_limits", "/resources_limits"}

// TestEveryFilterExampleIsAccepted: the contract's filter descriptions give
// an example, and a client copies it. Five were filters the API refuses:
// `id=1` on a uuid column (the three user lists) and `'name LIKE policy%'`,
// the value unquoted (the two policy lists). Every example in the contract
// is sent to every list that documents it, its path ids filled as the list
// fields sweep fills them, and must list.
//
// Not parallel: the suite shares one per-address rate-limit budget.
func TestEveryFilterExampleIsAccepted(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "api", "swagger.json"))
	require.NoError(t, err)

	var contract struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"parameters"`
		} `json:"paths"`
	}
	require.NoError(t, json.Unmarshal(raw, &contract))

	// route -> the example its filter description gives
	examples := map[string]string{}

	for route, methods := range contract.Paths {
		for _, p := range methods["get"].Parameters {
			if p.Name != "filter" {
				continue
			}

			m := filterExample.FindStringSubmatch(p.Description)
			if m == nil {
				require.Contains(t, filterWithoutExample, route, "%s's filter description gives no example the test can read: %q", route, p.Description)

				continue
			}

			require.NotContains(t, filterWithoutExample, route, "%s gives an example now: take it out of filterWithoutExample", route)

			// "(e.g., x)": the parenthesis the description opened, not the example's.
			example := strings.TrimSpace(m[1])
			if strings.Count(example, ")") > strings.Count(example, "(") {
				example = strings.TrimSuffix(example, ")")
			}

			examples[route] = example
		}
	}

	require.GreaterOrEqual(t, len(examples), 10, "the contract lost its filter examples: this would test nothing")

	admin := getAdminUserTokens(t)
	headers := map[string]string{"Authorization": "Bearer " + admin.AccessToken}

	t.Cleanup(func() { deleteUserByIDFromDB(t, admin.UserID) })

	project := createProjectInDB(t, generateRandomName(t, "filter-examples"), "a project for the filter examples")
	t.Cleanup(func() { deleteProjectByIDFromDB(t, project.ID) })

	ids := map[string]string{"{project_id}": project.ID.String()}
	for slug, list := range map[string]string{"{policy_id}": "/policies", "{role_id}": "/roles", "{user_id}": "/users"} {
		ids[slug] = firstID(t, list, headers)
	}

	routes := make([]string, 0, len(examples))
	for route := range examples {
		routes = append(routes, route)
	}

	slices.Sort(routes)

	for _, route := range routes {
		example := examples[route]

		t.Run(route+"?filter="+example, func(t *testing.T) {
			path := route
			for slug, id := range ids {
				path = strings.ReplaceAll(path, slug, id)
			}

			require.NotContains(t, path, "{", "%s has a path parameter the test does not fill: add it to ids", route)

			ep := newAPIEndpoint(http.MethodGet, path)
			ep.SetQueryParam("filter", example)

			status, body := getRetryingTooManyRequests(t, ep, headers)
			if status != http.StatusOK {
				t.Errorf("the contract's example %q is refused by %s: %d %s", example, route, status, body)
			}
		})
	}
}
