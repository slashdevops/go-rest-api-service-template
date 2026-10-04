//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// usageOf is the stored counter of one resource type in the system scope,
// summed over its rows; 0 when there is none.
func usageOf(t *testing.T, resource domain.ResourcesLimitsResourceType) int {
	t.Helper()

	var usage int

	err := testDBPool.QueryRow(t.Context(),
		`SELECT COALESCE(SUM(usage), 0) FROM resources_usage WHERE scope_type = $1 AND resource_type = $2`,
		domain.ResourcesLimitsScopeTypeSystem.String(), resource.String()).Scan(&usage)
	require.NoError(t, err)

	return usage
}

// TestADeleteOfAMissingRowRefundsNothing: deleting a user or an identity
// provider that does not exist is a 404 and changes no counter.
//
// The repository returned nil when no row was deleted, and the use-case went
// on as after a real delete: it decremented the system's usage counter. Three
// deletes of ids that never existed gave three slots back, so the limit the
// counter is checked against could be walked down by anyone allowed to
// delete.
//
// Not parallel, and with no parallel subtest: the counter is the whole
// system's, and every test that creates a user moves it.
func TestADeleteOfAMissingRowRefundsNothing(t *testing.T) {
	headers := map[string]string{"Authorization": "Bearer " + getAdminUserTokens(t).AccessToken}

	for _, tt := range []struct {
		name     string
		endpoint *apiEndpoint
		resource domain.ResourcesLimitsResourceType
	}{
		{name: "users", endpoint: usersDeleteEndpoint, resource: domain.ResourcesLimitsResourceTypeUsers},
		{name: "idps", endpoint: idpsDeleteEndpoint, resource: domain.ResourcesLimitsResourceTypeIDPs},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before := usageOf(t, tt.resource)

			for range 3 {
				response, err := sendHTTPRequest(t, t.Context(), tt.endpoint.RewriteSlugs(mustUUIDString(t)), nil, headers)
				require.NoError(t, err)

				body := readResponseBody(t, response)
				response.Body.Close()

				assert.Equal(t, http.StatusNotFound, response.StatusCode, "body: %s", body)
			}

			assert.Equal(t, before, usageOf(t, tt.resource), "deleting ids that do not exist moved the %s usage counter", tt.name)
		})
	}
}
