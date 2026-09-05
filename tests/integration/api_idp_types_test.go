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
	idpTypesGetEndpoint  = newAPIEndpoint(http.MethodGet, "/auth/idp_types/{idp_type_id}")
	idpTypesListEndpoint = newAPIEndpoint(http.MethodGet, "/auth/idp_types")
)

func TestIDPTypesGet(t *testing.T) {
	// Test IDP Type retrieval
	t.Run("get_idp_type_success", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Get a known IDP type from the database
		idpType := getIDPTypeFromDBByName(t, "Google")
		require.NotNil(t, idpType, "Should find Google IDP type in database")

		// 3. Get the IDP type via API
		getEndpoint := idpTypesGetEndpoint.RewriteSlugs(idpType.ID.String())
		response, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 4. Verify the response
		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		apiResp, err := parserResponseBody[domain.IDPTypes](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		// 5. Verify the returned IDP type
		assert.Equal(t, idpType.ID, apiResp.ID, "Expected IDP type ID to match")
		assert.Equal(t, idpType.Name, apiResp.Name, "Expected IDP type name to match")
		assert.Equal(t, idpType.Description, apiResp.Description, "Expected IDP type description to match")

		// 6. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test getting a non-existent IDP type
	t.Run("get_idp_type_not_found", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Use a non-existent UUID
		nonExistentID := uuid.NewV7()

		// 3. Try to get the non-existent IDP type
		getEndpoint := idpTypesGetEndpoint.RewriteSlugs(nonExistentID.String())
		response, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 4. Verify we get a 404 response
		assert.Equal(t, http.StatusNotFound, response.StatusCode, "Expected status code 404")

		apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		// 5. Verify error message
		assert.Contains(t, apiResp.Message, "not found", "Expected error message to contain 'not found'")
		assert.Equal(t, getEndpoint.method, apiResp.Method, "Expected method to be set")
		assert.Equal(t, getEndpoint.Path(), apiResp.Path, "Expected path to be set")

		// 6. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test with invalid UUID format
	t.Run("get_idp_type_invalid_uuid", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Use an invalid UUID format
		invalidUUID := "invalid-uuid-format"

		// 3. Try to get the IDP type with invalid UUID
		getEndpoint := idpTypesGetEndpoint.RewriteSlugs(invalidUUID)
		response, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 4. Verify we get a 400 response
		assert.Equal(t, http.StatusBadRequest, response.StatusCode, "Expected status code 400")

		apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		// 5. Verify error message
		assert.Contains(t, apiResp.Message, "invalid", "Expected error message to contain 'invalid'")

		// 6. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test missing authorization
	t.Run("get_idp_type_missing_authorization", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Get a known IDP type from the database
		idpType := getIDPTypeFromDBByName(t, "Google")
		require.NotNil(t, idpType, "Should find Google IDP type in database")

		// 2. Try to get the IDP type without authorization
		getEndpoint := idpTypesGetEndpoint.RewriteSlugs(idpType.ID.String())
		response, err := sendHTTPRequest(t, ctx, getEndpoint, nil, nil)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 3. Verify we get a 401 response
		assert.Equal(t, http.StatusUnauthorized, response.StatusCode, "Expected status code 401")
	})
}

func TestIDPTypesList(t *testing.T) {
	// Test IDP Types listing
	t.Run("list_idp_types", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. List the IDP types
		response, err := sendHTTPRequest(t, ctx, idpTypesListEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 3. Check the list response
		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		apiResp, err := parserResponseBody[payload.ListIDPTypesResponse](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		// 4. Verify we have some IDP types (at least Google should exist)
		assert.NotEmpty(t, apiResp.Items, "Expected to find some IDP types")

		// 5. Verify Google IDP type is in the list
		foundGoogle := false
		for _, idpType := range apiResp.Items {
			if idpType.Name == "Google" {
				foundGoogle = true
				assert.NotEmpty(t, idpType.ID, "IDP type should have an ID")
				assert.NotEmpty(t, idpType.Name, "IDP type should have a name")
				assert.NotEmpty(t, idpType.Description, "IDP type should have a description")
				break
			}
		}
		assert.True(t, foundGoogle, "Expected to find Google IDP type in the list")

		// 6. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test IDP types listing with pagination
	t.Run("list_idp_types_pagination", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Test with limit parameter
		endpointWithLimit := idpTypesListEndpoint.Clone()
		endpointWithLimit.SetQueryParam("limit", "2")
		response, err := sendHTTPRequest(t, ctx, endpointWithLimit, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 3. Check the response
		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		apiResp, err := parserResponseBody[payload.ListIDPTypesResponse](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		// 4. Verify pagination works (should return at most 2 items)
		assert.LessOrEqual(t, len(apiResp.Items), 2, "Expected at most 2 items with limit=2")

		// 5. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test IDP types listing with sort parameter
	t.Run("list_idp_types_sort", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Test sorting by name ascending
		endpointWithSort := idpTypesListEndpoint.Clone()
		endpointWithSort.SetQueryParam("sort", "name ASC")
		response, err := sendHTTPRequest(t, ctx, endpointWithSort, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 3. Check the response
		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		apiResp, err := parserResponseBody[payload.ListIDPTypesResponse](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		// 4. Verify sorting (names should be in ascending order)
		if len(apiResp.Items) > 1 {
			for i := 1; i < len(apiResp.Items); i++ {
				assert.GreaterOrEqual(t, apiResp.Items[i].Name, apiResp.Items[i-1].Name,
					"Expected names to be in ascending order")
			}
		}

		// 5. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test IDP types listing with fields filter
	t.Run("list_idp_types_fields", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Test with fields parameter (only return id and name)
		endpointWithFields := idpTypesListEndpoint.Clone()
		endpointWithFields.SetQueryParam("fields", "id,name")
		response, err := sendHTTPRequest(t, ctx, endpointWithFields, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 3. Check the response
		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		apiResp, err := parserResponseBody[payload.ListIDPTypesResponse](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		// 4. Verify field filtering works
		if len(apiResp.Items) > 0 {
			firstItem := apiResp.Items[0]
			assert.NotEmpty(t, firstItem.ID, "ID should be present")
			assert.NotEmpty(t, firstItem.Name, "Name should be present")
			// Note: Description field selection testing would require checking the raw JSON
			// to verify certain fields are excluded, which is complex in this test setup
		}

		// 5. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test missing authorization
	t.Run("list_idp_types_missing_authorization", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Try to list IDP types without authorization
		response, err := sendHTTPRequest(t, ctx, idpTypesListEndpoint, nil, nil)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 2. Verify we get a 401 response
		assert.Equal(t, http.StatusUnauthorized, response.StatusCode, "Expected status code 401")
	})

	// Test invalid query parameters
	t.Run("list_idp_types_invalid_params", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// Define test cases for invalid query parameters
		testCases := []struct {
			name           string
			queryParam     string
			queryValue     string
			expectedStatus int
		}{
			{
				name:           "Invalid limit parameter",
				queryParam:     "limit",
				queryValue:     "not-a-number",
				expectedStatus: http.StatusBadRequest,
			},
			{
				name:           "Invalid sort parameter",
				queryParam:     "sort",
				queryValue:     "invalid_field ASC",
				expectedStatus: http.StatusBadRequest,
			},
			{
				name:           "Limit too large",
				queryParam:     "limit",
				queryValue:     "10000",
				expectedStatus: http.StatusBadRequest,
			},
		}

		// 2. Run each test case
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				endpointWithParam := idpTypesListEndpoint.Clone()
				endpointWithParam.SetQueryParam(tc.queryParam, tc.queryValue)
				response, err := sendHTTPRequest(t, ctx, endpointWithParam, nil, accessTokenHeader)
				assert.NoError(t, err, "Failed to send request for %s", tc.name)
				defer response.Body.Close()

				assert.Equal(t, tc.expectedStatus, response.StatusCode,
					"Expected status code %d for %s", tc.expectedStatus, tc.name)

				if tc.expectedStatus == http.StatusBadRequest {
					apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
					assert.NoError(t, err, "Failed to parse error response for %s", tc.name)
					assert.NotEmpty(t, apiResp.Message, "Expected error message for %s", tc.name)
				}
			})
		}

		// 3. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}

// Test IDP types filtering functionality
func TestIDPTypesList_FilterByName(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// 1. Create an administrator user and get the token
	adminToken := getAdminUserTokens(t)
	assert.NotEmpty(t, adminToken, "Admin token should not be empty")

	accessTokenHeader := map[string]string{
		"Authorization": "Bearer " + adminToken.AccessToken,
	}

	// 2. Test filtering by name (case-insensitive partial match)
	endpointWithFilter := idpTypesListEndpoint.Clone()
	endpointWithFilter.SetQueryParam("filter", "name LIKE '%Google%'")
	response, err := sendHTTPRequest(t, ctx, endpointWithFilter, nil, accessTokenHeader)
	assert.NoError(t, err, "Failed to send request")
	defer response.Body.Close()

	// 3. Check the response
	assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

	apiResp, err := parserResponseBody[payload.ListIDPTypesResponse](t, response)
	assert.NoError(t, err, "Failed to parse response body")

	// 4. Verify filtering works - all returned items should contain "Google"
	for _, idpType := range apiResp.Items {
		assert.Contains(t, idpType.Name, "Google", "All returned IDP types should contain 'Google' in the name")
	} // 5. Should find at least the Google IDP type
	assert.NotEmpty(t, apiResp.Items, "Expected to find at least one IDP type matching the filter")

	// 6. Cleanup
	t.Cleanup(func() {
		deleteUserByIDFromDB(t, adminToken.UserID)
	})
}
