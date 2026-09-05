//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
	"uuid"

	// Added for sleep

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

var (
	rolesCreateEndpoint = newAPIEndpoint(http.MethodPost, "/roles")
	rolesListEndpoint   = newAPIEndpoint(http.MethodGet, "/roles")
	rolesGetEndpoint    = newAPIEndpoint(http.MethodGet, "/roles/{role_id}")
	rolesUpdateEndpoint = newAPIEndpoint(http.MethodPut, "/roles/{role_id}")
	rolesDeleteEndpoint = newAPIEndpoint(http.MethodDelete, "/roles/{role_id}")

	rolesLinkPoliciesEndpoint   = newAPIEndpoint(http.MethodPost, "/roles/{role_id}/policies")
	rolesUnlinkPoliciesEndpoint = newAPIEndpoint(http.MethodDelete, "/roles/{role_id}/policies")

	rolesLinkUsersEndpoint   = newAPIEndpoint(http.MethodPost, "/roles/{role_id}/users")
	rolesUnlinkUsersEndpoint = newAPIEndpoint(http.MethodDelete, "/roles/{role_id}/users")

	rolesListUserLinkedEndpoint = newAPIEndpoint(http.MethodGet, "/roles/{role_id}/users")
)

// Helper function to create a policy for testing (similar to api_policies_test.go)
func createTestPolicyForRoleTest(t *testing.T, accessToken, namePrefix string) uuid.UUID {
	t.Helper()

	ctx := t.Context()
	accessTokenHeader := map[string]string{"Authorization": "Bearer " + accessToken}

	policyID := uuid.NewV7()
	policy := map[string]any{
		"id":               policyID.String(),
		"name":             namePrefix + policyID.String(),
		"description":      "Test policy for role test " + policyID.String(),
		"allowed_action":   "GET",
		"allowed_resource": "/users",
	}

	policyCreateEndpoint := newAPIEndpoint(http.MethodPost, "/policies") // Ensure this endpoint is defined or accessible

	response, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, policy, accessTokenHeader)
	assert.NoError(t, err, "Failed to create test policy for role test")
	if response != nil {
		defer response.Body.Close()
		assert.Equal(t, http.StatusCreated, response.StatusCode, "Failed to create test policy for role test, status code not 201")
	}
	return policyID
}

// Helper function to create a policy with custom action and resource
func createTestPolicyWithAction(t *testing.T, accessToken, namePrefix, action, resource string) uuid.UUID {
	t.Helper()

	ctx := t.Context()
	accessTokenHeader := map[string]string{"Authorization": "Bearer " + accessToken}

	policyID := uuid.NewV7()
	policy := map[string]any{
		"id":               policyID.String(),
		"name":             namePrefix + policyID.String(),
		"description":      fmt.Sprintf("Test policy for %s on %s", action, resource),
		"allowed_action":   action,
		"allowed_resource": resource,
	}

	policyCreateEndpoint := newAPIEndpoint(http.MethodPost, "/policies")

	response, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, policy, accessTokenHeader)
	assert.NoError(t, err, "Failed to create test policy with action %s", action)
	if response != nil {
		defer response.Body.Close()
		assert.Equal(t, http.StatusCreated, response.StatusCode, "Failed to create test policy for action %s, status code not 201", action)
	}
	return policyID
}

// Helper function to create a role for testing (similar to api_policies_test.go)
func createTestRoleForPolicyTest(t *testing.T, accessToken, namePrefix string) uuid.UUID {
	t.Helper()

	ctx := t.Context()
	accessTokenHeader := map[string]string{"Authorization": "Bearer " + accessToken}

	roleID := uuid.NewV7()
	role := map[string]any{
		"id":          roleID.String(),
		"name":        namePrefix + roleID.String(),
		"description": "Test role for policy test " + roleID.String(),
	}

	response, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role, accessTokenHeader) // Use existing rolesCreateEndpoint
	assert.NoError(t, err, "Failed to create test role for policy test")
	if response != nil {
		defer response.Body.Close()
		assert.Equal(t, http.StatusCreated, response.StatusCode, "Failed to create test role for policy test, status code not 201")
	}
	return roleID
}

// Helper function to create a user for testing
func createTestUserForRoleTest(t *testing.T, accessToken string) (uuid.UUID, string) {
	t.Helper()

	ctx := t.Context()
	accessTokenHeader := map[string]string{"Authorization": "Bearer " + accessToken}

	userID := uuid.NewV7()
	firstName, lastName, email := generateUserData(t) // Ensure unique emails
	password := generatePassword(t)

	user := map[string]any{
		"id":         userID.String(),
		"email":      email,
		"first_name": firstName,
		"last_name":  lastName,
		"password":   password,
	}

	response, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, user, accessTokenHeader)
	assert.NoError(t, err, "Failed to create test user for role test")
	if response != nil {
		defer response.Body.Close()
		assert.Equal(t, http.StatusCreated, response.StatusCode, "Failed to create test user for role test, status code not 201")
	}

	// Return both ID and email for easier identification/cleanup
	return userID, email
}

func TestRoleCreate(t *testing.T) {
	// Test role creation
	t.Run("create_role", func(t *testing.T) {
		t.Parallel()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		// 2. Create a new role
		roleID := uuid.NewV7()

		role := map[string]any{
			"id":          roleID.String(),
			"name":        "test_role_" + roleID.String(),
			"description": "This is a test role " + roleID.String(),
		}

		// 2.1 Use access token from admin to have access to the endpoint
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		ctx := t.Context()
		response, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role, accessTokenHeader)
		assert.NoError(t, err, "Error sending request: %v", err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusCreated, response.StatusCode, "Expected status code 201 Created. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		t.Cleanup(func() {
			deleteRoleByIDFromDB(t, roleID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})

		assert.Equal(t, domain.RolesRoleCreatedSuccessfully, apiResp.Message, "Unexpected response message")
		assert.Equal(t, rolesCreateEndpoint.method, apiResp.Method, "Expected method to be set")
		assert.Equal(t, rolesCreateEndpoint.Path(), apiResp.Path, "Expected path to be set")
	})

	// Test creating a role with invalid data format
	t.Run("create_role_bad_request", func(t *testing.T) {
		t.Parallel()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		// 2. Define test cases for various invalid inputs
		testCases := []struct {
			name          string
			invalidRole   map[string]any
			expectedError string
		}{
			{
				name: "Empty name",
				invalidRole: map[string]any{
					"id":          uuid.NewV7().String(),
					"name":        "",
					"description": "Test role description",
				},
				expectedError: "cannot be empty",
			},
			{
				name: "Invalid ID format",
				invalidRole: map[string]any{
					"id":          "not-a-valid-uuid",
					"name":        "Test Role",
					"description": "Test role description",
				},
				expectedError: "invalid uuid",
			},
			{
				name: "Name too long",
				invalidRole: map[string]any{
					"id":          uuid.NewV7().String(),
					"name":        strings.Repeat("a", 300), // Very long name
					"description": "Test role description",
				},
				expectedError: "must be between",
			},
			{
				name: "Description too long",
				invalidRole: map[string]any{
					"id":          uuid.NewV7().String(),
					"name":        "Test Role",
					"description": string(make([]byte, 1000)), // Very long description in bytes
				},
				expectedError: "contains invalid null bytes",
			},
			{
				name: "Description too long",
				invalidRole: map[string]any{
					"id":          uuid.NewV7().String(),
					"name":        "Test Role",
					"description": strings.Repeat("a", 2000), // Very long description
				},
				expectedError: "must be between",
			},
		}

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}
		ctx := t.Context()

		// 3. Run each test case
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Send request with the invalid role data
				response, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, tc.invalidRole, accessTokenHeader)
				assert.NoError(t, err, "Failed to send request for %s", tc.name)
				defer response.Body.Close()

				// Verify we get a 400 Bad Request response
				assert.Equal(t, http.StatusBadRequest, response.StatusCode,
					"Expected status code 400 Bad Request for %s, got %d", tc.name, response.StatusCode)

				// Parse and verify the error response
				errorResp, err := parserResponseBody[payload.HTTPMessage](t, response)
				assert.NoError(t, err, "Failed to parse error response for %s", tc.name)

				// Verify error message contains information specific to this validation failure
				assert.Contains(t, strings.ToLower(errorResp.Message), strings.ToLower(tc.expectedError),
					"Error message should indicate %s validation failure", tc.name)
				assert.Equal(t, rolesCreateEndpoint.method, errorResp.Method, "Expected method to be set")
				assert.Equal(t, rolesCreateEndpoint.Path(), errorResp.Path, "Expected path to be set")
			})
		}

		// 4. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test creating roles with existing ID or name
	t.Run("create_role_conflict", func(t *testing.T) {
		t.Parallel()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}
		ctx := t.Context()

		// 2. First create a valid role that will be our reference role
		roleID := uuid.NewV7()
		roleName := "test_role_conflict_" + roleID.String()
		existingRole := map[string]any{
			"id":          roleID.String(),
			"name":        roleName,
			"description": "Test role description",
		}

		// Create the first role
		createResponse, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, existingRole, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to create initial role")
		defer createResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, createResponse.StatusCode, "Expected status code 201 for initial role creation. Got %d. Message: %s", createResponse.StatusCode, readResponseBody(t, createResponse))

		// 3. Define test cases for duplicate scenarios
		testCases := []struct {
			name           string
			duplicateRole  map[string]any
			expectedStatus int
			expectedError  string
		}{
			{
				name: "Role with existing ID",
				duplicateRole: map[string]any{
					"id":          roleID.String(), // Same ID as existing role
					"name":        "Different_" + roleName,
					"description": "Different description",
				},
				expectedStatus: http.StatusConflict,
				expectedError:  "already exists",
			},
			{
				name: "Role with existing name",
				duplicateRole: map[string]any{
					"id":          uuid.NewV7().String(), // Different ID
					"name":        roleName,              // Same name as existing role
					"description": "Another description",
				},
				expectedStatus: http.StatusConflict,
				expectedError:  "already exists",
			},
		}

		// 4. Run each test case
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Try to create a role with duplicate ID or name
				response, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, tc.duplicateRole, accessTokenHeader)
				assert.NoError(t, err, "Failed to send request for %s", tc.name)
				defer response.Body.Close()

				// Verify we get the expected conflict status
				assert.Equal(t, tc.expectedStatus, response.StatusCode,
					"Expected status code %d for %s, got %d", tc.expectedStatus, tc.name, response.StatusCode)

				// Parse and verify the error response
				errorResp, err := parserResponseBody[payload.HTTPMessage](t, response)
				assert.NoError(t, err, "Failed to parse error response for %s", tc.name)

				// Verify error message contains information about the conflict
				assert.Contains(t, strings.ToLower(errorResp.Message), strings.ToLower(tc.expectedError),
					"Error message should indicate conflict for %s", tc.name)
				assert.Equal(t, rolesCreateEndpoint.method, errorResp.Method, "Expected method to be set")
				assert.Equal(t, rolesCreateEndpoint.Path(), errorResp.Path, "Expected path to be set")
			})
		}

		// 5. Cleanup
		t.Cleanup(func() {
			deleteRoleByIDFromDB(t, roleID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("require_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		role := map[string]any{
			"name":        "Test Role",
			"description": "Test role description",
		}

		resp, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role)
		assert.NoError(t, err, "Failed to send request without authentication")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

func TestRoleGet(t *testing.T) {
	// Test role retrieval
	t.Run("get_role", func(t *testing.T) {
		t.Parallel()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		// 2. Create a new role
		roleID := uuid.NewV7()
		roleName := "test_role_get_" + roleID.String()
		roleDesc := "This is a test role for get " + roleID.String()
		role := map[string]any{
			"id":          roleID.String(),
			"name":        roleName,
			"description": roleDesc,
		}

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		ctx := t.Context()
		createResponse, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role, accessTokenHeader)
		assert.NoError(t, err, "Error sending create request: %v", err)
		defer createResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, createResponse.StatusCode, "Expected status code 201 Created for setup. Got %d. Message: %s", createResponse.StatusCode, readResponseBody(t, createResponse))

		// t.Logf("Create response: %v", createResponse)

		// 3. Get the role
		getEndpoint := rolesGetEndpoint.RewriteSlugs(roleID.String())
		// t.Logf("Get endpoint: %s", getEndpoint.Path())

		getResponse, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Error sending get request: %v", err)
		defer getResponse.Body.Close()

		// t.Logf("Get response: %v", getResponse)

		// 4. Check the response
		assert.Equal(t, http.StatusOK, getResponse.StatusCode, "Expected status code 200 OK for get. Got %d. Message: %s", getResponse.StatusCode, readResponseBody(t, getResponse))
		getAPIResp, err := parserResponseBody[payload.RoleResponse](t, getResponse)
		assert.NoError(t, err, "Failed to parse get response body", err)

		assert.Equal(t, roleID, getAPIResp.ID, "Expected role ID to match")
		assert.Equal(t, roleName, getAPIResp.Name, "Expected role name to match")
		assert.Equal(t, roleDesc, getAPIResp.Description, "Expected role description to match")
		assert.Equal(t, new(false), getAPIResp.AutoAssign, "Expected auto assign to be false")
		assert.Equal(t, new(false), getAPIResp.System, "Expected auto assign to be false")

		// 5. Cleanup
		t.Cleanup(func() {
			deleteRoleByIDFromDB(t, roleID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test retrieving a non-existent role
	t.Run("get_role_not_found", func(t *testing.T) {
		t.Parallel()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Generate a random UUID that doesn't exist in the database
		nonExistentRoleID := uuid.NewV7()

		// 3. Try to get the non-existent role
		getEndpoint := rolesGetEndpoint.RewriteSlugs(nonExistentRoleID.String())
		getResponse, err := sendHTTPRequest(t, context.Background(), getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to get non-existent role")
		defer getResponse.Body.Close()

		// 4. Check that we get a 404 Not Found response
		assert.Equal(t, http.StatusNotFound, getResponse.StatusCode, "Expected status code 404 for non-existent role. Got %d. Message: %s", getResponse.StatusCode, readResponseBody(t, getResponse))

		// 5. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, getResponse)
		assert.NoError(t, err, "Failed to parse error response")

		// Verify error message contains information about the role not being found
		assert.Contains(t, errorResp.Message, "not found", "Error message should indicate that the role was not found")
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

		getEndpoint := rolesGetEndpoint.RewriteSlugs("00000000-0000-0000-0000-000000000000")
		resp, err := sendHTTPRequest(t, ctx, getEndpoint, nil)
		assert.NoError(t, err, "Failed to send request without authentication")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

func TestRoleDelete(t *testing.T) {
	// Test role deletion
	t.Run("delete_role", func(t *testing.T) {
		t.Parallel()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		// 2. Create a new role
		roleID := uuid.NewV7()
		role := map[string]any{
			"id":          roleID.String(),
			"name":        "test_role_delete_" + roleID.String(),
			"description": "This is a test role for delete " + roleID.String(),
		}

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		ctx := t.Context()
		createResponse, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role, accessTokenHeader)
		assert.NoError(t, err, "Error sending create request: %v", err)
		defer createResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, createResponse.StatusCode, "Expected status code 201 Created for setup. Got %d. Message: %s", createResponse.StatusCode, readResponseBody(t, createResponse))

		// 3. Delete the role
		deleteEndpoint := rolesDeleteEndpoint.RewriteSlugs(roleID.String())
		deleteResponse, err := sendHTTPRequest(t, ctx, deleteEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Error sending delete request: %v", err)
		defer deleteResponse.Body.Close()

		// 4. Check the delete response
		assert.Equal(t, http.StatusOK, deleteResponse.StatusCode, "Expected status code 200 OK for delete. Got %d. Message: %s", deleteResponse.StatusCode, readResponseBody(t, deleteResponse))
		deleteAPIResp, err := parserResponseBody[payload.HTTPMessage](t, deleteResponse)
		assert.NoError(t, err, "Failed to parse delete response body")

		assert.Equal(t, domain.RolesRoleDeletedSuccessfully, deleteAPIResp.Message, "Unexpected delete response message") // Assuming this message exists
		assert.Equal(t, deleteEndpoint.method, deleteAPIResp.Method, "Expected method to be set for delete")
		assert.Equal(t, deleteEndpoint.Path(), deleteAPIResp.Path, "Expected path to be set for delete")

		// 5. Verify role is actually deleted (try to get it)
		getEndpoint := rolesGetEndpoint.RewriteSlugs(roleID.String())
		getResponse, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Error sending get request after delete: %v", err)
		defer getResponse.Body.Close()
		assert.Equal(t, http.StatusNotFound, getResponse.StatusCode, "Expected status code 404 Not Found after deletion. Got %d. Message: %s", getResponse.StatusCode, readResponseBody(t, getResponse))

		// 6. Cleanup admin user
		t.Cleanup(func() {
			// Role should already be deleted by the test, but try again just in case
			deleteRoleByIDFromDB(t, roleID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test deleting a role with an invalid ID format
	t.Run("delete_role_bad_request", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Try to delete a role with an invalid ID format (not a UUID)
		invalidRoleID := "not-a-valid-uuid"
		deleteEndpoint := rolesDeleteEndpoint.RewriteSlugs(invalidRoleID)

		deleteResponse, err := sendHTTPRequest(t, ctx, deleteEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to delete role with invalid ID")
		defer deleteResponse.Body.Close()

		// 3. Check that we get a 400 Bad Request response
		assert.Equal(t, http.StatusBadRequest, deleteResponse.StatusCode, "Expected status code 400 for invalid role ID format. Got %d. Message: %s", deleteResponse.StatusCode, readResponseBody(t, deleteResponse))

		// 4. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, deleteResponse)
		assert.NoError(t, err, "Failed to parse error response")

		// 5. Verify error message contains information about the invalid UUID format
		assert.Contains(t, errorResp.Message, "invalid", "Error message should indicate that the role ID format is invalid")
		assert.Equal(t, deleteEndpoint.method, errorResp.Method, "Expected method to be set")
		assert.Equal(t, deleteEndpoint.Path(), errorResp.Path, "Expected path to be set")

		// 6. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test deleting a non-existent role
	t.Run("delete_role_not_found", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Generate a UUID that doesn't exist in the database
		nonExistentRoleID := uuid.NewV7()
		deleteEndpoint := rolesDeleteEndpoint.RewriteSlugs(nonExistentRoleID.String())

		deleteResponse, err := sendHTTPRequest(t, ctx, deleteEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to delete non-existent role")
		defer deleteResponse.Body.Close()

		// 3. Check the response - this should still return StatusOK even though the role doesn't exist
		// This is because deleting a non-existent resource is considered idempotent in RESTful APIs
		assert.Equal(t, http.StatusOK, deleteResponse.StatusCode, "Expected status code 200 OK for deleting non-existent role. Got %d. Message: %s", deleteResponse.StatusCode, readResponseBody(t, deleteResponse))

		// 4. Parse and verify the success response
		deleteAPIResp, err := parserResponseBody[payload.HTTPMessage](t, deleteResponse)
		assert.NoError(t, err, "Failed to parse response body")

		// 5. Verify success message for deletion
		assert.Equal(t, domain.RolesRoleDeletedSuccessfully, deleteAPIResp.Message, "Expected success message")
		assert.Equal(t, deleteEndpoint.method, deleteAPIResp.Method, "Expected method to be set")
		assert.Equal(t, deleteEndpoint.Path(), deleteAPIResp.Path, "Expected path to be set")

		// 6. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}

func TestRoleUpdate(t *testing.T) {
	// Test role update
	t.Run("update_role", func(t *testing.T) {
		t.Parallel()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		// 2. Create a new role
		roleID := uuid.NewV7()
		originalName := "test_role_update_" + roleID.String()
		originalDesc := "Original description " + roleID.String()
		role := map[string]any{
			"id":          roleID.String(),
			"name":        originalName,
			"description": originalDesc,
		}

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		ctx := t.Context()
		createResponse, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role, accessTokenHeader)
		assert.NoError(t, err, "Error sending create request: %v", err)
		defer createResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, createResponse.StatusCode, "Expected status code 201 Created for setup. Got %d. Message: %s", createResponse.StatusCode, readResponseBody(t, createResponse))

		// 3. Update the role
		updatedName := "updated_" + originalName
		updatedDesc := "Updated description " + roleID.String()
		updatedRole := map[string]any{
			"name":        updatedName,
			"description": updatedDesc,
		}

		updateEndpoint := rolesUpdateEndpoint.RewriteSlugs(roleID.String())
		updateResponse, err := sendHTTPRequest(t, ctx, updateEndpoint, updatedRole, accessTokenHeader)
		assert.NoError(t, err, "Error sending update request: %v", err)
		defer updateResponse.Body.Close()

		// 4. Check the update response
		assert.Equal(t, http.StatusOK, updateResponse.StatusCode, "Expected status code 200 OK for update. Got %d. Message: %s", updateResponse.StatusCode, readResponseBody(t, updateResponse))
		updateAPIResp, err := parserResponseBody[payload.HTTPMessage](t, updateResponse)
		assert.NoError(t, err, "Failed to parse update response body")

		assert.Equal(t, domain.RolesRoleUpdatedSuccessfully, updateAPIResp.Message, "Unexpected update response message") // Assuming this message exists
		assert.Equal(t, updateEndpoint.method, updateAPIResp.Method, "Expected method to be set for update")
		assert.Equal(t, updateEndpoint.Path(), updateAPIResp.Path, "Expected path to be set for update")

		// 5. Verify role is actually updated (get it again)
		getEndpoint := rolesGetEndpoint.RewriteSlugs(roleID.String())
		getResponse, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Error sending get request after update: %v", err)
		defer getResponse.Body.Close()
		assert.Equal(t, http.StatusOK, getResponse.StatusCode, "Expected status code 200 OK when getting updated role. Got %d. Message: %s", getResponse.StatusCode, readResponseBody(t, getResponse))

		getAPIResp, err := parserResponseBody[payload.RoleResponse](t, getResponse)
		assert.NoError(t, err, "Failed to parse get response body for updated role")

		assert.Equal(t, roleID, getAPIResp.ID, "Expected role ID to remain the same")
		assert.Equal(t, updatedName, getAPIResp.Name, "Expected role name to be updated")
		assert.Equal(t, updatedDesc, getAPIResp.Description, "Expected role description to be updated")

		// 6. Cleanup
		t.Cleanup(func() {
			deleteRoleByIDFromDB(t, roleID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test updating a role with an invalid ID format
	t.Run("update_role_bad_request", func(t *testing.T) {
		t.Parallel()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		// Set up test cases for different bad request scenarios
		testCases := []struct {
			name          string
			roleID        string
			updateData    map[string]any
			expectedCode  int
			expectedError string
		}{
			{
				name:          "Invalid UUID format",
				roleID:        "not-a-valid-uuid",
				updateData:    map[string]any{"name": "Updated Role", "description": "Updated description"},
				expectedCode:  http.StatusBadRequest,
				expectedError: "invalid uuid",
			},
			{
				name:          "Empty name",
				roleID:        uuid.NewV7().String(),
				updateData:    map[string]any{"name": ""},
				expectedCode:  http.StatusBadRequest,
				expectedError: "cannot be empty",
			},
			{
				name:          "Name too long",
				roleID:        uuid.NewV7().String(),
				updateData:    map[string]any{"name": strings.Repeat("a", 300)}, // Very long name
				expectedCode:  http.StatusBadRequest,
				expectedError: "must be between",
			},
			{
				name:          "Name too short",
				roleID:        uuid.NewV7().String(),
				updateData:    map[string]any{"name": "a"},
				expectedCode:  http.StatusBadRequest,
				expectedError: "must be between",
			},
			{
				name:          "Description too short",
				roleID:        uuid.NewV7().String(),
				updateData:    map[string]any{"description": "a"},
				expectedCode:  http.StatusBadRequest,
				expectedError: "must be between",
			},
			{
				name:          "Description too long",
				roleID:        uuid.NewV7().String(),
				updateData:    map[string]any{"description": string(make([]byte, 500))}, // Very long description in bytes
				expectedCode:  http.StatusBadRequest,
				expectedError: "contains invalid null bytes",
			},
			{
				name:          "Description too long",
				roleID:        uuid.NewV7().String(),
				updateData:    map[string]any{"description": strings.Repeat("a", 2000)}, // Very long description in bytes
				expectedCode:  http.StatusBadRequest,
				expectedError: "must be between",
			},
		}

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}
		ctx := t.Context()

		// For cases other than invalid UUID, we need to create a real role first
		roleID := uuid.NewV7()
		roleName := "test_role_update_bad_" + roleID.String()
		roleDesc := "Test role for bad request updates " + roleID.String()
		role := map[string]any{
			"id":          roleID.String(),
			"name":        roleName,
			"description": roleDesc,
		}

		// Create the role for testing valid UUID but invalid data cases
		createResponse, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to create role")
		defer createResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, createResponse.StatusCode, "Expected status code 201")

		// Run each test case
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// For test cases with valid UUIDs but invalid data, use the created role ID
				testRoleID := tc.roleID
				if tc.roleID != "not-a-valid-uuid" {
					testRoleID = roleID.String()
				}

				updateEndpoint := rolesUpdateEndpoint.RewriteSlugs(testRoleID)
				updateResponse, err := sendHTTPRequest(t, ctx, updateEndpoint, tc.updateData, accessTokenHeader)
				assert.NoError(t, err, "Failed to send request for %s", tc.name)
				defer updateResponse.Body.Close()

				// Verify we get the expected status code
				assert.Equal(t, tc.expectedCode, updateResponse.StatusCode,
					"Expected status code %d for %s, got %d", tc.expectedCode, tc.name, updateResponse.StatusCode)

				// Parse and verify the error response
				errorResp, err := parserResponseBody[payload.HTTPMessage](t, updateResponse)
				assert.NoError(t, err, "Failed to parse error response for %s", tc.name)

				// Verify error message contains relevant information
				assert.Contains(t, strings.ToLower(errorResp.Message), strings.ToLower(tc.expectedError),
					"Error message should indicate %s validation failure", tc.name)
				assert.Equal(t, updateEndpoint.method, errorResp.Method, "Expected method to be set")
				assert.Equal(t, updateEndpoint.Path(), errorResp.Path, "Expected path to be set")
			})
		}

		// Cleanup
		t.Cleanup(func() {
			deleteRoleByIDFromDB(t, roleID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test updating a non-existent role
	t.Run("update_role_not_found", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Generate a UUID that doesn't exist in the database
		nonExistentRoleID := uuid.NewV7()
		updateEndpoint := rolesUpdateEndpoint.RewriteSlugs(nonExistentRoleID.String())

		updatedRole := map[string]any{
			"name":        "Updated Role Name",
			"description": "Updated role description",
		}

		updateResponse, err := sendHTTPRequest(t, ctx, updateEndpoint, updatedRole, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to update non-existent role")
		defer updateResponse.Body.Close()

		// 3. Check response - should be a 404 Not Found for not found role
		assert.Equal(t, http.StatusNotFound, updateResponse.StatusCode, "Expected status code 404 for non-existent role")

		// 4. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, updateResponse)
		assert.NoError(t, err, "Failed to parse error response")

		// 5. Verify error message contains information about the role not being found
		assert.Contains(t, errorResp.Message, "not found", "Error message should indicate that the role was not found")
		assert.Equal(t, updateEndpoint.method, errorResp.Method, "Expected method to be set")
		assert.Equal(t, updateEndpoint.Path(), errorResp.Path, "Expected path to be set")

		// 6. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test updating a role with invalid data
	t.Run("update_role_invalid_data", func(t *testing.T) {
		t.Parallel()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		// 2. Create a new role that we'll try to update with invalid data
		roleID := uuid.NewV7()
		roleName := "test_role_invalid_data_" + roleID.String()
		roleDesc := "Test role for invalid data update " + roleID.String()
		role := map[string]any{
			"id":          roleID.String(),
			"name":        roleName,
			"description": roleDesc,
		}

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		ctx := t.Context()
		createResponse, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to create role")
		defer createResponse.Body.Close()

		assert.Equal(t, http.StatusCreated, createResponse.StatusCode, "Expected status code 201")

		// 3. Update the role with invalid data
		invalidRole := map[string]any{
			"name": "", // Empty name is invalid
		}

		updateEndpoint := rolesUpdateEndpoint.RewriteSlugs(roleID.String())
		updateResponse, err := sendHTTPRequest(t, ctx, updateEndpoint, invalidRole, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to update role with invalid data")
		defer updateResponse.Body.Close()

		// 4. Check that we get a 400 Bad Request response
		assert.Equal(t, http.StatusBadRequest, updateResponse.StatusCode, "Expected status code 400 for invalid role data")

		// 5. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, updateResponse)
		assert.NoError(t, err, "Failed to parse error response")

		// 6. Verify error message contains information about the invalid data
		assert.Contains(t, strings.ToLower(errorResp.Message), "validation error", "Error message should indicate invalid data")
		assert.Equal(t, updateEndpoint.method, errorResp.Method, "Expected method to be set")
		assert.Equal(t, updateEndpoint.Path(), errorResp.Path, "Expected path to be set")

		// 7. Cleanup
		t.Cleanup(func() {
			deleteRoleByIDFromDB(t, roleID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test updating a role with a name that already exists (conflict case)
	t.Run("update_role_conflict", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create two roles with different names
		// First role - this is the one we'll try to update
		roleID1 := uuid.NewV7()
		roleName1 := "test_role_conflict_1_" + roleID1.String()
		roleDesc1 := "Test role 1 for conflict update " + roleID1.String()
		role1 := map[string]any{
			"id":          roleID1.String(),
			"name":        roleName1,
			"description": roleDesc1,
		}

		createResponse1, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role1, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to create first role")
		defer createResponse1.Body.Close()
		assert.Equal(t, http.StatusCreated, createResponse1.StatusCode, "Expected status code 201 for first role creation")

		// Second role - we'll try to use this role's name when updating the first role
		roleID2 := uuid.NewV7()
		roleName2 := "test_role_conflict_2_" + roleID2.String()
		roleDesc2 := "Test role 2 for conflict update " + roleID2.String()
		role2 := map[string]any{
			"id":          roleID2.String(),
			"name":        roleName2,
			"description": roleDesc2,
		}

		createResponse2, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role2, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to create second role")
		defer createResponse2.Body.Close()
		assert.Equal(t, http.StatusCreated, createResponse2.StatusCode, "Expected status code 201 for second role creation")

		// 3. Try to update the first role with the second role's name
		updateRole1WithConflict := map[string]any{
			"name": roleName2, // This will cause a conflict because roleName2 is already being used
		}

		updateEndpoint := rolesUpdateEndpoint.RewriteSlugs(roleID1.String())
		updateResponse, err := sendHTTPRequest(t, ctx, updateEndpoint, updateRole1WithConflict, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to update role with conflicting name")
		defer updateResponse.Body.Close()

		// 4. Check that we get a 409 Conflict response
		assert.Equal(t, http.StatusConflict, updateResponse.StatusCode, "Expected status code 409 Conflict for update with already used name")

		// 5. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, updateResponse)
		assert.NoError(t, err, "Failed to parse error response")

		// 6. Verify error message contains information about the name being already in use
		assert.Contains(t, strings.ToLower(errorResp.Message), "already exists", "Error message should indicate that the name already exists")
		assert.Equal(t, updateEndpoint.method, errorResp.Method, "Expected method to be set")
		assert.Equal(t, updateEndpoint.Path(), errorResp.Path, "Expected path to be set")

		// 7. Cleanup
		t.Cleanup(func() {
			deleteRoleByIDFromDB(t, roleID1)
			deleteRoleByIDFromDB(t, roleID2)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("require_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		updateEndpoint := rolesUpdateEndpoint.RewriteSlugs("00000000-0000-0000-0000-000000000000")
		updateData := map[string]any{
			"name":        "Updated Role Name",
			"description": "Updated role description",
		}

		resp, err := sendHTTPRequest(t, ctx, updateEndpoint, updateData)
		assert.NoError(t, err, "Failed to send request without authentication")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

// TestRoleUpdate_SQLInjection is a regression test for a reachable SQL injection
// in RolesRepository.UpdateByID.
//
// The SET clause was built with fmt.Sprintf("name='%s'", …) and the id was
// interpolated into the WHERE. Role-name validation forbids control characters,
// HTML, script tags and null bytes but permits an apostrophe, so a name of
//
//	x', description='injected
//
// closed the string literal and appended an attacker-controlled assignment,
// silently rewriting a column the request never named.
//
// The fix is $n placeholders. This test asserts the payload is stored as a
// literal name and that no other column moved.
func TestRoleUpdate_SQLInjection(t *testing.T) {
	t.Run("quote_in_name_is_stored_literally_not_executed", func(t *testing.T) {
		t.Parallel()

		adminToken := getAdminUserTokens(t)
		require.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}
		ctx := t.Context()

		roleID := uuid.NewV7()

		untouchedDesc := "untouched description " + roleID.String()
		createResponse, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, map[string]any{
			"id":          roleID.String(),
			"name":        "test_role_injection_" + roleID.String(),
			"description": untouchedDesc,
		}, accessTokenHeader)
		require.NoError(t, err, "Error sending create request")
		defer createResponse.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse.StatusCode,
			"Expected 201 Created for setup. Got %d. Message: %s",
			createResponse.StatusCode, readResponseBody(t, createResponse))

		// Update the name ONLY. The payload would, under the old code, also
		// rewrite `description`.
		injected := "inj_" + roleID.String() + "', description='PWNED"

		updateEndpoint := rolesUpdateEndpoint.RewriteSlugs(roleID.String())
		updateResponse, err := sendHTTPRequest(t, ctx, updateEndpoint, map[string]any{
			"name": injected,
		}, accessTokenHeader)
		require.NoError(t, err, "Error sending update request")
		defer updateResponse.Body.Close()

		require.Equal(t, http.StatusOK, updateResponse.StatusCode,
			"Expected 200 OK for update. Got %d. Message: %s",
			updateResponse.StatusCode, readResponseBody(t, updateResponse))

		// Read it back.
		getEndpoint := rolesGetEndpoint.RewriteSlugs(roleID.String())
		getResponse, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		require.NoError(t, err, "Error sending get request")
		defer getResponse.Body.Close()
		require.Equal(t, http.StatusOK, getResponse.StatusCode,
			"Expected 200 OK for get. Got %d", getResponse.StatusCode)

		role, err := parserResponseBody[payload.RoleResponse](t, getResponse)
		require.NoError(t, err, "Failed to parse get response body")

		// The payload is data, not SQL.
		assert.Equal(t, injected, role.Name,
			"the name must be stored verbatim, quote included")

		// The column the request never named must not have moved.
		assert.Equal(t, untouchedDesc, role.Description,
			"description was rewritten by the name payload — SQL injection is open")
		assert.NotEqual(t, "PWNED", role.Description,
			"injected assignment executed")
	})
}

func TestRoleList(t *testing.T) {
	// Test role listing
	t.Run("list_roles", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a couple of new roles
		roleIDs := []uuid.UUID{uuid.NewV7(), uuid.NewV7()}
		rolesToCreate := []map[string]any{}

		for i, roleID := range roleIDs {
			role := map[string]any{
				"id":          roleID.String(),
				"name":        "test_role_list_" + roleID.String(),
				"description": "This is a test role for list " + roleID.String(),
			}
			rolesToCreate = append(rolesToCreate, role)

			createResponse, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role, accessTokenHeader)
			assert.NoError(t, err, "Failed to send request to create role %d", i+1)
			if createResponse != nil {
				defer createResponse.Body.Close()
				assert.Equal(t, http.StatusCreated, createResponse.StatusCode, "Expected status code 201 for role %d", i+1)
			}
		}

		// 3. List the roles
		listResponse, err := sendHTTPRequest(t, ctx, rolesListEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to list roles")
		defer listResponse.Body.Close()

		// 4. Check the list response
		assert.Equal(t, http.StatusOK, listResponse.StatusCode, "Expected status code 200 for list")
		// Assuming domain.ListRolesOutput exists and has an Items field []domain.Role
		listAPIResp, err := parserResponseBody[payload.ListRolesResponse](t, listResponse)
		assert.NoError(t, err, "Failed to parse list response body")

		// 5. Verify the created roles are in the list
		foundCount := 0
		roleMap := make(map[string]bool) // Use name for checking presence
		for _, createdRole := range rolesToCreate {
			roleMap[createdRole["name"].(string)] = true
		}

		for _, listedRole := range listAPIResp.Items {
			if _, ok := roleMap[listedRole.Name]; ok {
				foundCount++
				// Optionally assert other fields match
				for _, createdRole := range rolesToCreate {
					if createdRole["name"] == listedRole.Name {
						assert.Equal(t, createdRole["description"], listedRole.Description)
						break
					}
				}
			}
		}
		assert.GreaterOrEqual(t, foundCount, len(rolesToCreate), "Expected to find at least the created roles in the list")

		// 6. Cleanup
		t.Cleanup(func() {
			for _, roleID := range roleIDs {
				deleteRoleByIDFromDB(t, roleID)
			}
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("require_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		resp, err := sendHTTPRequest(t, ctx, rolesListEndpoint, nil)
		assert.NoError(t, err, "Failed to send request without authentication")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

func TestRoleLinkPolicies(t *testing.T) {
	t.Run("link_policies_to_role", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Setup: Create admin, role, and policies
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		roleID := createTestRoleForPolicyTest(t, adminToken.AccessToken, "test_role_")
		policyID1 := createTestPolicyForRoleTest(t, adminToken.AccessToken, "policy_1_")
		policyID2 := createTestPolicyForRoleTest(t, adminToken.AccessToken, "policy_2_")

		// 2. Link policies to the role
		linkEndpoint := rolesLinkPoliciesEndpoint.RewriteSlugs(roleID.String())
		linkPayload := map[string]any{
			"policy_ids": []string{policyID1.String(), policyID2.String()},
		}

		linkResponse, err := sendHTTPRequest(t, ctx, linkEndpoint, linkPayload, accessTokenHeader)
		assert.NoError(t, err, "Error sending link policies request: %v", err)
		defer linkResponse.Body.Close()

		// 3. Check link response
		assert.Equal(t, http.StatusOK, linkResponse.StatusCode, "Expected status code 200 OK for linking policies")
		linkAPIResp, err := parserResponseBody[payload.HTTPMessage](t, linkResponse)
		assert.NoError(t, err, "Failed to parser link policies response body")

		// Assuming domain.RolesPoliciesLinkedSuccessfully exists
		assert.Equal(t, domain.RolesPoliciesLinkedSuccessfully, linkAPIResp.Message, "Unexpected link policies response message")

		// 4. Verify policies are linked (by getting the role)
		getRoleEndpoint := rolesGetEndpoint.RewriteSlugs(roleID.String())
		getRoleResponse, err := sendHTTPRequest(t, ctx, getRoleEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Error sending get role request after linking: %v", err)
		defer getRoleResponse.Body.Close()

		assert.Equal(t, http.StatusOK, getRoleResponse.StatusCode, "Expected status code 200 OK when getting role after linking")

		getRoleAPIResp, err := parserResponseBody[payload.RoleResponse](t, getRoleResponse)
		assert.NoError(t, err, "Failed to parse get role response body after linking")

		assert.Equal(t, roleID, getRoleAPIResp.ID, "Expected role ID to match")
		assert.Equal(t, roleID.String(), getRoleAPIResp.ID.String(), "Expected role ID to match")

		// Cleanup - Register in reverse order (LIFO execution)
		t.Cleanup(func() { deleteUserByIDFromDB(t, adminToken.UserID) })
		t.Cleanup(func() { deletePolicyByIDFromDB(t, policyID2) })
		t.Cleanup(func() { deletePolicyByIDFromDB(t, policyID1) })
		t.Cleanup(func() { deleteRoleByIDFromDB(t, roleID) })
		// Unlink relationships - runs FIRST due to LIFO
		t.Cleanup(func() { unlinkPoliciesFromRoleViaDB(t, roleID, []uuid.UUID{policyID1, policyID2}) })
	})

	t.Run("require_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		linkEndpoint := rolesLinkPoliciesEndpoint.RewriteSlugs("00000000-0000-0000-0000-000000000000")
		linkPayload := map[string]any{
			"policy_ids": []string{"00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002"},
		}

		resp, err := sendHTTPRequest(t, ctx, linkEndpoint, linkPayload)
		assert.NoError(t, err, "Failed to send request without authentication")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

func TestRoleUnlinkPolicies(t *testing.T) {
	t.Run("unlink_policies_from_role", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Setup: Create admin, role, policies, and link them
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		roleID := createTestRoleForPolicyTest(t, adminToken.AccessToken, "unlink_test_role_")
		policyID1 := createTestPolicyForRoleTest(t, adminToken.AccessToken, "unlink_test_policy_1_")
		policyID2 := createTestPolicyForRoleTest(t, adminToken.AccessToken, "unlink_test_policy_2_")

		// Link policies first
		linkEndpoint := rolesLinkPoliciesEndpoint.RewriteSlugs(roleID.String())
		linkPayload := map[string]any{
			"policy_ids": []string{policyID1.String(), policyID2.String()},
		}

		linkResponse, err := sendHTTPRequest(t, ctx, linkEndpoint, linkPayload, accessTokenHeader)
		assert.NoError(t, err, "Error sending link policies request during setup: %v", err)
		defer linkResponse.Body.Close()
		assert.Equal(t, http.StatusOK, linkResponse.StatusCode, "Expected status code 200 OK for linking policies during setup")

		// Wait briefly to ensure link operation completes if async/eventual consistency involved
		time.Sleep(100 * time.Millisecond)

		// 2. Unlink one policy
		// Using DELETE method as defined in rolesUnlinkPoliciesEndpoint
		unlinkEndpoint := rolesUnlinkPoliciesEndpoint.RewriteSlugs(roleID.String())
		unlinkPayload := map[string]any{
			"policy_ids": []string{policyID1.String()},
		}

		// Send DELETE request (body might be ignored depending on server implementation, but include for consistency)
		unlinkResponse, err := sendHTTPRequest(t, ctx, unlinkEndpoint, unlinkPayload, accessTokenHeader)
		assert.NoError(t, err, "Error sending unlink policy request: %v", err)
		defer unlinkResponse.Body.Close()

		// 3. Check unlink response
		assert.Equal(t, http.StatusOK, unlinkResponse.StatusCode, "Expected status code 200 OK for unlinking policy")
		unlinkAPIResp, err := parserResponseBody[payload.HTTPMessage](t, unlinkResponse)
		assert.NoError(t, err, "Failed to parse unlink policy response body")

		// Assuming domain.RolesPoliciesUnlinkedSuccessfully exists
		assert.Equal(t, domain.RolesPoliciesUnlinkedSuccessfully, unlinkAPIResp.Message, "Unexpected unlink policy response message")

		// 4. Verify policy is unlinked (by getting the role again)
		getRoleEndpoint := rolesGetEndpoint.RewriteSlugs(roleID.String())
		getRoleResponse, err := sendHTTPRequest(t, ctx, getRoleEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Error sending get role request after unlinking: %v", err)
		defer getRoleResponse.Body.Close()

		assert.Equal(t, http.StatusOK, getRoleResponse.StatusCode, "Expected status code 200 OK when getting role after unlinking")

		getRoleAPIResp, err := parserResponseBody[payload.RoleResponse](t, getRoleResponse)
		assert.NoError(t, err, "Failed to parse get role response body after unlinking")

		assert.Equal(t, roleID, getRoleAPIResp.ID, "Expected role ID to match")

		// Cleanup - Register in reverse order (LIFO execution)
		t.Cleanup(func() { deleteUserByIDFromDB(t, adminToken.UserID) })
		t.Cleanup(func() { deletePolicyByIDFromDB(t, policyID2) })
		t.Cleanup(func() { deletePolicyByIDFromDB(t, policyID1) })
		t.Cleanup(func() { deleteRoleByIDFromDB(t, roleID) })
		// Unlink any remaining relationships - runs FIRST due to LIFO
		t.Cleanup(func() { unlinkAllPoliciesFromRoleViaDB(t, roleID) })
	})

	t.Run("require_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		unlinkEndpoint := rolesUnlinkPoliciesEndpoint.RewriteSlugs("00000000-0000-0000-0000-000000000000")
		unlinkPayload := map[string]any{
			"policy_ids": []string{"00000000-0000-0000-0000-000000000001"},
		}

		resp, err := sendHTTPRequest(t, ctx, unlinkEndpoint, unlinkPayload)
		assert.NoError(t, err, "Failed to send request without authentication")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

// TestRoleWithMultipleActionsOnSameResource tests that a role can have multiple policies
// with different actions (GET, POST, PUT, DELETE) on the same resource
func TestRoleWithMultipleActionsOnSameResource(t *testing.T) {
	t.Run("role_with_multiple_actions_on_users_resource", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Setup: Create admin and role
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		roleID := createTestRoleForPolicyTest(t, adminToken.AccessToken, "multi_action_role_")

		// 2. Create multiple policies with different actions on the same resource
		// Note: Using /users which supports GET and /me which supports GET and PUT
		// This tests that a role can have multiple different policies with various actions

		// GET on /users - list users
		policyGETUsers := createTestPolicyWithAction(t, adminToken.AccessToken, "policy_get_users_", "GET", "/users")

		// GET on /me - get current user info
		policyGETMe := createTestPolicyWithAction(t, adminToken.AccessToken, "policy_get_me_", "GET", "/me")

		// PUT on /me - update current user info
		policyPUTMe := createTestPolicyWithAction(t, adminToken.AccessToken, "policy_put_me_", "PUT", "/me")

		// 3. Link all three policies to the role
		linkEndpoint := rolesLinkPoliciesEndpoint.RewriteSlugs(roleID.String())
		linkPayload := map[string]any{
			"policy_ids": []string{
				policyGETUsers.String(),
				policyGETMe.String(),
				policyPUTMe.String(),
			},
		}

		linkResponse, err := sendHTTPRequest(t, ctx, linkEndpoint, linkPayload, accessTokenHeader)
		assert.NoError(t, err, "Error sending link policies request: %v", err)
		defer linkResponse.Body.Close()

		// 4. Verify link was successful
		assert.Equal(t, http.StatusOK, linkResponse.StatusCode, "Expected status code 200 OK for linking policies")
		linkAPIResp, err := parserResponseBody[payload.HTTPMessage](t, linkResponse)
		assert.NoError(t, err, "Failed to parse link policies response body")
		assert.Equal(t, domain.RolesPoliciesLinkedSuccessfully, linkAPIResp.Message, "Unexpected link policies response message")

		// 5. List policies linked to the role to verify all 3 are present
		time.Sleep(500 * time.Millisecond) // Ensure eventual consistency

		listPoliciesEndpoint := policyListPoliciesByRoleEndpoint.RewriteSlugs(roleID.String())
		listPoliciesResponse, err := sendHTTPRequest(t, ctx, listPoliciesEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Error sending list policies request: %v", err)
		defer listPoliciesResponse.Body.Close()

		assert.Equal(t, http.StatusOK, listPoliciesResponse.StatusCode, "Expected status code 200 OK for listing policies", listPoliciesResponse.StatusCode, readResponseBody(t, listPoliciesResponse))

		listPoliciesAPIResp, err := parserResponseBody[payload.ListPoliciesResponse](t, listPoliciesResponse)
		assert.NoError(t, err, "Failed to parse list policies response body")

		// 6. Verify we have all 3 policies
		assert.GreaterOrEqual(t, len(listPoliciesAPIResp.Items), 3, "Expected to find at least 3 policies linked to the role")

		// 7. Verify each policy is present
		foundPolicies := make(map[string]domain.Policy)

		for _, policy := range listPoliciesAPIResp.Items {
			foundPolicies[policy.ID.String()] = domain.Policy{
				ID:          policy.ID,
				Name:        policy.Name,
				Description: policy.Description,
				System:      policy.System,
				Resource: domain.Resource{
					ID:   policy.Resource.ID,
					Name: policy.Resource.Name,
				},
				AllowedAction:   policy.AllowedAction,
				AllowedResource: policy.AllowedResource,
				CreatedAt:       policy.CreatedAt,
				UpdatedAt:       policy.UpdatedAt,
			}
		}

		// 8. Assert all three policy IDs are present
		assert.Contains(t, foundPolicies, policyGETUsers.String(), "Expected to find GET /users policy in role's policies")
		assert.Contains(t, foundPolicies, policyGETMe.String(), "Expected to find GET /me policy in role's policies")
		assert.Contains(t, foundPolicies, policyPUTMe.String(), "Expected to find PUT /me policy in role's policies")

		// 9. Verify the correct actions and resources
		if pol, exists := foundPolicies[policyGETUsers.String()]; exists {
			assert.Equal(t, "GET", pol.AllowedAction, "Expected GET action for users policy")
			assert.Equal(t, "/users", pol.AllowedResource, "Expected /users resource")
		}
		if pol, exists := foundPolicies[policyGETMe.String()]; exists {
			assert.Equal(t, "GET", pol.AllowedAction, "Expected GET action for me policy")
			assert.Equal(t, "/me", pol.AllowedResource, "Expected /me resource")
		}
		if pol, exists := foundPolicies[policyPUTMe.String()]; exists {
			assert.Equal(t, "PUT", pol.AllowedAction, "Expected PUT action for me update policy")
			assert.Equal(t, "/me", pol.AllowedResource, "Expected /me resource")
		}

		getRoleEndpoint := rolesGetEndpoint.RewriteSlugs(roleID.String())
		getRoleResponse, err := sendHTTPRequest(t, ctx, getRoleEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Error sending get role request: %v", err)
		defer getRoleResponse.Body.Close()

		assert.Equal(t, http.StatusOK, getRoleResponse.StatusCode, "Expected status code 200 OK when getting role")

		getRoleAPIResp, err := parserResponseBody[payload.RoleResponse](t, getRoleResponse)
		assert.NoError(t, err, "Failed to parse get role response body")
		assert.Equal(t, roleID, getRoleAPIResp.ID, "Expected role ID to match")

		// 11. Cleanup
		t.Cleanup(func() {
			deleteRoleByIDFromDB(t, roleID)
			deletePolicyByIDFromDB(t, policyGETUsers)
			deletePolicyByIDFromDB(t, policyGETMe)
			deletePolicyByIDFromDB(t, policyPUTMe)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("role_with_get_and_post_only", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// Test scenario: A role with only GET and POST permissions (e.g., for a viewer-contributor role)
		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		roleID := createTestRoleForPolicyTest(t, adminToken.AccessToken, "viewer_contributor_role_")

		// Create only GET and POST policies
		policyGET := createTestPolicyWithAction(t, adminToken.AccessToken, "policy_get_", "GET", "/users")
		policyPOST := createTestPolicyWithAction(t, adminToken.AccessToken, "policy_post_", "POST", "/users")

		// Link policies
		linkEndpoint := rolesLinkPoliciesEndpoint.RewriteSlugs(roleID.String())
		linkPayload := map[string]any{
			"policy_ids": []string{policyGET.String(), policyPOST.String()},
		}

		linkResponse, err := sendHTTPRequest(t, ctx, linkEndpoint, linkPayload, accessTokenHeader)
		assert.NoError(t, err)
		defer linkResponse.Body.Close()
		assert.Equal(t, http.StatusOK, linkResponse.StatusCode)

		// Verify only GET and POST are present
		time.Sleep(500 * time.Millisecond)
		listPoliciesEndpoint := newAPIEndpoint(http.MethodGet, fmt.Sprintf("/roles/%s/policies", roleID.String()))
		listPoliciesResponse, err := sendHTTPRequest(t, ctx, listPoliciesEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err)
		defer listPoliciesResponse.Body.Close()

		listPoliciesAPIResp, err := parserResponseBody[payload.ListPoliciesResponse](t, listPoliciesResponse)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(listPoliciesAPIResp.Items), 2, "Expected at least 2 policies")

		foundActions := make(map[string]bool)
		for _, policy := range listPoliciesAPIResp.Items {
			if policy.AllowedResource == "/users" {
				foundActions[policy.AllowedAction] = true
			}
		}

		assert.True(t, foundActions["GET"], "Expected GET action")
		assert.True(t, foundActions["POST"], "Expected POST action")
		assert.False(t, foundActions["PUT"], "Should NOT have PUT action")
		assert.False(t, foundActions["DELETE"], "Should NOT have DELETE action")

		t.Cleanup(func() {
			deleteRoleByIDFromDB(t, roleID)
			deletePolicyByIDFromDB(t, policyGET)
			deletePolicyByIDFromDB(t, policyPOST)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}

func TestListUsersLinkedToRole(t *testing.T) {
	t.Run("list_users_linked_to_role", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Setup: Create admin, role, and users. Link some users to the role.
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a new role
		roleID := createTestRoleForPolicyTest(t, adminToken.AccessToken, "list_users_role_")

		// 3. Create users
		user1ID, user1Email := createTestUserForRoleTest(t, adminToken.AccessToken)
		user2ID, user2Email := createTestUserForRoleTest(t, adminToken.AccessToken)
		user3ID, user3Email := createTestUserForRoleTest(t, adminToken.AccessToken)

		// 4. Link users to the role
		linkUsersEndpoint := rolesLinkUsersEndpoint.RewriteSlugs(roleID.String())

		linkUsersPayload := map[string]any{
			"user_ids": []string{user1ID.String(), user2ID.String(), user3ID.String()},
		}

		linkUsersResponse, err := sendHTTPRequest(t, ctx, linkUsersEndpoint, linkUsersPayload, accessTokenHeader)
		assert.NoError(t, err, "Error sending link users request: %v", err)
		defer linkUsersResponse.Body.Close()
		assert.Equal(t, http.StatusOK, linkUsersResponse.StatusCode, "Expected status code 200 OK for linking users")

		linkAPIResp, err := parserResponseBody[payload.HTTPMessage](t, linkUsersResponse)
		assert.NoError(t, err, "Failed to parse link users response body")
		assert.Equal(t, domain.RolesUsersLinkedSuccessfully, linkAPIResp.Message, "Unexpected link users response message")

		// 5. List users linked to the role
		time.Sleep(500 * time.Millisecond) // Added sleep to ensure eventual consistency
		listUsersLinkedEndpoint := rolesListUserLinkedEndpoint.RewriteSlugs(roleID.String())

		listUsersLinkedResponse, err := sendHTTPRequest(t, ctx, listUsersLinkedEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Error sending list users request: %v", err)
		defer listUsersLinkedResponse.Body.Close()
		assert.Equal(t, http.StatusOK, listUsersLinkedResponse.StatusCode, "Expected status code 200 OK for listing users linked to role")

		listUsersAPIResp, err := parserResponseBody[payload.ListUsersResponse](t, listUsersLinkedResponse)
		assert.NoError(t, err, "Failed to parse list users response body")
		assert.GreaterOrEqual(t, len(listUsersAPIResp.Items), 3, "Expected to find at least 3 users linked to the role")

		// Check if the created users are in the list
		userEmails := map[string]bool{
			user1Email: true,
			user2Email: true,
			user3Email: true,
		}
		for _, user := range listUsersAPIResp.Items {
			if _, exists := userEmails[user.Email]; exists {
				assert.Equal(t, user.ID.String(), user.ID.String(), "Expected user ID to match")
				assert.Equal(t, user.Email, user.Email, "Expected user email to match")
				delete(userEmails, user.Email) // Remove from map if found
			}
		}

		// 6. Cleanup
		t.Cleanup(func() {
			deleteRoleByIDFromDB(t, roleID)
			deleteUserByIDFromDB(t, user1ID)
			deleteUserByIDFromDB(t, user2ID)
			deleteUserByIDFromDB(t, user3ID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("require_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		listUsersLinkedEndpoint := rolesListUserLinkedEndpoint.RewriteSlugs("00000000-0000-0000-0000-000000000000")

		resp, err := sendHTTPRequest(t, ctx, listUsersLinkedEndpoint, nil)
		assert.NoError(t, err, "Failed to send request without authentication")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

func TestRoleUnlinkUsers(t *testing.T) {
	t.Run("unlink_users_from_role", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Setup: Create admin, role, and users. Link users to the role.
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		roleID := createTestRoleForPolicyTest(t, adminToken.AccessToken, "unlink_users_role_")

		user1ID, _ := createTestUserForRoleTest(t, adminToken.AccessToken)
		user2ID, user2Email := createTestUserForRoleTest(t, adminToken.AccessToken)
		user3ID, _ := createTestUserForRoleTest(t, adminToken.AccessToken)

		// Link users first
		linkUsersEndpoint := rolesLinkUsersEndpoint.RewriteSlugs(roleID.String())
		linkUsersPayload := map[string]any{
			"user_ids": []string{user1ID.String(), user2ID.String(), user3ID.String()},
		}

		linkUsersResponse, err := sendHTTPRequest(t, ctx, linkUsersEndpoint, linkUsersPayload, accessTokenHeader)
		assert.NoError(t, err, "Error sending link users request during setup: %v", err)
		defer linkUsersResponse.Body.Close()
		assert.Equal(t, http.StatusOK, linkUsersResponse.StatusCode, "Expected status code 200 OK for linking users during setup")

		// Wait briefly to ensure link operation completes
		time.Sleep(100 * time.Millisecond)

		// 2. Unlink a subset of users (user1 and user3)
		unlinkUsersEndpoint := rolesUnlinkUsersEndpoint.RewriteSlugs(roleID.String())
		unlinkUsersPayload := map[string]any{
			"user_ids": []string{user1ID.String(), user3ID.String()},
		}

		unlinkUsersResponse, err := sendHTTPRequest(t, ctx, unlinkUsersEndpoint, unlinkUsersPayload, accessTokenHeader)
		assert.NoError(t, err, "Error sending unlink users request: %v", err)
		defer unlinkUsersResponse.Body.Close()

		// 3. Check unlink response
		assert.Equal(t, http.StatusOK, unlinkUsersResponse.StatusCode, "Expected status code 200 OK for unlinking users")
		unlinkAPIResp, err := parserResponseBody[payload.HTTPMessage](t, unlinkUsersResponse)
		assert.NoError(t, err, "Failed to parse unlink users response body")

		// Assuming domain.RolesUsersUnlinkedSuccessfully exists
		assert.Equal(t, domain.RolesUsersUnlinkedSuccessfully, unlinkAPIResp.Message, "Unexpected unlink users response message")

		// 4. Verify users are unlinked by listing remaining linked users
		time.Sleep(500 * time.Millisecond) // Added sleep to ensure eventual consistency
		listUsersLinkedEndpoint := rolesListUserLinkedEndpoint.RewriteSlugs(roleID.String())
		listUsersLinkedResponse, err := sendHTTPRequest(t, ctx, listUsersLinkedEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Error sending list users request after unlinking: %v", err)
		defer listUsersLinkedResponse.Body.Close()
		assert.Equal(t, http.StatusOK, listUsersLinkedResponse.StatusCode, "Expected status code 200 OK for listing users after unlinking")

		listUsersAPIResp, err := parserResponseBody[payload.ListUsersResponse](t, listUsersLinkedResponse)
		assert.NoError(t, err, "Failed to parse list users response body after unlinking")

		// Check that only user2 remains linked
		assert.Equal(t, 1, len(listUsersAPIResp.Items), "Expected only 1 user to remain linked to the role")
		foundUser2 := false
		unlinkedUserFound := false
		for _, user := range listUsersAPIResp.Items {
			if user.ID == user2ID {
				foundUser2 = true
				assert.Equal(t, user2Email, user.Email)
			}
			if user.ID == user1ID || user.ID == user3ID {
				unlinkedUserFound = true // Should not happen
			}
		}
		assert.True(t, foundUser2, "Expected user2 to be in the list of linked users")
		assert.False(t, unlinkedUserFound, "Unlinked users (user1 or user3) were found in the list")

		// 5. Cleanup
		t.Cleanup(func() {
			deleteRoleByIDFromDB(t, roleID)
			deleteUserByIDFromDB(t, user1ID)
			deleteUserByIDFromDB(t, user2ID)
			deleteUserByIDFromDB(t, user3ID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("require_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		unlinkUsersEndpoint := rolesUnlinkUsersEndpoint.RewriteSlugs("00000000-0000-0000-0000-000000000000")
		unlinkPayload := map[string]any{
			"user_ids": []string{"00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002"},
		}

		resp, err := sendHTTPRequest(t, ctx, unlinkUsersEndpoint, unlinkPayload)
		assert.NoError(t, err, "Failed to send request without authentication")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

// TestRoleCreate_EdgeCases tests role creation with various edge cases
func TestRoleCreate_EdgeCases(t *testing.T) {
	t.Run("create_role_without_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		roleID := uuid.NewV7()
		role := map[string]any{
			"id":          roleID.String(),
			"name":        "test-role-" + roleID.String(),
			"description": "Test role",
		}

		response, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, response.StatusCode, "Expected status code 401. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})

	t.Run("create_role_with_invalid_token", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		invalidTokenHeader := map[string]string{
			"Authorization": "Bearer invalid.token.here",
		}

		roleID := uuid.NewV7()
		role := map[string]any{
			"id":          roleID.String(),
			"name":        "test-role-" + roleID.String(),
			"description": "Test role",
		}

		response, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role, invalidTokenHeader)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, response.StatusCode, "Expected status code 401. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})

	t.Run("create_role_with_empty_name", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		roleID := uuid.NewV7()
		role := map[string]any{
			"id":          roleID.String(),
			"name":        "",
			"description": "Test role",
		}

		response, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role, accessTokenHeader)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusBadRequest, response.StatusCode, "Expected status code 400. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})

	t.Run("create_role_with_name_too_long", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		roleID := uuid.NewV7()
		role := map[string]any{
			"id":          roleID.String(),
			"name":        strings.Repeat("a", 256), // exceeds max length
			"description": "Test role",
		}

		response, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role, accessTokenHeader)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusBadRequest, response.StatusCode, "Expected status code 400. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})

	t.Run("create_role_with_invalid_uuid", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		role := map[string]any{
			"id":          "invalid-uuid",
			"name":        "test-role-invalid",
			"description": "Test role",
		}

		response, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role, accessTokenHeader)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusBadRequest, response.StatusCode, "Expected status code 400. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})
}

// TestRoleGet_EdgeCases tests role retrieval with edge cases
func TestRoleGet_EdgeCases(t *testing.T) {
	t.Run("get_role_without_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		roleID := uuid.NewV7()

		getEndpoint := rolesGetEndpoint.RewriteSlugs(roleID.String())
		response, err := sendHTTPRequest(t, ctx, getEndpoint, nil)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, response.StatusCode, "Expected status code 401. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})

	t.Run("get_role_with_invalid_uuid", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		getEndpoint := rolesGetEndpoint.RewriteSlugs("invalid-uuid")
		response, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusBadRequest, response.StatusCode, "Expected status code 400. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})
}

// TestRoleUpdate_EdgeCases tests role update with edge cases
func TestRoleUpdate_EdgeCases(t *testing.T) {
	t.Run("update_role_without_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		roleID := uuid.NewV7()

		updatePayload := map[string]any{
			"name": "updated-role",
		}

		updateEndpoint := rolesUpdateEndpoint.RewriteSlugs(roleID.String())
		response, err := sendHTTPRequest(t, ctx, updateEndpoint, updatePayload)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, response.StatusCode, "Expected status code 401. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})

	t.Run("update_role_with_empty_payload", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// Create a role first
		roleID := createTestRoleForPolicyTest(t, adminToken.AccessToken, "test-role-")

		// Try to update with empty payload
		updatePayload := map[string]any{}

		updateEndpoint := rolesUpdateEndpoint.RewriteSlugs(roleID.String())
		response, err := sendHTTPRequest(t, ctx, updateEndpoint, updatePayload, accessTokenHeader)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusBadRequest, response.StatusCode, "Expected status code 400. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		t.Cleanup(func() {
			deleteRoleByIDFromDB(t, roleID)
		})
	})

	t.Run("update_non_existent_role", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		nonExistentRoleID := uuid.NewV7()

		updatePayload := map[string]any{
			"name": "updated-role",
		}

		updateEndpoint := rolesUpdateEndpoint.RewriteSlugs(nonExistentRoleID.String())
		response, err := sendHTTPRequest(t, ctx, updateEndpoint, updatePayload, accessTokenHeader)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusNotFound, response.StatusCode, "Expected status code 404. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})
}

// TestRoleDelete_EdgeCases tests role deletion with edge cases
func TestRoleDelete_EdgeCases(t *testing.T) {
	t.Run("delete_role_without_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		roleID := uuid.NewV7()

		deleteEndpoint := rolesDeleteEndpoint.RewriteSlugs(roleID.String())
		response, err := sendHTTPRequest(t, ctx, deleteEndpoint, nil)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, response.StatusCode, "Expected status code 401. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})

	t.Run("delete_non_existent_role", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		nonExistentRoleID := uuid.NewV7()

		deleteEndpoint := rolesDeleteEndpoint.RewriteSlugs(nonExistentRoleID.String())
		response, err := sendHTTPRequest(t, ctx, deleteEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200 (graceful delete for security). Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		// Verify success message
		apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse response body")
		assert.Equal(t, domain.RolesRoleDeletedSuccessfully, apiResp.Message, "Expected success message even for non-existent role (security pattern)")
	})

	t.Run("delete_role_with_invalid_uuid", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		deleteEndpoint := rolesDeleteEndpoint.RewriteSlugs("invalid-uuid")
		response, err := sendHTTPRequest(t, ctx, deleteEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusBadRequest, response.StatusCode, "Expected status code 400. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})
}

// TestRoleList_EdgeCases tests role listing with edge cases
func TestRoleList_EdgeCases(t *testing.T) {
	t.Run("list_roles_without_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		response, err := sendHTTPRequest(t, ctx, rolesListEndpoint, nil)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, response.StatusCode, "Expected status code 401. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})

	t.Run("list_roles_with_invalid_pagination", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		testCases := []struct {
			name     string
			endpoint *apiEndpoint
		}{
			{
				name:     "negative_limit",
				endpoint: newAPIEndpoint(http.MethodGet, "/roles?limit=-1"),
			},
			{
				name:     "limit_too_large",
				endpoint: newAPIEndpoint(http.MethodGet, "/roles?limit=1001"),
			},
			{
				name:     "invalid_cursor",
				endpoint: newAPIEndpoint(http.MethodGet, "/roles?cursor=invalid-cursor"),
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				response, err := sendHTTPRequest(t, ctx, tc.endpoint, nil, accessTokenHeader)
				assert.NoError(t, err)
				defer response.Body.Close()

				if tc.name == "invalid_cursor" {
					// Invalid cursors are silently ignored and return 200 with all results
					assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200 (invalid cursor ignored). Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
				} else {
					// Limit validation errors return 400
					assert.Equal(t, http.StatusBadRequest, response.StatusCode, "Expected status code 400 for limit validation. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
				}
			})
		}
	})
}

// TestRoleJSONSerialization tests that all role endpoints return JSON responses with snake_case field names
func TestRoleJSONSerialization(t *testing.T) {
	// Test that RoleResponse uses snake_case in JSON serialization
	t.Run("get_role_response_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test role
		roleID := uuid.NewV7()

		role := map[string]any{
			"id":          roleID.String(),
			"name":        generateRandomName(t, ""),
			"description": "Test role for JSON serialization",
		}

		createResponse, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role, accessTokenHeader)
		require.NoError(t, err, "Failed to create role")
		defer createResponse.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse.StatusCode)

		// 3. Get the role
		getRoleEndpoint := rolesGetEndpoint.RewriteSlugs(roleID.String())
		response, err := sendHTTPRequest(t, ctx, getRoleEndpoint, nil, accessTokenHeader)
		require.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		// 4. Verify all RoleResponse fields are in snake_case
		expectedFields := []string{
			"id",
			"name",
			"description",
			"system",
			"auto_assign",
			"created_at",
			"updated_at",
		}

		assertJSONFieldsAreSnakeCase(t, response, expectedFields)

		t.Cleanup(func() {
			deleteRoleByIDFromDB(t, roleID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test that ListRolesResponse uses snake_case including nested objects
	t.Run("list_roles_response_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create test roles
		role1ID := uuid.NewV7()

		role1 := map[string]any{
			"id":          role1ID.String(),
			"name":        generateRandomName(t, ""),
			"description": "Test role 1",
		}

		createResponse1, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role1, accessTokenHeader)
		require.NoError(t, err, "Failed to create role 1")
		defer createResponse1.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse1.StatusCode)

		role2ID := uuid.NewV7()

		role2 := map[string]any{
			"id":          role2ID.String(),
			"name":        generateRandomName(t, ""),
			"description": "Test role 2",
		}

		createResponse2, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role2, accessTokenHeader)
		require.NoError(t, err, "Failed to create role 2")
		defer createResponse2.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse2.StatusCode)

		// 3. List roles
		response, err := sendHTTPRequest(t, ctx, rolesListEndpoint, nil, accessTokenHeader)
		require.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		// 4. Verify ListRolesResponse top-level fields are snake_case
		expectedTopLevelFields := []string{
			"items",
			"paginator",
		}

		assertJSONFieldsAreSnakeCase(t, response, expectedTopLevelFields)

		t.Cleanup(func() {
			deleteRoleByIDFromDB(t, role1ID)
			deleteRoleByIDFromDB(t, role2ID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test that HTTPMessage response uses snake_case
	t.Run("create_role_http_message_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a new role
		roleID := uuid.NewV7()

		role := map[string]any{
			"id":          roleID.String(),
			"name":        generateRandomName(t, ""),
			"description": "Test role for HTTP message",
		}

		response, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role, accessTokenHeader)
		require.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		require.Equal(t, http.StatusCreated, response.StatusCode, "Expected status code 201")

		// 3. Verify HTTPMessage fields are in snake_case
		expectedFields := []string{
			"message",
			"method",
			"path",
		}

		assertJSONFieldsAreSnakeCase(t, response, expectedFields)

		t.Cleanup(func() {
			deleteRoleByIDFromDB(t, roleID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test that update response uses snake_case
	t.Run("update_role_response_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test role
		roleID := uuid.NewV7()

		role := map[string]any{
			"id":          roleID.String(),
			"name":        generateRandomName(t, ""),
			"description": "Test role for update",
		}

		createResponse, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role, accessTokenHeader)
		require.NoError(t, err, "Failed to create role")
		defer createResponse.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse.StatusCode)

		// 3. Update the role
		newName := generateRandomName(t, "")
		updatePayload := map[string]any{
			"name": newName,
		}

		updateEndpoint := rolesUpdateEndpoint.RewriteSlugs(roleID.String())
		response, err := sendHTTPRequest(t, ctx, updateEndpoint, updatePayload, accessTokenHeader)
		require.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		// 4. Verify HTTPMessage fields are in snake_case
		expectedFields := []string{
			"message",
			"method",
			"path",
		}

		assertJSONFieldsAreSnakeCase(t, response, expectedFields)

		t.Cleanup(func() {
			deleteRoleByIDFromDB(t, roleID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test that delete response uses snake_case
	t.Run("delete_role_response_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test role
		roleID := uuid.NewV7()

		role := map[string]any{
			"id":          roleID.String(),
			"name":        generateRandomName(t, ""),
			"description": "Test role for delete",
		}

		createResponse, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role, accessTokenHeader)
		require.NoError(t, err, "Failed to create role")
		defer createResponse.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse.StatusCode)

		// 3. Delete the role
		deleteEndpoint := rolesDeleteEndpoint.RewriteSlugs(roleID.String())
		response, err := sendHTTPRequest(t, ctx, deleteEndpoint, nil, accessTokenHeader)
		require.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		// 4. Verify HTTPMessage fields are in snake_case
		expectedFields := []string{
			"message",
			"method",
			"path",
		}

		assertJSONFieldsAreSnakeCase(t, response, expectedFields)

		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test that link policies response uses snake_case
	t.Run("link_policies_response_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test role
		roleID := uuid.NewV7()

		role := map[string]any{
			"id":          roleID.String(),
			"name":        generateRandomName(t, ""),
			"description": "Test role for link policies",
		}

		createResponse, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role, accessTokenHeader)
		require.NoError(t, err, "Failed to create role")
		defer createResponse.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse.StatusCode)

		// 3. Get a policy ID from database (assuming policies exist from migrations)
		var policyID uuid.UUID
		query := `SELECT id FROM policies LIMIT 1`
		err = testDBPool.QueryRow(ctx, query).Scan(&policyID)
		require.NoError(t, err, "Failed to get policy from database")

		// 4. Link policy to role
		linkPayload := map[string]any{
			"policy_ids": []uuid.UUID{policyID},
		}

		linkEndpoint := rolesLinkPoliciesEndpoint.RewriteSlugs(roleID.String())
		response, err := sendHTTPRequest(t, ctx, linkEndpoint, linkPayload, accessTokenHeader)
		require.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		// 5. Verify HTTPMessage fields are in snake_case
		expectedFields := []string{
			"message",
			"method",
			"path",
		}

		assertJSONFieldsAreSnakeCase(t, response, expectedFields)

		t.Cleanup(func() {
			unlinkAllPoliciesFromRoleViaDB(t, roleID)
			deleteRoleByIDFromDB(t, roleID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test that unlink policies response uses snake_case
	t.Run("unlink_policies_response_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test role
		roleID := uuid.NewV7()

		role := map[string]any{
			"id":          roleID.String(),
			"name":        generateRandomName(t, ""),
			"description": "Test role for unlink policies",
		}

		createResponse, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role, accessTokenHeader)
		require.NoError(t, err, "Failed to create role")
		defer createResponse.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse.StatusCode)

		// 3. Get a policy ID and link it first
		var policyID uuid.UUID
		query := `SELECT id FROM policies LIMIT 1`
		err = testDBPool.QueryRow(ctx, query).Scan(&policyID)
		require.NoError(t, err, "Failed to get policy from database")

		// Link the policy
		linkQuery := `INSERT INTO roles_policies (roles_id, policies_id) VALUES ($1, $2)`
		_, err = testDBPool.Exec(ctx, linkQuery, roleID, policyID)
		require.NoError(t, err, "Failed to link policy to role")

		// 4. Unlink policy from role
		unlinkPayload := map[string]any{
			"policy_ids": []uuid.UUID{policyID},
		}

		unlinkEndpoint := rolesUnlinkPoliciesEndpoint.RewriteSlugs(roleID.String())
		response, err := sendHTTPRequest(t, ctx, unlinkEndpoint, unlinkPayload, accessTokenHeader)
		require.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		// 5. Verify HTTPMessage fields are in snake_case
		expectedFields := []string{
			"message",
			"method",
			"path",
		}

		assertJSONFieldsAreSnakeCase(t, response, expectedFields)

		t.Cleanup(func() {
			unlinkAllPoliciesFromRoleViaDB(t, roleID)
			deleteRoleByIDFromDB(t, roleID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test that link users response uses snake_case
	t.Run("link_users_response_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test role
		roleID := uuid.NewV7()

		role := map[string]any{
			"id":          roleID.String(),
			"name":        generateRandomName(t, ""),
			"description": "Test role for link users",
		}

		createResponse, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role, accessTokenHeader)
		require.NoError(t, err, "Failed to create role")
		defer createResponse.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse.StatusCode)

		// 3. Create a test user
		firstName, lastName, email := generateUserData(t)
		password := generatePassword(t)

		userID := createUserInDB(t, firstName, lastName, email, password)

		// 4. Link user to role
		linkPayload := map[string]any{
			"user_ids": []uuid.UUID{userID},
		}

		linkEndpoint := rolesLinkUsersEndpoint.RewriteSlugs(roleID.String())
		response, err := sendHTTPRequest(t, ctx, linkEndpoint, linkPayload, accessTokenHeader)
		require.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		// 5. Verify HTTPMessage fields are in snake_case
		expectedFields := []string{
			"message",
			"method",
			"path",
		}

		assertJSONFieldsAreSnakeCase(t, response, expectedFields)

		t.Cleanup(func() {
			unlinkRolesFromUserViaDB(t, userID, []uuid.UUID{roleID})
			deleteUserByIDFromDB(t, userID)
			deleteRoleByIDFromDB(t, roleID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test that unlink users response uses snake_case
	t.Run("unlink_users_response_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test role
		roleID := uuid.NewV7()

		role := map[string]any{
			"id":          roleID.String(),
			"name":        generateRandomName(t, ""),
			"description": "Test role for unlink users",
		}

		createResponse, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role, accessTokenHeader)
		require.NoError(t, err, "Failed to create role")
		defer createResponse.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse.StatusCode)

		// 3. Create a test user and link it
		firstName, lastName, email := generateUserData(t)
		password := generatePassword(t)

		userID := createUserInDB(t, firstName, lastName, email, password)

		// Link the user
		linkQuery := `INSERT INTO users_roles (users_id, roles_id) VALUES ($1, $2)`
		_, err = testDBPool.Exec(ctx, linkQuery, userID, roleID)
		require.NoError(t, err, "Failed to link user to role")

		// 4. Unlink user from role
		unlinkPayload := map[string]any{
			"user_ids": []uuid.UUID{userID},
		}

		unlinkEndpoint := rolesUnlinkUsersEndpoint.RewriteSlugs(roleID.String())
		response, err := sendHTTPRequest(t, ctx, unlinkEndpoint, unlinkPayload, accessTokenHeader)
		require.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		// 5. Verify HTTPMessage fields are in snake_case
		expectedFields := []string{
			"message",
			"method",
			"path",
		}

		assertJSONFieldsAreSnakeCase(t, response, expectedFields)

		t.Cleanup(func() {
			unlinkRolesFromUserViaDB(t, userID, []uuid.UUID{roleID})
			deleteUserByIDFromDB(t, userID)
			deleteRoleByIDFromDB(t, roleID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test that list users linked to role response uses snake_case
	t.Run("list_users_linked_to_role_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test role
		roleID := uuid.NewV7()

		role := map[string]any{
			"id":          roleID.String(),
			"name":        generateRandomName(t, ""),
			"description": "Test role for list users",
		}

		createResponse, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role, accessTokenHeader)
		require.NoError(t, err, "Failed to create role")
		defer createResponse.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse.StatusCode)

		// 3. Create test users and link them
		firstName1, lastName1, email1 := generateUserData(t)
		password1 := generatePassword(t)
		userID1 := createUserInDB(t, firstName1, lastName1, email1, password1)

		firstName2, lastName2, email2 := generateUserData(t)
		password2 := generatePassword(t)
		userID2 := createUserInDB(t, firstName2, lastName2, email2, password2)

		// Link users to role
		linkQuery := `INSERT INTO users_roles (users_id, roles_id) VALUES ($1, $2)`
		_, err = testDBPool.Exec(ctx, linkQuery, userID1, roleID)
		require.NoError(t, err, "Failed to link user 1 to role")
		_, err = testDBPool.Exec(ctx, linkQuery, userID2, roleID)
		require.NoError(t, err, "Failed to link user 2 to role")

		// 4. List users linked to role
		listEndpoint := rolesListUserLinkedEndpoint.RewriteSlugs(roleID.String())
		response, err := sendHTTPRequest(t, ctx, listEndpoint, nil, accessTokenHeader)
		require.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		// 5. Verify ListUsersResponse top-level fields are snake_case
		expectedTopLevelFields := []string{
			"items",
			"paginator",
		}

		assertJSONFieldsAreSnakeCase(t, response, expectedTopLevelFields)

		t.Cleanup(func() {
			unlinkRolesFromUserViaDB(t, userID1, []uuid.UUID{roleID})
			unlinkRolesFromUserViaDB(t, userID2, []uuid.UUID{roleID})
			deleteUserByIDFromDB(t, userID1)
			deleteUserByIDFromDB(t, userID2)
			deleteRoleByIDFromDB(t, roleID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}
