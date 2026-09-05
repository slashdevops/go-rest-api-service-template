//go:build integration

package integration

import (
	"net/http"
	"testing"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

var (
	resourcesListEndpoint    = newAPIEndpoint(http.MethodGet, "/resources")
	resourcesMatchesEndpoint = newAPIEndpoint(http.MethodGet, "/resources/matches")
	resourcesGetEndpoint     = newAPIEndpoint(http.MethodGet, "/resources/{resource_id}")
)

func TestResourcesList(t *testing.T) {
	t.Run("list_resources", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. List the resources
		listResponse, err := sendHTTPRequest(t, ctx, resourcesListEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to list resources")
		defer listResponse.Body.Close()

		// 3. Check the list response
		assert.Equal(t, http.StatusOK, listResponse.StatusCode, "Expected status code 200 for list. Got %d. Message: %s", listResponse.StatusCode, readResponseBody(t, listResponse))
		listAPIResp, err := parserResponseBody[domain.ListResourcesOutput](t, listResponse)
		assert.NoError(t, err, "Failed to parse list response body")

		// 4. Verify that resources are returned
		assert.NotEmpty(t, listAPIResp.Items, "Expected resources to be returned")

		// 5. Verify the structure of the returned resources
		for _, resource := range listAPIResp.Items {
			assert.NotEqual(t, uuid.Nil(), resource.ID, "Resource ID should not be nil")
			assert.NotEmpty(t, resource.Name, "Resource name should not be empty")
			assert.NotEmpty(t, resource.Action, "Resource action should not be empty")
			assert.NotEmpty(t, resource.Resource, "Resource resource path should not be empty")
		}

		// 6. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("require_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		resp, err := sendHTTPRequest(t, ctx, resourcesListEndpoint, nil)
		assert.NoError(t, err, "Failed to send request without authentication")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

func TestResourcesGet(t *testing.T) {
	t.Run("get_resource", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Get a list of resources first to find one to get by ID
		listResponse, err := sendHTTPRequest(t, ctx, resourcesListEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to list resources")
		defer listResponse.Body.Close()

		assert.Equal(t, http.StatusOK, listResponse.StatusCode, "Expected status code 200 for list. Got %d. Message: %s", listResponse.StatusCode, readResponseBody(t, listResponse))
		listAPIResp, err := parserResponseBody[domain.ListResourcesOutput](t, listResponse)
		assert.NoError(t, err, "Failed to parse list response body")

		// Ensure we have at least one resource to test with
		assert.NotEmpty(t, listAPIResp.Items, "Expected resources to be returned")
		resourceID := listAPIResp.Items[0].ID

		// 3. Get a specific resource by ID
		getEndpoint := resourcesGetEndpoint.RewriteSlugs(resourceID.String())
		getResponse, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to get resource")
		defer getResponse.Body.Close()

		// 4. Check the get response
		assert.Equal(t, http.StatusOK, getResponse.StatusCode, "Expected status code 200 for get. Got %d. Message: %s", getResponse.StatusCode, readResponseBody(t, getResponse))
		resourceResp, err := parserResponseBody[domain.Resource](t, getResponse)
		assert.NoError(t, err, "Failed to parse get response body")

		// 5. Verify the structure of the returned resource
		assert.Equal(t, resourceID, resourceResp.ID, "Resource ID should match")
		assert.NotEmpty(t, resourceResp.Name, "Resource name should not be empty")
		assert.NotEmpty(t, resourceResp.Action, "Resource action should not be empty")
		assert.NotEmpty(t, resourceResp.Resource, "Resource resource path should not be empty")

		// 6. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("get_resource_bad_request", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Try to get a resource with an invalid ID format (not a UUID)
		invalidResourceID := "not-a-valid-uuid"
		getEndpoint := resourcesGetEndpoint.RewriteSlugs(invalidResourceID)
		getResponse, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to get resource with invalid ID")
		defer getResponse.Body.Close()

		// 3. Check that we get a 400 Bad Request response
		assert.Equal(t, http.StatusBadRequest, getResponse.StatusCode, "Expected status code 400 for invalid resource ID format. Got %d. Message: %s", getResponse.StatusCode, readResponseBody(t, getResponse))

		// 4. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, getResponse)
		assert.NoError(t, err, "Failed to parse error response")

		// Verify error message contains information about the invalid UUID format
		assert.Contains(t, errorResp.Message, "invalid", "Error message should indicate that the resource ID format is invalid")
		assert.Equal(t, getEndpoint.method, errorResp.Method, "Expected method to be set")
		assert.Equal(t, getEndpoint.Path(), errorResp.Path, "Expected path to be set")

		// 5. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("get_resource_not_found", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Generate a random UUID that doesn't exist in the database
		nonExistentResourceID := uuid.NewV7()

		// 3. Try to get the non-existent resource
		getEndpoint := resourcesGetEndpoint.RewriteSlugs(nonExistentResourceID.String())
		getResponse, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to get non-existent resource")
		defer getResponse.Body.Close()

		// 4. Check that we get a 404 Not Found response
		assert.Equal(t, http.StatusNotFound, getResponse.StatusCode, "Expected status code 404 for non-existent resource. Got %d. Message: %s", getResponse.StatusCode, readResponseBody(t, getResponse))

		// 5. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, getResponse)
		assert.NoError(t, err, "Failed to parse error response")

		// Verify error message contains information about the resource not being found
		assert.Contains(t, errorResp.Message, "not found", "Error message should indicate that the resource was not found")
		assert.Equal(t, getEndpoint.method, errorResp.Method, "Expected method to be set")
		assert.Equal(t, getEndpoint.Path(), errorResp.Path, "Expected path to be set")

		// 6. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("require_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		getEndpoint := resourcesGetEndpoint.RewriteSlugs("00000000-0000-0000-0000-000000000000")
		resp, err := sendHTTPRequest(t, ctx, getEndpoint, nil)
		assert.NoError(t, err, "Failed to send request without authentication")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

func TestResourcesMatches(t *testing.T) {
	t.Run("list_resources_matches", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Set up query parameters for the matches endpoint
		resourcesMatchesEndpoint.SetQueryParams(map[string]string{
			"action":   "GET",
			"resource": "/resources",
		})

		// 3. Call the matches endpoint
		matchesResponse, err := sendHTTPRequest(t, ctx, resourcesMatchesEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to list resources matches")
		defer matchesResponse.Body.Close()

		// 4. Check the matches response
		assert.Equal(t, http.StatusOK, matchesResponse.StatusCode, "Expected status code 200 for matches. Got %d. Message: %s", matchesResponse.StatusCode, readResponseBody(t, matchesResponse))
		matchesAPIResp, err := parserResponseBody[domain.ListResourcesOutput](t, matchesResponse)
		assert.NoError(t, err, "Failed to parser matches response body")

		// 5. Verify that matching resources are returned
		// Note: We're not checking specific matches because we don't know exact values,
		// but at minimum the structure should be correct
		for _, resource := range matchesAPIResp.Items {
			assert.NotEqual(t, uuid.Nil(), resource.ID, "Resource ID should not be nil")
			assert.NotEmpty(t, resource.Name, "Resource name should not be empty")
		}

		// 6. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("list_resources_matches_bad_request", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Define test cases for bad request scenarios
		testCases := []struct {
			name            string
			queryParams     map[string]string
			expectedStatus  int
			expectedMessage string
		}{
			{
				name:            "Missing both action and resource",
				queryParams:     map[string]string{},
				expectedStatus:  http.StatusBadRequest,
				expectedMessage: "invalid action",
			},
			{
				name:            "Missing action",
				queryParams:     map[string]string{"resource": "/resources"},
				expectedStatus:  http.StatusBadRequest,
				expectedMessage: "invalid action",
			},
			{
				name:            "Missing resource",
				queryParams:     map[string]string{"action": "GET"},
				expectedStatus:  http.StatusBadRequest,
				expectedMessage: "invalid resource",
			},
			{
				name:            "Invalid action value",
				queryParams:     map[string]string{"action": "INVALID", "resource": "/resources"},
				expectedStatus:  http.StatusBadRequest,
				expectedMessage: "invalid action",
			},
			{
				name:            "Invalid resource value",
				queryParams:     map[string]string{"action": "GET", "resource": "INVALID"},
				expectedStatus:  http.StatusBadRequest,
				expectedMessage: "invalid resource",
			},
		}

		// 3. Run test cases for bad requests
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Create a fresh endpoint with specific query params for this test case
				matchesEndpoint := resourcesMatchesEndpoint.Clone()
				matchesEndpoint.SetQueryParams(tc.queryParams)

				// Send request with the invalid parameters
				matchesResponse, err := sendHTTPRequest(t, ctx, matchesEndpoint, nil, accessTokenHeader)
				assert.NoError(t, err, "Failed to send request for %s", tc.name)
				defer matchesResponse.Body.Close()

				// Verify we get the expected error status code
				assert.Equal(t, tc.expectedStatus, matchesResponse.StatusCode,
					"Expected status code %d for %s, got %d", tc.expectedStatus, tc.name, matchesResponse.StatusCode)

				// Parse and verify the error response
				errorResp, err := parserResponseBody[payload.HTTPMessage](t, matchesResponse)
				assert.NoError(t, err, "Failed to parse error response for %s", tc.name)

				// Verify error message contains expected content
				assert.Contains(t, errorResp.Message, tc.expectedMessage,
					"Error message should indicate %s validation failure", tc.name)
				assert.Equal(t, matchesEndpoint.method, errorResp.Method, "Expected method to be set")
				assert.Equal(t, matchesEndpoint.Path(), errorResp.Path, "Expected path to be set")
			})
		}

		// 4. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("list_resources_matches_no_results", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Set up query parameters for a unlikely match
		matchesEndpoint := resourcesMatchesEndpoint.Clone()

		matchesEndpoint.SetQueryParams(map[string]string{
			"action":   "GET",
			"resource": "/notfoundendpoint",
		})

		// 3. Call the matches endpoint
		matchesResponse, err := sendHTTPRequest(t, ctx, matchesEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to list resources matches with non-matching criteria")
		defer matchesResponse.Body.Close()

		// 4. Check the response - should return 404 Not Found
		assert.Equal(t, http.StatusNotFound, matchesResponse.StatusCode, "Expected status code 404 for no matches. Got %d. Message: %s", matchesResponse.StatusCode, readResponseBody(t, matchesResponse))
		// 5. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, matchesResponse)
		assert.NoError(t, err, "Failed to parse error response")
		// Verify error message contains information about no matches found
		assert.Contains(t, errorResp.Message, "not found", "Error message should indicate that no matches were found")
		assert.Equal(t, matchesEndpoint.method, errorResp.Method, "Expected method to be set")
		assert.Equal(t, matchesEndpoint.Path(), errorResp.Path, "Expected path to be set")

		// 6. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("require_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		matchesEndpoint := resourcesMatchesEndpoint.Clone()
		matchesEndpoint.SetQueryParams(map[string]string{
			"action":   "GET",
			"resource": "/resources",
		})

		resp, err := sendHTTPRequest(t, ctx, matchesEndpoint, nil)
		assert.NoError(t, err, "Failed to send request without authentication")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

// TestResourceJSONSerialization tests that all resource endpoints return JSON responses with snake_case field names
func TestResourceJSONSerialization(t *testing.T) {
	// Test that Resource uses snake_case in JSON serialization
	t.Run("get_resource_response_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Get a list of resources first to find one to get by ID
		listResponse, err := sendHTTPRequest(t, ctx, resourcesListEndpoint, nil, accessTokenHeader)
		require.NoError(t, err, "Failed to send request to list resources")
		defer listResponse.Body.Close()

		require.Equal(t, http.StatusOK, listResponse.StatusCode)
		listAPIResp, err := parserResponseBody[domain.ListResourcesOutput](t, listResponse)
		require.NoError(t, err, "Failed to parse list response body")

		// Ensure we have at least one resource to test with
		require.NotEmpty(t, listAPIResp.Items, "Expected resources to be returned")
		resourceID := listAPIResp.Items[0].ID

		// 3. Get a specific resource by ID
		getEndpoint := resourcesGetEndpoint.RewriteSlugs(resourceID.String())
		response, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		require.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		// 4. Verify all Resource fields are in snake_case
		expectedFields := []string{
			"id",
			"name",
			"action",
			"resource",
		}

		assertJSONFieldsAreSnakeCase(t, response, expectedFields)

		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test that ListResourcesOutput uses snake_case including nested objects
	t.Run("list_resources_response_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. List the resources
		response, err := sendHTTPRequest(t, ctx, resourcesListEndpoint, nil, accessTokenHeader)
		require.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		// 3. Verify ListResourcesOutput top-level fields are snake_case
		expectedTopLevelFields := []string{
			"items",
			"paginator",
		}

		assertJSONFieldsAreSnakeCase(t, response, expectedTopLevelFields)

		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test that resources matches endpoint uses snake_case
	t.Run("list_resources_matches_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Set up query parameters for the matches endpoint
		matchesEndpoint := resourcesMatchesEndpoint.Clone()
		matchesEndpoint.SetQueryParams(map[string]string{
			"action":   "GET",
			"resource": "/resources",
		})

		// 3. Call the matches endpoint
		response, err := sendHTTPRequest(t, ctx, matchesEndpoint, nil, accessTokenHeader)
		require.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		// 4. Verify ListResourcesOutput top-level fields are snake_case
		expectedTopLevelFields := []string{
			"items",
			"paginator",
		}

		assertJSONFieldsAreSnakeCase(t, response, expectedTopLevelFields)

		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}
