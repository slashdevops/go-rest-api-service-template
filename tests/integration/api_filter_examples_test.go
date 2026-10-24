//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// filterExample is the example a filter parameter's description gives, after
// "e.g." or "Example:".
var filterExample = regexp.MustCompile(`(?:e\.g\.,?|Example:)\s*(.+?)\)?$`)

// TestEveryFilterExampleIsAccepted: the contract's filter descriptions give
// an example, and a client copies it. Five were filters the API refuses:
// `id=1` on a uuid column (the three user lists) and `'name LIKE policy%'`,
// the value unquoted (the two policy lists). Every example in the contract
// is sent to a list that documents it, and must list.
func TestEveryFilterExampleIsAccepted(t *testing.T) {
	t.Parallel()

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

	// example -> the routes without path parameters that document it
	examples := map[string][]string{}

	for path, methods := range contract.Paths {
		for _, p := range methods["get"].Parameters {
			if p.Name != "filter" {
				continue
			}

			m := filterExample.FindStringSubmatch(p.Description)
			if m == nil {
				continue
			}

			example := strings.TrimSpace(m[1])
			if _, ok := examples[example]; !ok {
				examples[example] = nil
			}

			if !strings.Contains(path, "{") {
				examples[example] = append(examples[example], path)
			}
		}
	}

	require.NotEmpty(t, examples, "the contract gives no filter example: this would test nothing")

	admin := getAdminUserTokens(t)
	headers := map[string]string{"Authorization": "Bearer " + admin.AccessToken}

	t.Cleanup(func() { deleteUserByIDFromDB(t, admin.UserID) })

	for example, paths := range examples {
		if len(paths) == 0 {
			t.Errorf("the example %q is documented only on routes with path parameters: send it from a list that has none", example)

			continue
		}

		for _, path := range paths {
			t.Run(path+"?filter="+example, func(t *testing.T) {
				ep := newAPIEndpoint(http.MethodGet, path)
				ep.requestURL.RawQuery = url.Values{"filter": {example}}.Encode()

				status, body := getRetryingTooManyRequests(t, ep, headers)
				if status != http.StatusOK {
					t.Errorf("the contract's example %q is refused by %s: %d %s", example, path, status, body)
				}
			})
		}
	}
}
