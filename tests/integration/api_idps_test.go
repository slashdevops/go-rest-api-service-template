//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
)

var (
	idpsCreateEndpoint    = newAPIEndpoint(http.MethodPost, "/auth/idps")
	idpsGetEndpoint       = newAPIEndpoint(http.MethodGet, "/auth/idps/{idp_id}")
	idpsDeleteEndpoint    = newAPIEndpoint(http.MethodDelete, "/auth/idps/{idp_id}")
	idpsUpdateEndpoint    = newAPIEndpoint(http.MethodPut, "/auth/idps/{idp_id}")
	idpsListEndpoint      = newAPIEndpoint(http.MethodGet, "/auth/idps")
	idpsAvailableEndpoint = newAPIEndpoint(http.MethodGet, "/auth/idp/available")
)

func TestIDPCreate(t *testing.T) {
	// Test IDP creation
	t.Run("create_idp", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Get a valid IDP type ID (Google)
		idpTypeID := getIDPTypeFromDBByName(t, "Google").ID

		// 3. Create a new IDP
		idpName := generateRandomName(t, "TestIDP")
		idp := map[string]any{
			"idp_type_id":           idpTypeID,
			"name":                  idpName,
			"description":           "Test Google Identity Provider",
			"callback_url":          "http://localhost:8080/api/v1/auth/idp/callback",
			"login_redirect_url":    "http://localhost:8080/login",
			"register_redirect_url": "http://localhost:8080/register",
			"logo":                  "https://example.com/google-logo.png",
			"client_id":             "test-google-client-id",
			"client_secret":         "test-google-client-secret",
		}

		response, err := sendHTTPRequest(t, ctx, idpsCreateEndpoint, idp, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusCreated, response.StatusCode, "Expected status code 201")

		apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		t.Cleanup(func() {
			deleteIDPByNameFromDB(t, idpName)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})

		assert.Equal(t, "IDP created successfully", apiResp.Message, "Expected success message")
		assert.Equal(t, idpsCreateEndpoint.method, apiResp.Method, "Expected method to be set")
		assert.Equal(t, idpsCreateEndpoint.Path(), apiResp.Path, "Expected path to be set")
	})

	// Test creating an IDP with invalid data format
	t.Run("create_idp_bad_request", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Get a valid IDP type ID for valid test cases
		idpTypeID := getIDPTypeFromDBByName(t, "Google").ID

		// 3. Define test cases for various invalid inputs
		testCases := []struct {
			name          string
			invalidIDP    map[string]any
			expectedError string
		}{
			{
				name: "Empty IDP type ID",
				invalidIDP: map[string]any{
					"idp_type_id":           uuid.Nil(),
					"name":                  "TestIDP",
					"description":           "Test Description",
					"callback_url":          "http://localhost:8080/callback",
					"login_redirect_url":    "http://localhost:8080/login",
					"register_redirect_url": "http://localhost:8080/register",
					"client_id":             "test-client",
					"client_secret":         "test-secret",
				},
				expectedError: "UUID cannot be nil or empty",
			},
			{
				name: "Empty name",
				invalidIDP: map[string]any{
					"idp_type_id":           idpTypeID,
					"name":                  "",
					"description":           "Test Description",
					"callback_url":          "http://localhost:8080/callback",
					"login_redirect_url":    "http://localhost:8080/login",
					"register_redirect_url": "http://localhost:8080/register",
					"client_id":             "test-client",
					"client_secret":         "test-secret",
				},
				expectedError: "name is required",
			},
			{
				name: "Empty description",
				invalidIDP: map[string]any{
					"idp_type_id":           idpTypeID,
					"name":                  "TestIDP",
					"description":           "",
					"callback_url":          "http://localhost:8080/callback",
					"login_redirect_url":    "http://localhost:8080/login",
					"register_redirect_url": "http://localhost:8080/register",
					"client_id":             "test-client",
					"client_secret":         "test-secret",
				},
				expectedError: "description is required",
			},
			{
				name: "Empty callback URL",
				invalidIDP: map[string]any{
					"idp_type_id":           idpTypeID,
					"name":                  "TestIDP",
					"description":           "Test Description",
					"callback_url":          "",
					"login_redirect_url":    "http://localhost:8080/login",
					"register_redirect_url": "http://localhost:8080/register",
					"client_id":             "test-client",
					"client_secret":         "test-secret",
				},
				expectedError: "callback_url is required",
			},
			{
				name: "Empty login redirect URL",
				invalidIDP: map[string]any{
					"idp_type_id":           idpTypeID,
					"name":                  "TestIDP",
					"description":           "Test Description",
					"callback_url":          "http://localhost:8080/callback",
					"login_redirect_url":    "",
					"register_redirect_url": "http://localhost:8080/register",
					"client_id":             "test-client",
					"client_secret":         "test-secret",
				},
				expectedError: "login_redirect_url is required",
			},
			{
				name: "Empty register redirect URL",
				invalidIDP: map[string]any{
					"idp_type_id":           idpTypeID,
					"name":                  "TestIDP",
					"description":           "Test Description",
					"callback_url":          "http://localhost:8080/callback",
					"login_redirect_url":    "http://localhost:8080/login",
					"register_redirect_url": "",
					"client_id":             "test-client",
					"client_secret":         "test-secret",
				},
				expectedError: "register_redirect_url is required",
			},
			{
				name: "Empty client ID",
				invalidIDP: map[string]any{
					"idp_type_id":           idpTypeID,
					"name":                  "TestIDP",
					"description":           "Test Description",
					"callback_url":          "http://localhost:8080/callback",
					"login_redirect_url":    "http://localhost:8080/login",
					"register_redirect_url": "http://localhost:8080/register",
					"client_id":             "",
					"client_secret":         "test-secret",
				},
				expectedError: "client_id is required",
			},
			{
				name: "Empty client secret",
				invalidIDP: map[string]any{
					"idp_type_id":           idpTypeID,
					"name":                  "TestIDP",
					"description":           "Test Description",
					"callback_url":          "http://localhost:8080/callback",
					"login_redirect_url":    "http://localhost:8080/login",
					"register_redirect_url": "http://localhost:8080/register",
					"client_id":             "test-client",
					"client_secret":         "",
				},
				expectedError: "client_secret is required",
			},
		}

		// 4. Run each test case
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				response, err := sendHTTPRequest(t, ctx, idpsCreateEndpoint, tc.invalidIDP, accessTokenHeader)
				assert.NoError(t, err, "Failed to send request")
				defer response.Body.Close()

				assert.Equal(t, http.StatusBadRequest, response.StatusCode, "Expected status code 400")

				apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
				assert.NoError(t, err, "Failed to parse response body")

				assert.Contains(t, apiResp.Message, tc.expectedError, "Expected error message to contain: %s", tc.expectedError)
			})
		}

		// 5. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test creating IDPs with existing name
	t.Run("create_idp_already_exists", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Get a valid IDP type ID
		idpTypeID := getIDPTypeFromDBByName(t, "Google").ID

		// 3. First create a valid IDP that will be our reference IDP
		firstIDPName := generateRandomName(t, "FirstTestIDP")
		firstIDP := map[string]any{
			"idp_type_id":           idpTypeID,
			"name":                  firstIDPName,
			"description":           "First Test IDP",
			"callback_url":          "http://localhost:8080/api/v1/auth/idp/callback",
			"login_redirect_url":    "http://localhost:8080/login",
			"register_redirect_url": "http://localhost:8080/register",
			"client_id":             "test-client-1",
			"client_secret":         "test-secret-1",
		}

		// Create the first IDP
		response, err := sendHTTPRequest(t, ctx, idpsCreateEndpoint, firstIDP, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusCreated, response.StatusCode, "Expected status code 201")

		// 4. Try to create another IDP with the same name
		duplicateIDP := map[string]any{
			"idp_type_id":           idpTypeID,
			"name":                  firstIDPName, // Same name as first IDP
			"description":           "Second Test IDP",
			"callback_url":          "http://localhost:8080/api/v1/auth/idp/callback2",
			"login_redirect_url":    "http://localhost:8080/login2",
			"register_redirect_url": "http://localhost:8080/register2",
			"client_id":             "test-client-2",
			"client_secret":         "test-secret-2",
		}

		// 5. Send request to create duplicate IDP
		response, err = sendHTTPRequest(t, ctx, idpsCreateEndpoint, duplicateIDP, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 6. Check that we get a 409 Conflict response (duplicate name)
		assert.Equal(t, http.StatusConflict, response.StatusCode, "Expected status code 409")

		// 7. Parse and verify the error response
		apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		// 8. Verify error message contains information about the conflict
		assert.Contains(t, apiResp.Message, "already exists", "Expected error message to indicate identity provider already exists")

		// 9. Cleanup
		t.Cleanup(func() {
			deleteIDPByNameFromDB(t, firstIDPName)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}

func TestIDPGet(t *testing.T) {
	// Test IDP retrieval
	t.Run("get_idp", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a new IDP via API (so it gets properly encrypted)
		idpTypeID := getIDPTypeFromDBByName(t, "Google").ID
		idpName := generateRandomName(t, "GetTestIDP")
		idp := map[string]any{
			"idp_type_id":           idpTypeID,
			"name":                  idpName,
			"description":           "Test IDP for get operation",
			"callback_url":          "http://localhost:8080/api/v1/auth/idp/callback",
			"login_redirect_url":    "http://localhost:8080/login",
			"register_redirect_url": "http://localhost:8080/register",
			"logo":                  "https://example.com/logo.png",
			"client_id":             "test-client-id",
			"client_secret":         "test-client-secret",
		}

		createResponse, err := sendHTTPRequest(t, ctx, idpsCreateEndpoint, idp, accessTokenHeader)
		assert.NoError(t, err, "Failed to create IDP")
		defer createResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, createResponse.StatusCode, "Expected IDP creation to succeed")

		// Extract the IDP ID from the Location header
		location := createResponse.Header.Get("Location")
		assert.NotEmpty(t, location, "Expected Location header")

		// Get the IDP ID from the location header (last part of the URL)
		parts := strings.Split(location, "/")
		idpIDStr := parts[len(parts)-1]
		createdIDPID, err := uuid.Parse(idpIDStr)
		assert.NoError(t, err, "Failed to parse IDP ID from Location header")

		// 3. Get the IDP
		getEndpoint := idpsGetEndpoint.RewriteSlugs(createdIDPID.String())
		t.Logf("IDP Get Endpoint: %s", getEndpoint)

		response, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// Debug: log the response status and body if it's not 200
		if response.StatusCode != http.StatusOK {
			responseBody := readResponseBody(t, response)
			t.Logf("Response status: %d, body: %s", response.StatusCode, responseBody)
		}

		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		// Read the response body
		apiResp, err := parserResponseBody[payload.IDPResponse](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		// 4. Check the response
		assert.Equal(t, createdIDPID, apiResp.ID, "Expected IDP ID to match")
		assert.Equal(t, idpName, apiResp.Name, "Expected IDP name to match")
		assert.Equal(t, "Test IDP for get operation", apiResp.Description, "Expected IDP description to match")

		// 5. Cleanup
		t.Cleanup(func() {
			deleteIDPByIDFromDB(t, createdIDPID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test retrieving a non-existent IDP
	t.Run("get_idp_not_found", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Generate a random UUID that doesn't exist in the database
		nonExistentID := mustUUIDString(t)

		// 3. Try to get the non-existent IDP
		getEndpoint := idpsGetEndpoint.RewriteSlugs(nonExistentID)
		response, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 4. Check that we get a 404 Not Found response
		assert.Equal(t, http.StatusNotFound, response.StatusCode, "Expected status code 404")

		// 5. Parse and verify the error response
		apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		// Verify error message contains information about the IDP not being found
		assert.Contains(t, apiResp.Message, "identity provider", "Expected error message to contain 'identity provider'")
		assert.Contains(t, apiResp.Message, "not found", "Expected error message to contain 'not found'")

		// 6. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test retrieving an IDP with an invalid ID format
	t.Run("get_idp_bad_request", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Try to get an IDP with an invalid ID format (not a UUID)
		getEndpoint := idpsGetEndpoint.RewriteSlugs("invalid-uuid-format")
		response, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 3. Check that we get a 400 Bad Request response
		assert.Equal(t, http.StatusBadRequest, response.StatusCode, "Expected status code 400")

		// 4. Parse and verify the error response
		apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		// Verify error message contains information about the invalid UUID format
		assert.Contains(t, apiResp.Message, "invalid UUID", "Expected error message to contain information about invalid UUID")

		// 5. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}

func TestIDPDelete(t *testing.T) {
	// Test IDP deletion
	t.Run("delete_idp", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a new IDP in the database
		idpTypeID := getIDPTypeFromDBByName(t, "Google").ID
		idpName := generateRandomName(t, "DeleteTestIDP")
		createdIDP := createIDPInDB(t, idpTypeID, idpName, "Test IDP for delete operation")

		// 3. Delete the IDP
		deleteEndpoint := idpsDeleteEndpoint.RewriteSlugs(createdIDP.ID.String())
		t.Logf("IDP Delete Endpoint: %s", deleteEndpoint)

		response, err := sendHTTPRequest(t, ctx, deleteEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 4. Check the response
		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		assert.Equal(t, "IDP deleted successfully", apiResp.Message, "Expected success message")

		// 5. Verify IDP is actually deleted (optional, try to get the IDP)
		getEndpoint := idpsGetEndpoint.RewriteSlugs(createdIDP.ID.String())
		verifyResponse, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer verifyResponse.Body.Close()

		assert.Equal(t, http.StatusNotFound, verifyResponse.StatusCode, "Expected IDP to be deleted")

		// 6. Cleanup admin user
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test deleting an IDP with an invalid ID format
	t.Run("delete_idp_bad_request", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Try to delete an IDP with an invalid ID format (not a UUID)
		deleteEndpoint := idpsDeleteEndpoint.RewriteSlugs("invalid-uuid-format")
		response, err := sendHTTPRequest(t, ctx, deleteEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 3. Check that we get a 400 Bad Request response
		assert.Equal(t, http.StatusBadRequest, response.StatusCode, "Expected status code 400")

		// 4. Parse and verify the error response
		apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		// 5. Verify error message contains information about the invalid UUID format
		assert.Contains(t, apiResp.Message, "invalid UUID", "Expected error message to contain information about invalid UUID")

		// 6. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test deleting a non-existent IDP
	t.Run("delete_idp_not_found", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Generate a UUID that doesn't exist in the database
		nonExistentID := mustUUIDString(t)

		// 3. Try to delete the non-existent IDP
		deleteEndpoint := idpsDeleteEndpoint.RewriteSlugs(nonExistentID)
		response, err := sendHTTPRequest(t, ctx, deleteEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 4. Check the response - this should still return StatusOK even though the IDP doesn't exist
		// This is because deleting a non-existent resource is considered idempotent in RESTful APIs
		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		// 5. Parse and verify the success response
		apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		// 6. Verify success message for deletion
		assert.Equal(t, "IDP deleted successfully", apiResp.Message, "Expected success message for idempotent deletion")

		// 7. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}

func TestIDPUpdate(t *testing.T) {
	// Test IDP update
	t.Run("update_idp", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a new IDP via API (so it gets properly encrypted)
		idpTypeID := getIDPTypeFromDBByName(t, "Google").ID
		idpName := generateRandomName(t, "UpdateTestIDP")
		idp := map[string]any{
			"idp_type_id":           idpTypeID,
			"name":                  idpName,
			"description":           "Test IDP for update operation",
			"callback_url":          "http://localhost:8080/api/v1/auth/idp/callback",
			"login_redirect_url":    "http://localhost:8080/login",
			"register_redirect_url": "http://localhost:8080/register",
			"logo":                  "https://example.com/logo.png",
			"client_id":             "test-client-id",
			"client_secret":         "test-client-secret",
		}

		createResponse, err := sendHTTPRequest(t, ctx, idpsCreateEndpoint, idp, accessTokenHeader)
		assert.NoError(t, err, "Failed to create IDP")
		defer createResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, createResponse.StatusCode, "Expected IDP creation to succeed")

		// Extract the IDP ID from the Location header
		location := createResponse.Header.Get("Location")
		assert.NotEmpty(t, location, "Expected Location header")

		// Get the IDP ID from the location header (last part of the URL)
		parts := strings.Split(location, "/")
		idpIDStr := parts[len(parts)-1]
		createdIDPID, err := uuid.Parse(idpIDStr)
		assert.NoError(t, err, "Failed to parse IDP ID from Location header")

		// 3. Update the IDP
		updatedName := generateRandomName(t, "UpdatedTestIDP")
		updateData := map[string]any{
			"name":        updatedName,
			"description": "Updated Test IDP Description",
			"logo":        "https://example.com/updated-logo.png",
		}

		updateEndpoint := idpsUpdateEndpoint.RewriteSlugs(createdIDPID.String())
		t.Logf("IDP Update Endpoint: %s", updateEndpoint)

		response, err := sendHTTPRequest(t, ctx, updateEndpoint, updateData, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 4. Check the update response
		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		assert.Equal(t, "IDP updated successfully", apiResp.Message, "Expected success message")

		// 5. Verify IDP is actually updated (get the IDP again)
		getEndpoint := idpsGetEndpoint.RewriteSlugs(createdIDPID.String())
		t.Logf("IDP Get Endpoint after update: %s", getEndpoint)

		verifyResponse, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer verifyResponse.Body.Close()

		assert.Equal(t, http.StatusOK, verifyResponse.StatusCode, "Expected status code 200")

		updatedIDP, err := parserResponseBody[payload.IDPResponse](t, verifyResponse)
		assert.NoError(t, err, "Failed to parse response body")

		assert.Equal(t, updatedName, updatedIDP.Name, "Expected IDP name to be updated")
		assert.Equal(t, "Updated Test IDP Description", updatedIDP.Description, "Expected IDP description to be updated")
		assert.Equal(t, "https://example.com/updated-logo.png", updatedIDP.Logo, "Expected IDP logo to be updated")

		// 6. Cleanup
		t.Cleanup(func() {
			deleteIDPByIDFromDB(t, createdIDPID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test updating an IDP with an invalid ID format
	t.Run("update_idp_bad_request", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// Set up test cases for different bad request scenarios
		testCases := []struct {
			name          string
			endpoint      string
			updateData    map[string]any
			expectedError string
		}{
			{
				name:     "Invalid UUID format",
				endpoint: "invalid-uuid-format",
				updateData: map[string]any{
					"name": "Valid Name",
				},
				expectedError: "invalid UUID",
			},
		}

		// Run each test case
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				updateEndpoint := idpsUpdateEndpoint.RewriteSlugs(tc.endpoint)
				response, err := sendHTTPRequest(t, ctx, updateEndpoint, tc.updateData, accessTokenHeader)
				assert.NoError(t, err, "Failed to send request")
				defer response.Body.Close()

				assert.Equal(t, http.StatusBadRequest, response.StatusCode, "Expected status code 400")

				apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
				assert.NoError(t, err, "Failed to parse response body")

				assert.Contains(t, apiResp.Message, tc.expectedError, "Expected error message to contain: %s", tc.expectedError)
			})
		}

		// Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test updating a non-existent IDP
	t.Run("update_idp_not_found", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Generate a UUID that doesn't exist in the database
		nonExistentID := mustUUIDString(t)

		// 3. Try to update the non-existent IDP
		updateData := map[string]any{
			"name": "Updated Name",
		}

		updateEndpoint := idpsUpdateEndpoint.RewriteSlugs(nonExistentID)
		response, err := sendHTTPRequest(t, ctx, updateEndpoint, updateData, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 4. Check that we get a 404 Not Found response for not found IDP
		assert.Equal(t, http.StatusNotFound, response.StatusCode, "Expected status code 404")

		// 5. Parse and verify the error response
		apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		// 6. Verify error message contains information about the IDP not being found
		assert.Contains(t, apiResp.Message, "not found", "Expected error message to contain information about IDP not found")

		// 7. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test updating an IDP with a name that already exists (conflict case)
	t.Run("update_idp_conflict_name", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create two IDPs with different names
		idpTypeID := getIDPTypeFromDBByName(t, "Google").ID

		// First IDP - this is the one we'll try to update
		firstIDPName := generateRandomName(t, "FirstConflictIDP")
		firstIDP := createIDPInDB(t, idpTypeID, firstIDPName, "First Test IDP")

		// Second IDP - we'll try to use this IDP's name when updating the first IDP
		secondIDPName := generateRandomName(t, "SecondConflictIDP")
		secondIDP := createIDPInDB(t, idpTypeID, secondIDPName, "Second Test IDP")

		// 3. Try to update the first IDP with the second IDP's name
		updateData := map[string]any{
			"name": secondIDPName, // Using the second IDP's name
		}

		updateEndpoint := idpsUpdateEndpoint.RewriteSlugs(firstIDP.ID.String())
		response, err := sendHTTPRequest(t, ctx, updateEndpoint, updateData, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 4. Check that we get a 409 Conflict response (duplicate name)
		assert.Equal(t, http.StatusConflict, response.StatusCode, "Expected status code 409")

		// 5. Parse and verify the error response
		apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		// 6. Verify error message contains information about the conflict
		assert.Contains(t, apiResp.Message, "already exists", "Expected error message to indicate identity provider already exists")

		// 7. Cleanup
		t.Cleanup(func() {
			deleteIDPByIDFromDB(t, firstIDP.ID)
			deleteIDPByIDFromDB(t, secondIDP.ID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}

func TestIDPList(t *testing.T) {
	// Test IDP listing
	t.Run("list_idps", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a couple of new IDPs via API
		idpTypeID := getIDPTypeFromDBByName(t, "Google").ID
		idpNames := []string{}

		for i := 0; i < 3; i++ {
			idpName := generateRandomName(t, fmt.Sprintf("ListTestIDP%d", i))
			idpNames = append(idpNames, idpName)

			createData := map[string]any{
				"idp_type_id":           idpTypeID.String(),
				"name":                  idpName,
				"description":           fmt.Sprintf("Test IDP %d for list operation", i),
				"callback_url":          fmt.Sprintf("https://example.com/callback%d", i),
				"login_redirect_url":    fmt.Sprintf("https://example.com/login%d", i),
				"register_redirect_url": fmt.Sprintf("https://example.com/register%d", i),
				"logo":                  fmt.Sprintf("https://example.com/logo%d.png", i),
				"client_id":             fmt.Sprintf("test_client_id_%d", i),
				"client_secret":         fmt.Sprintf("test_client_secret_%d", i),
			}

			response, err := sendHTTPRequest(t, ctx, idpsCreateEndpoint, createData, accessTokenHeader)
			assert.NoError(t, err, "Failed to send request")
			defer response.Body.Close()

			assert.Equal(t, http.StatusCreated, response.StatusCode, "Expected status code 201")
		}

		// 3. List the IDPs
		response, err := sendHTTPRequest(t, ctx, idpsListEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 4. Check the list response
		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200.  Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		apiResp, err := parserResponseBody[payload.ListIDPsResponse](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		// 5. Verify the created IDPs are in the list
		// Note: The list might contain other IDPs, so we check if our created IDPs are present.
		foundCount := 0
		for _, createdName := range idpNames {
			for _, listIDP := range apiResp.Items {
				if listIDP.Name == createdName {
					foundCount++
					break
				}
			}
		}

		assert.GreaterOrEqual(t, foundCount, len(idpNames), "Expected to find at least the created IDPs in the list")

		// 6. Cleanup
		t.Cleanup(func() {
			for _, idpName := range idpNames {
				deleteIDPByNameFromDB(t, idpName)
			}
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("list_idps_pagination", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// Generate a unique test identifier to prevent conflicts with other parallel tests
		testID := generateRandomName(t, "PagTest")

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create at least 20 IDPs via API to ensure we have enough for pagination
		idpTypeID := getIDPTypeFromDBByName(t, "Google").ID
		numIDPs := 25 // Create extra IDPs to ensure we have at least 20 (in case of any failures)
		idpNames := make([]string, 0, numIDPs)

		// Create IDPs with sequential names for easier verification
		for i := range numIDPs {
			idpName := fmt.Sprintf("%s-IDP-%03d", testID, i)
			idpNames = append(idpNames, idpName)

			createData := map[string]any{
				"idp_type_id":           idpTypeID.String(),
				"name":                  idpName,
				"description":           fmt.Sprintf("Test IDP %d for pagination", i),
				"callback_url":          fmt.Sprintf("https://example.com/callback%d", i),
				"login_redirect_url":    fmt.Sprintf("https://example.com/login%d", i),
				"register_redirect_url": fmt.Sprintf("https://example.com/register%d", i),
				"logo":                  fmt.Sprintf("https://example.com/logo%d.png", i),
				"client_id":             fmt.Sprintf("test_client_id_%d", i),
				"client_secret":         fmt.Sprintf("test_client_secret_%d", i),
			}

			response, err := sendHTTPRequest(t, ctx, idpsCreateEndpoint, createData, accessTokenHeader)
			assert.NoError(t, err, "Failed to send request")
			defer response.Body.Close()

			assert.Equal(t, http.StatusCreated, response.StatusCode, "Expected status code 201")
		}

		// 3. Test pagination with limit=4
		paginatedEndpoint := idpsListEndpoint.Clone()
		paginatedEndpoint.SetQueryParam("limit", "4")

		// Use default sort order for pagination (serial_id DESC, id DESC)
		// Custom sorting with pagination requires more complex cursor logic
		// which will be addressed in a future iteration

		// Add a filter to only return IDPs created by this test instance
		// This ensures we don't get IDPs from other test runs
		paginatedEndpoint.SetQueryParam("filter", fmt.Sprintf("name LIKE '%s%%'", testID))

		// Track pages we've fetched
		var allPages []payload.ListIDPsResponse
		var allIDPs []payload.IDPResponse

		// First page
		response, err := sendHTTPRequest(t, ctx, paginatedEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		firstPage, err := parserResponseBody[payload.ListIDPsResponse](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		allPages = append(allPages, firstPage)
		allIDPs = append(allIDPs, firstPage.Items...)

		// Verify first page
		assert.LessOrEqual(t, len(firstPage.Items), 4, "Expected at most 4 items in first page")

		// Validate pagination structure
		assert.NotNil(t, firstPage.Paginator, "Expected paginator to be present")

		// Validate next token exists (since we have more than 4 IDPs)
		assert.NotEmpty(t, firstPage.Paginator.NextToken, "Expected next token to be present")

		// Track IDPs we've seen
		seenIDPs := make(map[uuid.UUID]bool)

		// Verify that all items contain our test ID (proper filtering)
		for _, idp := range firstPage.Items {
			assert.True(t, strings.HasPrefix(idp.Name, testID), "Expected IDP name to start with test ID")
			seenIDPs[idp.ID] = true
		}

		// Navigate through all pages using next tokens
		currentToken := firstPage.Paginator.NextToken
		pageCount := 1

		for currentToken != "" && pageCount < 10 { // Safety limit to prevent infinite loops
			nextPageEndpoint := idpsListEndpoint.Clone()
			nextPageEndpoint.SetQueryParam("limit", "4")
			nextPageEndpoint.SetQueryParam("filter", fmt.Sprintf("name LIKE '%s%%'", testID))
			nextPageEndpoint.SetQueryParam("next_token", currentToken)

			response, err := sendHTTPRequest(t, ctx, nextPageEndpoint, nil, accessTokenHeader)
			assert.NoError(t, err, "Failed to send request for page %d", pageCount+1)
			defer response.Body.Close()

			assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200 for page %d", pageCount+1)

			nextPage, err := parserResponseBody[payload.ListIDPsResponse](t, response)
			assert.NoError(t, err, "Failed to parse response body for page %d", pageCount+1)

			allPages = append(allPages, nextPage)
			allIDPs = append(allIDPs, nextPage.Items...)

			// Verify no duplicate IDPs across pages
			for _, idp := range nextPage.Items {
				assert.False(t, seenIDPs[idp.ID], "Found duplicate IDP across pages: %s", idp.ID)
				seenIDPs[idp.ID] = true
				assert.True(t, strings.HasPrefix(idp.Name, testID), "Expected IDP name to start with test ID")
			}

			currentToken = nextPage.Paginator.NextToken
			pageCount++
		}

		// Verify we can navigate backward using prev tokens
		if len(allPages) > 1 && allPages[len(allPages)-1].Paginator.PrevToken != "" {
			prevPageEndpoint := idpsListEndpoint.Clone()
			prevPageEndpoint.SetQueryParam("limit", "4")
			prevPageEndpoint.SetQueryParam("filter", fmt.Sprintf("name LIKE '%s%%'", testID))
			prevPageEndpoint.SetQueryParam("prev_token", allPages[len(allPages)-1].Paginator.PrevToken)

			response, err := sendHTTPRequest(t, ctx, prevPageEndpoint, nil, accessTokenHeader)
			assert.NoError(t, err, "Failed to send request for previous page")
			defer response.Body.Close()

			assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200 for previous page")
		}

		// Ensure we've seen at least 20 IDPs across all pages
		assert.GreaterOrEqual(t, len(allIDPs), 20, "Expected to see at least 20 IDPs across all pages")

		// Verify that we found all the test IDPs we created
		assert.Equal(t, len(idpNames), len(seenIDPs), "Expected to find all created test IDPs Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		// 4. Cleanup
		t.Cleanup(func() {
			for _, idpName := range idpNames {
				deleteIDPByNameFromDB(t, idpName)
			}
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}

func TestIDPList_SortNameDesc(t *testing.T) {
	// Test IDP listing with sorting by name DESC
	t.Run("list_idps_sorted_name_desc", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Define and create specific IDPs via API for sorting test (ensure distinct names)
		idpTypeID := getIDPTypeFromDBByName(t, "Google").ID
		testIDPNames := []string{
			"Alpha-IDP-Sort-Test",
			"Beta-IDP-Sort-Test",
			"Charlie-IDP-Sort-Test",
			"Delta-IDP-Sort-Test",
		}

		for i, idpName := range testIDPNames {
			createData := map[string]any{
				"idp_type_id":           idpTypeID.String(),
				"name":                  idpName,
				"description":           fmt.Sprintf("Test IDP for sorting: %s", idpName),
				"callback_url":          fmt.Sprintf("https://example.com/callback%d", i),
				"login_redirect_url":    fmt.Sprintf("https://example.com/login%d", i),
				"register_redirect_url": fmt.Sprintf("https://example.com/register%d", i),
				"logo":                  fmt.Sprintf("https://example.com/logo%d.png", i),
				"client_id":             fmt.Sprintf("test_client_id_%d", i),
				"client_secret":         fmt.Sprintf("test_client_secret_%d", i),
			}

			response, err := sendHTTPRequest(t, ctx, idpsCreateEndpoint, createData, accessTokenHeader)
			assert.NoError(t, err, "Failed to send request")
			defer response.Body.Close()

			assert.Equal(t, http.StatusCreated, response.StatusCode, "Expected status code 201")
		}

		// 3. List the IDPs with sorting parameter name DESC
		// Create a fresh endpoint instance for this test
		sortedEndpoint := idpsListEndpoint.Clone()
		sortedEndpoint.SetQueryParam("sort", "name DESC") // Sort by name descending

		// Ask for a page big enough to hold this test's IDPs alongside anything
		// another test has created. The suite runs in parallel, so the default
		// page size made this assert "nobody else created an IDP while I ran",
		// which is not what it is checking and fails whenever that is untrue.
		sortedEndpoint.SetQueryParam("limit", "100")
		t.Logf("IDP List Endpoint with Sort: %s", sortedEndpoint.requestURL.String())

		response, err := sendHTTPRequest(t, ctx, sortedEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 4. Check the list response
		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		apiResp, err := parserResponseBody[payload.ListIDPsResponse](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		// 5. Verify the IDPs are sorted correctly by name DESC
		// Extract names from the response, considering only the IDPs we created
		var responseNames []string
		for _, idp := range apiResp.Items {
			for _, createdName := range testIDPNames {
				if idp.Name == createdName {
					responseNames = append(responseNames, idp.Name)
					break
				}
			}
		}

		// Check if the extracted names are sorted in descending order
		for i := 1; i < len(responseNames); i++ {
			assert.True(t, strings.Compare(responseNames[i-1], responseNames[i]) >= 0,
				"Expected IDPs to be sorted by name DESC, but found %s before %s",
				responseNames[i-1], responseNames[i])
		}

		// Also check if all created IDPs were found
		assert.Equal(t, len(testIDPNames), len(responseNames), "Expected to find all created IDPs in the sorted list (name DESC)")

		// 6. Cleanup
		t.Cleanup(func() {
			for _, idpName := range testIDPNames {
				deleteIDPByNameFromDB(t, idpName)
			}
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}

func TestIDPList_SortNameDesc_FilterNameLikePrefix(t *testing.T) {
	// Test IDP listing with sorting by name DESC and filtering by name LIKE
	t.Run("list_idps_sorted_filtered_name_desc", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Define and create specific IDPs via API for sorting/filtering test
		idpTypeID := getIDPTypeFromDBByName(t, "Google").ID
		testIDPNames := []string{
			"Alpha-Filter-Test", // Contains 'l'
			"Beta-Test",         // No 'l'
			"Charlie-Filter",    // Contains 'l'
			"Delta-Test",        // Contains 'l'
			"Echo-NoMatch",      // No 'l'
		}
		expectedFiltered := []string{} // Names that should match the filter (contain 'l')

		for i, idpName := range testIDPNames {
			createData := map[string]any{
				"idp_type_id":           idpTypeID.String(),
				"name":                  idpName,
				"description":           fmt.Sprintf("Test IDP for filter/sort: %s", idpName),
				"callback_url":          fmt.Sprintf("https://example.com/callback%d", i),
				"login_redirect_url":    fmt.Sprintf("https://example.com/login%d", i),
				"register_redirect_url": fmt.Sprintf("https://example.com/register%d", i),
				"logo":                  fmt.Sprintf("https://example.com/logo%d.png", i),
				"client_id":             fmt.Sprintf("test_client_id_%d", i),
				"client_secret":         fmt.Sprintf("test_client_secret_%d", i),
			}

			response, err := sendHTTPRequest(t, ctx, idpsCreateEndpoint, createData, accessTokenHeader)
			assert.NoError(t, err, "Failed to send request")
			defer response.Body.Close()

			assert.Equal(t, http.StatusCreated, response.StatusCode, "Expected status code 201")

			// Determine which names should be included in filtered results
			if strings.Contains(strings.ToLower(idpName), "l") {
				expectedFiltered = append(expectedFiltered, idpName)
			}
		}

		// Sort expectedFiltered in descending order for comparison
		sort.Slice(expectedFiltered, func(i, j int) bool {
			return strings.Compare(expectedFiltered[i], expectedFiltered[j]) > 0
		})

		// 3. List the IDPs with sorting and filtering parameters
		// Create a fresh endpoint instance for this test
		sortedFilteredEndpoint := idpsListEndpoint.Clone()
		sortedFilteredEndpoint.SetQueryParam("sort", "name DESC")         // Sort by name descending
		sortedFilteredEndpoint.SetQueryParam("filter", "name LIKE '%l%'") // Filter by name containing 'l'
		t.Logf("IDP List Endpoint with Sort and Filter: %s", sortedFilteredEndpoint.requestURL.String())

		response, err := sendHTTPRequest(t, ctx, sortedFilteredEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 4. Check the list response
		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		apiResp, err := parserResponseBody[payload.ListIDPsResponse](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		// 5. Verify the IDPs are filtered and sorted correctly
		// Extract names from the response
		var responseNames []string
		for _, idp := range apiResp.Items {
			for _, createdName := range testIDPNames {
				if idp.Name == createdName {
					responseNames = append(responseNames, idp.Name)
					break
				}
			}
		}

		// Check if the number of results matches the expected filtered count
		assert.Equal(t, len(expectedFiltered), len(responseNames),
			"Expected %d filtered results, got %d", len(expectedFiltered), len(responseNames))

		// Check if the extracted names are sorted in descending order
		for i := 1; i < len(responseNames); i++ {
			assert.True(t, strings.Compare(responseNames[i-1], responseNames[i]) >= 0,
				"Expected IDPs to be sorted by name DESC, but found %s before %s",
				responseNames[i-1], responseNames[i])
		}

		// Verify all returned IDPs contain 'l' in their name
		for _, name := range responseNames {
			assert.True(t, strings.Contains(strings.ToLower(name), "l"),
				"Expected filtered IDP name '%s' to contain 'l'", name)
		}

		// 6. Cleanup
		t.Cleanup(func() {
			for _, idpName := range testIDPNames {
				deleteIDPByNameFromDB(t, idpName)
			}
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}

func TestIDPAvailable(t *testing.T) {
	t.Run("success_no_auth", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// No authorization header — this endpoint is publicly accessible
		response, err := sendHTTPRequest(t, ctx, idpsAvailableEndpoint, nil)
		require.NoError(t, err, "Error sending HTTP request")
		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200 OK. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		listResp, err := parserResponseBody[payload.ListIDPAvailableResponse](t, response)
		require.NoError(t, err, "Error parsing response body")

		assert.NotNil(t, listResp.Items, "Available IDPs items should not be nil")
	})
}
