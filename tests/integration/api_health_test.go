//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
)

var (
	healthStatusEndpoint   = newAPIEndpoint(http.MethodGet, "/health/status")
	healthDetailedEndpoint = newAPIEndpoint(http.MethodGet, "/health/detailed")
)

func TestHealthStatus(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// No authorization required for health endpoints
		response, err := sendHTTPRequest(t, ctx, healthStatusEndpoint, nil)
		require.NoError(t, err, "Error sending HTTP request")
		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200 OK. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		health, err := parserResponseBody[payload.Health](t, response)
		require.NoError(t, err, "Error parsing response body")

		assert.NotNil(t, health.Status, "Health status should not be nil")
		assert.NotEmpty(t, health.Checks, "Health checks should not be empty")
		for _, check := range health.Checks {
			assert.NotEmpty(t, check.Name, "Check name should not be empty")
			assert.NotEmpty(t, check.Kind, "Check kind should not be empty")
		}
	})
}

func TestHealthDetailed(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// /health/detailed REQUIRES authentication, unlike the probe endpoints.
		//
		// This test sent no token and expected 200. It has been asserting the
		// old, public shape ever since the endpoint moved behind the auth
		// chain: `mux.Handle("GET /health/detailed", authenticated.ThenFunc(...))`,
		// declared as `@Security AccessToken`.
		//
		// The distinction is deliberate and worth not undoing: /health/live and
		// /health/status stay public because a probe cannot hold a token, while
		// this one names every component, its configuration and the database
		// pool -- which is exactly what an anonymous caller should not get.
		adminToken := getAdminUserTokens(t)
		require.NotEmpty(t, adminToken.AccessToken, "Admin access token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		response, err := sendHTTPRequest(t, ctx, healthDetailedEndpoint, nil, accessTokenHeader)
		require.NoError(t, err, "Error sending HTTP request")
		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200 OK. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		health, err := parserResponseBody[payload.DetailedHealth](t, response)
		require.NoError(t, err, "Error parsing response body")

		assert.NotEmpty(t, health.Status, "Detailed health status should not be empty")
		assert.NotEmpty(t, health.Version, "Detailed health version should not be empty")
		assert.NotEmpty(t, health.Uptime, "Detailed health uptime should not be empty")
		assert.NotEmpty(t, health.Components, "Detailed health components should not be empty")
		for name, component := range health.Components {
			assert.NotEmpty(t, name, "Component name should not be empty")
			assert.NotEmpty(t, component.Status, "Component status should not be empty")
		}
	})
}
