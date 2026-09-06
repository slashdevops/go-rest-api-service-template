//go:build integration

package integration

import (
	"math/rand"
	"net/http"
	"strings"
	"testing"
	"time"

	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

var (
	policyCreateEndpoint      = newAPIEndpoint(http.MethodPost, "/policies")
	policyGetEndpoint         = newAPIEndpoint(http.MethodGet, "/policies/{policy_id}")
	policyListEndpoint        = newAPIEndpoint(http.MethodGet, "/policies")
	policyUpdateEndpoint      = newAPIEndpoint(http.MethodPut, "/policies/{policy_id}")
	policyDeleteEndpoint      = newAPIEndpoint(http.MethodDelete, "/policies/{policy_id}")
	policyLinkRolesEndpoint   = newAPIEndpoint(http.MethodPost, "/policies/{policy_id}/roles")
	policyUnlinkRolesEndpoint = newAPIEndpoint(http.MethodDelete, "/policies/{policy_id}/roles")

	policyListPoliciesByRoleEndpoint = newAPIEndpoint(http.MethodGet, "/roles/{role_id}/policies")
)

// Helper function to create a policy for testing
func createTestPolicy(t *testing.T, accessToken, namePrefix string) uuid.UUID {
	t.Helper()

	policyID := uuid.NewV7()
	policy := map[string]any{
		"id":               policyID.String(),
		"name":             namePrefix + policyID.String(),
		"description":      "Test policy " + policyID.String(),
		"allowed_action":   "GET",
		"allowed_resource": "/users/" + policyID.String(),
	}

	accessTokenHeader := map[string]string{"Authorization": "Bearer " + accessToken}

	ctx := t.Context()
	response, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, policy, accessTokenHeader)
	assert.NoError(t, err, "Failed to send request to create policy")
	defer response.Body.Close()

	assert.Equal(t, http.StatusCreated, response.StatusCode, "Expected status code 201 for policy creation. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

	// Verify the response
	createAPIResp, err := parserResponseBody[payload.HTTPMessage](t, response)
	assert.NoError(t, err, "Failed to parse response body")
	assert.Equal(t, domain.PoliciesPolicyCreatedSuccessfully, createAPIResp.Message, "Expected success message")

	return policyID
}

// Helper function to create a policy with custom parameters
func createTestPolicyWithParams(t *testing.T, accessToken, namePrefix, resource, action, effect string) uuid.UUID {
	t.Helper()

	policyID := uuid.NewV7()
	policy := map[string]any{
		"id":               policyID.String(),
		"name":             namePrefix + policyID.String(),
		"description":      "Test policy " + policyID.String(),
		"allowed_action":   action,
		"allowed_resource": resource,
	}

	accessTokenHeader := map[string]string{"Authorization": "Bearer " + accessToken}

	ctx := t.Context()
	response, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, policy, accessTokenHeader)
	assert.NoError(t, err, "Failed to send request to create policy")
	defer response.Body.Close()

	assert.Equal(t, http.StatusCreated, response.StatusCode, "Expected status code 201 for policy creation. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

	// Verify the response
	createAPIResp, err := parserResponseBody[payload.HTTPMessage](t, response)
	assert.NoError(t, err, "Failed to parse response body")
	assert.Equal(t, domain.PoliciesPolicyCreatedSuccessfully, createAPIResp.Message, "Expected success message")

	return policyID
}

// Helper function to create a role for testing
func createTestRole(t *testing.T, accessToken, namePrefix string) uuid.UUID {
	t.Helper()

	roleID := uuid.NewV7()
	role := map[string]any{
		"id":          roleID.String(),
		"name":        namePrefix + roleID.String(),
		"description": "Test role " + roleID.String(),
	}

	accessTokenHeader := map[string]string{"Authorization": "Bearer " + accessToken}

	ctx := t.Context()

	rolesCreateEndpoint := newAPIEndpoint(http.MethodPost, "/roles")
	response, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role, accessTokenHeader)
	assert.NoError(t, err, "Failed to create test role")
	defer response.Body.Close()

	assert.Equal(t, http.StatusCreated, response.StatusCode, "Failed to create test role, status code not 201. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	return roleID
}

func TestPolicyCreate(t *testing.T) {
	// Test policy creation
	t.Run("create_policy", func(t *testing.T) {
		t.Parallel()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		// 2. Create a new policy
		policyID := uuid.NewV7()

		policy := map[string]any{
			"id":               policyID.String(),
			"name":             "test_policy_" + policyID.String(),
			"description":      "This is a test policy " + policyID.String(),
			"allowed_action":   "GET",
			"allowed_resource": "/users/" + policyID.String(),
		}

		// 2.1 Use access token from admin to have access to the endpoint
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		ctx := t.Context()
		response, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, policy, accessTokenHeader)
		assert.NoError(t, err, "Error sending request: %v", err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusCreated, response.StatusCode, "Expected status code 201 Created. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		t.Cleanup(func() {
			deletePolicyByIDFromDB(t, policyID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})

		assert.Equal(t, domain.PoliciesPolicyCreatedSuccessfully, apiResp.Message, "Unexpected response message")
		assert.Equal(t, policyCreateEndpoint.method, apiResp.Method, "Expected method to be set")
		assert.Equal(t, policyCreateEndpoint.Path(), apiResp.Path, "Expected path to be set")
	})

	// Test creating a policy with invalid data format
	t.Run("create_policy_bad_request", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Define test cases for various invalid inputs
		testCases := []struct {
			name          string
			invalidPolicy map[string]any
			expectedError string
		}{
			{
				name: "Invalid ID format",
				invalidPolicy: map[string]any{
					"id":               "not-a-valid-uuid",
					"name":             "Test Policy",
					"description":      "Test policy description",
					"allowed_action":   "GET",
					"allowed_resource": "/users/*",
				},
				expectedError: "invalid uuid",
			},
			{
				name: "Empty name",
				invalidPolicy: map[string]any{
					"id":               uuid.NewV7().String(),
					"name":             "",
					"description":      "Test policy description",
					"allowed_action":   "GET",
					"allowed_resource": "/users/*",
				},
				expectedError: "cannot be empty",
			},
			{
				name: "Invalid action",
				invalidPolicy: map[string]any{
					"id":               uuid.NewV7().String(),
					"name":             "Test Policy",
					"description":      "Test policy description",
					"allowed_action":   "INVALID_ACTION",
					"allowed_resource": "/users/*",
				},
				expectedError: "invalid action",
			},
			{
				name: "Empty resource",
				invalidPolicy: map[string]any{
					"id":               uuid.NewV7().String(),
					"name":             "Test Policy",
					"description":      "Test policy description",
					"allowed_action":   "GET",
					"allowed_resource": "",
				},
				expectedError: "cannot be empty",
			},
		}

		// 3. Run each test case
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Send request with the invalid policy data
				response, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, tc.invalidPolicy, accessTokenHeader)
				assert.NoError(t, err, "Failed to send request for %s", tc.name)
				defer response.Body.Close()

				// Verify we get a 400 Bad Request response
				assert.Equal(t, http.StatusBadRequest, response.StatusCode,
					"Expected status code 400 Bad Request for %s. Got %d. Message: %s", tc.name, response.StatusCode, readResponseBody(t, response))

				// Parse and verify the error response
				errorResp, err := parserResponseBody[payload.HTTPMessage](t, response)
				assert.NoError(t, err, "Failed to parse error response for %s", tc.name)

				// Verify error message contains information specific to this validation failure
				assert.Contains(t, strings.ToLower(errorResp.Message), strings.ToLower(tc.expectedError),
					"Error message should indicate %s validation failure", tc.name)
				assert.Equal(t, policyCreateEndpoint.method, errorResp.Method, "Expected method to be set")
				assert.Equal(t, policyCreateEndpoint.Path(), errorResp.Path, "Expected path to be set")
			})
		}

		// 4. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test creating policies with existing ID or name
	t.Run("create_policy_already_exists", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. First create a valid policy that will be our reference policy
		policyID := uuid.NewV7()
		policyName := "test_" + policyID.String()
		allowedAction := "GET"
		allowedResource := "/users/*"

		existingPolicy := map[string]any{
			"id":               policyID.String(),
			"name":             policyName,
			"description":      "This is a test policy for duplicate checks",
			"allowed_action":   allowedAction,
			"allowed_resource": allowedResource,
		}

		// Create the first policy
		createResponse, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, existingPolicy, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to create initial policy")
		defer createResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, createResponse.StatusCode, "Expected status code 201 for initial policy creation. Got %d. Message: %s", createResponse.StatusCode, readResponseBody(t, createResponse))

		// 3. Define test cases for duplicate scenarios
		testCases := []struct {
			name            string
			duplicatePolicy map[string]any
			expectedStatus  int
			expectedError   string
		}{
			{
				name: "Policy with existing ID",
				duplicatePolicy: map[string]any{
					"id":               policyID.String(), // Same ID as existing policy
					"name":             "Different_" + policyName,
					"description":      "This is a different policy",
					"allowed_action":   "POST",
					"allowed_resource": "/roles",
				},
				expectedStatus: http.StatusConflict,
				expectedError:  "already exists",
			},
			{
				name: "Policy with existing name",
				duplicatePolicy: map[string]any{
					"id":               uuid.NewV7().String(), // Different ID
					"name":             policyName,            // Same name as existing policy has the same allowed action and allowed resource
					"description":      "This is another policy",
					"allowed_action":   allowedAction,
					"allowed_resource": allowedResource,
				},
				expectedStatus: http.StatusConflict,
				expectedError:  "already exists",
			},
		}

		// 4. Run each test case
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Try to create a policy with duplicate ID or name
				response, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, tc.duplicatePolicy, accessTokenHeader)
				assert.NoError(t, err, "Failed to send request for %s", tc.name)
				defer response.Body.Close()

				// Verify we get the expected conflict status
				assert.Equal(t, tc.expectedStatus, response.StatusCode,
					"Expected status code %d for %s. Got %d. Message: %s", tc.expectedStatus, tc.name, response.StatusCode, readResponseBody(t, response))

				// Parse and verify the error response
				errorResp, err := parserResponseBody[payload.HTTPMessage](t, response)
				assert.NoError(t, err, "Failed to parse error response for %s", tc.name)

				// Verify error message contains information about the conflict
				assert.Contains(t, strings.ToLower(errorResp.Message), strings.ToLower(tc.expectedError),
					"Error message should indicate conflict for %s", tc.name)
				assert.Equal(t, policyCreateEndpoint.method, errorResp.Method, "Expected method to be set")
				assert.Equal(t, policyCreateEndpoint.Path(), errorResp.Path, "Expected path to be set")
			})
		}

		// 5. Cleanup
		t.Cleanup(func() {
			deletePolicyByIDFromDB(t, policyID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test creating policies with various valid resources from the system
	t.Run("create_policies_with_valid_resources", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Define test cases for various valid resources from the migration file
		testCases := []struct {
			name            string
			allowedAction   string
			allowedResource string
			description     string
		}{
			// David error
			{
				name:            "TestDavid",
				allowedAction:   "DELETE",
				allowedResource: "/users/*",
				description:     "Test",
			},
			// Auth endpoints
			{
				name:            "Auth login resource",
				allowedAction:   "POST",
				allowedResource: "/auth/login",
				description:     "Policy for user authentication",
			},
			{
				name:            "Auth logout resource",
				allowedAction:   "DELETE",
				allowedResource: "/auth/logout",
				description:     "Policy for user logout",
			},
			{
				name:            "Auth refresh token resource",
				allowedAction:   "POST",
				allowedResource: "/auth/refresh",
				description:     "Policy for refreshing tokens",
			},
			{
				name:            "Auth register resource",
				allowedAction:   "POST",
				allowedResource: "/auth/register",
				description:     "Policy for user registration",
			},
			{
				name:            "Auth resend verification resource",
				allowedAction:   "POST",
				allowedResource: "/auth/verify",
				description:     "Policy for resending verification email",
			},
			{
				name:            "Auth verify account resource",
				allowedAction:   "POST",
				allowedResource: "/auth/verify/confirm",
				description:     "Policy for verifying user account",
			},
			// Policy endpoints
			{
				name:            "List policies resource",
				allowedAction:   "GET",
				allowedResource: "/policies",
				description:     "Policy for listing policies",
			},
			{
				name:            "Create policy resource",
				allowedAction:   "POST",
				allowedResource: "/policies",
				description:     "Policy for creating policies",
			},
			{
				name:            "Delete policy resource",
				allowedAction:   "DELETE",
				allowedResource: "/policies/019826a1-081e-7b23-a27b-8551fa40c489",
				description:     "Policy for deleting policies",
			},
			{
				name:            "Update policy resource",
				allowedAction:   "PUT",
				allowedResource: "/policies/019826a1-081e-7b27-bc3f-7b03697ec03c",
				description:     "Policy for updating policies",
			},
			{
				name:            "Get specific policy resource",
				allowedAction:   "GET",
				allowedResource: "/policies/019826a1-081e-7b2b-b894-d9d9ac9568c7",
				description:     "Policy for getting specific policy",
			},
			{
				name:            "Unlink roles from policy resource",
				allowedAction:   "DELETE",
				allowedResource: "/policies/019826a1-081e-7b2f-923a-684494b233ef/roles",
				description:     "Policy for unlinking roles from policy",
			},
			{
				name:            "Link roles to policy resource",
				allowedAction:   "POST",
				allowedResource: "/policies/019826a1-081e-7b33-9817-85294d752935/roles",
				description:     "Policy for linking roles to policy",
			},
			{
				name:            "List roles by policy resource",
				allowedAction:   "GET",
				allowedResource: "/policies/019826a1-081e-7b37-a9f5-21ce828c73b5/roles",
				description:     "Policy for listing roles by policy",
			},
			// Project endpoints
			{
				name:            "List projects resource",
				allowedAction:   "GET",
				allowedResource: "/projects",
				description:     "Policy for listing projects",
			},
			{
				name:            "Create project resource",
				allowedAction:   "POST",
				allowedResource: "/projects",
				description:     "Policy for creating projects",
			},
			{
				name:            "Delete project resource",
				allowedAction:   "DELETE",
				allowedResource: "/projects/019826a1-081e-7b3b-9e6e-283fd4acb5db",
				description:     "Policy for deleting projects",
			},
			{
				name:            "Update project resource",
				allowedAction:   "PUT",
				allowedResource: "/projects/019826a1-081e-7b3c-8809-fa8837365151",
				description:     "Policy for updating projects",
			},
			{
				name:            "Get project resource",
				allowedAction:   "GET",
				allowedResource: "/projects/019826a1-081e-7b42-9525-c406b0d7f453",
				description:     "Policy for getting specific project",
			},
			// Product endpoints (the worked example)
			{
				name:            "List products resource",
				allowedAction:   "GET",
				allowedResource: "/products",
				description:     "Policy for listing products across projects",
			},
			{
				name:            "List products by project resource",
				allowedAction:   "GET",
				allowedResource: "/projects/01a07117-c545-7595-8025-6c04d5cba26f/products",
				description:     "Policy for listing the products of a project",
			},
			{
				name:            "Create product resource",
				allowedAction:   "POST",
				allowedResource: "/projects/01a07117-c545-7645-81c9-1c904b9c9c7f/products",
				description:     "Policy for creating products",
			},
			{
				name:            "Get product resource",
				allowedAction:   "GET",
				allowedResource: "/projects/01a07117-c545-7649-bcec-9b7d74125b3c/products/01a07117-c545-764d-891e-cc6092620502",
				description:     "Policy for getting a product",
			},
			{
				name:            "Update product resource",
				allowedAction:   "PUT",
				allowedResource: "/projects/01a07117-c545-765e-8003-cfb29ee8b555/products/01a07117-c545-7662-bb59-7aa249796bce",
				description:     "Policy for updating a product",
			},
			{
				name:            "Delete product resource",
				allowedAction:   "DELETE",
				allowedResource: "/projects/01a07117-c545-7666-9fe0-396f46e72097/products/01a07117-c545-7667-bcf6-34f6003fc54f",
				description:     "Policy for deleting a product",
			},
			// Resource endpoints
			{
				name:            "List resources resource",
				allowedAction:   "GET",
				allowedResource: "/resources",
				description:     "Policy for listing resources",
			},
			{
				name:            "Match resources by action resource",
				allowedAction:   "GET",
				allowedResource: "/resources/matches",
				description:     "Policy for matching resources by action",
			},
			{
				name:            "Get resource resource",
				allowedAction:   "GET",
				allowedResource: "/resources/019826a1-081e-7d52-83c5-2553f5f29019",
				description:     "Policy for getting specific resource",
			},
			// Role endpoints
			{
				name:            "Create role resource",
				allowedAction:   "POST",
				allowedResource: "/roles",
				description:     "Policy for creating roles",
			},
			{
				name:            "List roles resource",
				allowedAction:   "GET",
				allowedResource: "/roles",
				description:     "Policy for listing roles",
			},
			{
				name:            "Delete role resource",
				allowedAction:   "DELETE",
				allowedResource: "/roles/019826a1-081e-7d53-8be4-a5d197cf9b2e",
				description:     "Policy for deleting roles",
			},
			{
				name:            "Update role resource",
				allowedAction:   "PUT",
				allowedResource: "/roles/019826a1-081e-7d59-ac8c-26e40a46c1bf",
				description:     "Policy for updating roles",
			},
			{
				name:            "Get role resource",
				allowedAction:   "GET",
				allowedResource: "/roles/019826a1-081e-7d5d-ad2f-3def5d34e1a6",
				description:     "Policy for getting specific role",
			},
			{
				name:            "Link policies to role resource",
				allowedAction:   "POST",
				allowedResource: "/roles/019826a1-081e-7d61-9835-9fa58d741ced/policies",
				description:     "Policy for linking policies to role",
			},
			{
				name:            "List policies by role resource",
				allowedAction:   "GET",
				allowedResource: "/roles/019826a1-081e-7d62-9647-eaa862428728/policies",
				description:     "Policy for listing policies by role",
			},
			{
				name:            "Unlink policies from role resource",
				allowedAction:   "DELETE",
				allowedResource: "/roles/019826a1-081e-7d65-86fb-39a4e530081d/policies",
				description:     "Policy for unlinking policies from role",
			},
			{
				name:            "Link users to role resource",
				allowedAction:   "POST",
				allowedResource: "/roles/019826a1-081e-7d69-b9d0-0fa407c06957/users",
				description:     "Policy for linking users to role",
			},
			{
				name:            "Unlink users from role resource",
				allowedAction:   "DELETE",
				allowedResource: "/roles/019826a1-081e-7d6d-89be-d5f8a719ba7c/users",
				description:     "Policy for unlinking users from role",
			},
			{
				name:            "List users by role resource",
				allowedAction:   "GET",
				allowedResource: "/roles/019826a1-081e-7d71-8da0-91f821211feb/users",
				description:     "Policy for listing users by role",
			},
			// User endpoints
			{
				name:            "Create user resource",
				allowedAction:   "POST",
				allowedResource: "/users",
				description:     "Policy for creating users",
			},
			{
				name:            "List users resource",
				allowedAction:   "GET",
				allowedResource: "/users",
				description:     "Policy for listing users",
			},
			{
				name:            "Update user resource",
				allowedAction:   "PUT",
				allowedResource: "/users/019826a1-081e-7d75-949f-cddf8ba8624b",
				description:     "Policy for updating users",
			},
			{
				name:            "Delete user resource",
				allowedAction:   "DELETE",
				allowedResource: "/users/019826a1-081e-7d79-b194-b88945191fc0",
				description:     "Policy for deleting users",
			},
			{
				name:            "Get user resource",
				allowedAction:   "GET",
				allowedResource: "/users/019826a1-081e-7d7d-a3b3-464de6369be0",
				description:     "Policy for getting specific user",
			},
			{
				name:            "Get user authorization resource",
				allowedAction:   "GET",
				allowedResource: "/users/019826a1-081e-7d81-ae85-60d1ce3d66f5/authz",
				description:     "Policy for getting user authorization",
			},
			// User role management endpoints
			{
				name:            "Unlink roles from user resource",
				allowedAction:   "DELETE",
				allowedResource: "/users/019826a1-081e-7da4-8072-8422d9b612cd/roles",
				description:     "Policy for unlinking roles from user",
			},
			{
				name:            "Link roles to user resource",
				allowedAction:   "POST",
				allowedResource: "/users/019826a1-081e-7dbf-a971-69186f453351/roles",
				description:     "Policy for linking roles to user",
			},
			{
				name:            "List roles by user resource",
				allowedAction:   "GET",
				allowedResource: "/users/019826a1-081e-7dc3-a7a9-3d1a3d321e13/roles",
				description:     "Policy for listing roles by user",
			},
			// Wildcard permissions for comprehensive access control testing
			{
				name:            "Wildcard all actions resource",
				allowedAction:   "*",
				allowedResource: "*",
				description:     "Policy with full access to all resources",
			},
			{
				name:            "Read-only all resources",
				allowedAction:   "GET",
				allowedResource: "*",
				description:     "Policy for read-only access to all resources",
			},
			{
				name:            "Create-only all resources",
				allowedAction:   "POST",
				allowedResource: "*",
				description:     "Policy for create-only access to all resources",
			},
			{
				name:            "Delete-only all resources",
				allowedAction:   "DELETE",
				allowedResource: "*",
				description:     "Policy for delete-only access to all resources",
			},
			{
				name:            "Update-only all resources",
				allowedAction:   "PUT",
				allowedResource: "*",
				description:     "Policy for update-only access to all resources",
			},
			{
				name:            "Patch-only all resources",
				allowedAction:   "PATCH",
				allowedResource: "*",
				description:     "Policy for patch-only access to all resources",
			},
			{
				name:            "Options-only all resources",
				allowedAction:   "OPTIONS",
				allowedResource: "*",
				description:     "Policy for options-only access to all resources",
			},
			{
				name:            "Head-only all resources",
				allowedAction:   "HEAD",
				allowedResource: "*",
				description:     "Policy for head-only access to all resources",
			},
			// Pattern matching permissions
			{
				name:            "Delete any users resource",
				allowedAction:   "DELETE",
				allowedResource: "/users/*",
				description:     "Policy for deleting any users",
			},
			{
				name:            "Get any projects resource",
				allowedAction:   "GET",
				allowedResource: "/projects/*",
				description:     "Policy for getting any projects",
			},
			{
				name:            "Update any roles resource",
				allowedAction:   "PUT",
				allowedResource: "/roles/*",
				description:     "Policy for updating any roles",
			},
		}

		var createdPolicyIDs []uuid.UUID

		// 3. Run each test case
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Generate unique policy ID and name
				policyID := uuid.NewV7()

				// Add some randomness to avoid naming conflicts
				uniqueSuffix := policyID.String()[:8]
				policyName := "test_resource_policy_" + strings.ReplaceAll(tc.name, " ", "_") + "_" + uniqueSuffix

				policy := map[string]any{
					"id":               policyID.String(),
					"name":             policyName,
					"description":      tc.description + " - " + uniqueSuffix,
					"allowed_action":   tc.allowedAction,
					"allowed_resource": tc.allowedResource,
				}

				// Send request to create the policy
				response, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, policy, accessTokenHeader)
				assert.NoError(t, err, "Failed to send request for %s", tc.name)
				defer response.Body.Close()

				// Verify successful creation
				assert.Equal(t, http.StatusCreated, response.StatusCode,
					"Expected status code 201 Created for %s. Got %d. Message: %s", tc.name, response.StatusCode, readResponseBody(t, response))

				// Parse and verify the success response
				apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
				assert.NoError(t, err, "Failed to parse success response for %s", tc.name)

				assert.Equal(t, domain.PoliciesPolicyCreatedSuccessfully, apiResp.Message, "Expected success message for %s", tc.name)
				assert.Equal(t, policyCreateEndpoint.method, apiResp.Method, "Expected method to be set for %s", tc.name)
				assert.Equal(t, policyCreateEndpoint.Path(), apiResp.Path, "Expected path to be set for %s", tc.name)

				// Store policy ID for cleanup
				createdPolicyIDs = append(createdPolicyIDs, policyID)

				// Verify the policy was actually created by retrieving it
				getEndpoint := policyGetEndpoint.RewriteSlugs(policyID.String())
				getResponse, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
				assert.NoError(t, err, "Failed to retrieve created policy for %s", tc.name)
				defer getResponse.Body.Close()

				assert.Equal(t, http.StatusOK, getResponse.StatusCode,
					"Expected status code 200 OK when retrieving %s policy. Got %d. Message: %s", tc.name, getResponse.StatusCode, readResponseBody(t, getResponse))

				// Parse and verify the retrieved policy
				retrievedPolicy, err := parserResponseBody[payload.PolicyResponse](t, getResponse)
				assert.NoError(t, err, "Failed to parse retrieved policy for %s", tc.name)

				assert.Equal(t, policyID, retrievedPolicy.ID, "Expected policy ID to match for %s", tc.name)
				assert.Equal(t, policyName, retrievedPolicy.Name, "Expected policy name to match for %s", tc.name)
				assert.Equal(t, tc.allowedAction, retrievedPolicy.AllowedAction, "Expected action to match for %s", tc.name)
				assert.Equal(t, tc.allowedResource, retrievedPolicy.AllowedResource, "Expected resource to match for %s", tc.name)
				assert.Contains(t, retrievedPolicy.Description, tc.description, "Expected description to contain base text for %s", tc.name)
			})
		}

		// 4. Cleanup - delete all created policies and the admin user
		t.Cleanup(func() {
			for _, policyID := range createdPolicyIDs {
				deletePolicyByIDFromDB(t, policyID)
			}
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("require_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		policy := map[string]any{
			"id":          "00000000-0000-0000-0000-000000000000",
			"name":        "Test Policy",
			"description": "Test policy description",
		}

		resp, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, policy)
		assert.NoError(t, err, "Failed to send request without authentication")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

func TestPolicyGet(t *testing.T) {
	// Test policy retrieval
	t.Run("get_policy", func(t *testing.T) {
		t.Parallel()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		// 2. Create a new policy
		policyID := uuid.NewV7()
		policyName := "test_policy_get_" + policyID.String()
		policyDesc := "This is a test policy for get " + policyID.String()
		policyAction := "GET"
		policyResource := "/roles/" + policyID.String()

		policy := map[string]any{
			"id":               policyID.String(),
			"name":             policyName,
			"description":      policyDesc,
			"allowed_action":   policyAction,
			"allowed_resource": policyResource,
		}

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		ctx := t.Context()
		createResponse, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, policy, accessTokenHeader)
		assert.NoError(t, err, "Error sending create request: %v", err)
		defer createResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, createResponse.StatusCode, "Expected status code 201 Created for setup. Got %d. Message: %s", createResponse.StatusCode, readResponseBody(t, createResponse))

		// t.Logf("Create response: %v", createResponse)

		// 3. Get the policy
		getEndpoint := policyGetEndpoint.RewriteSlugs(policyID.String())
		// t.Logf("Get endpoint: %s", getEndpoint.Path())

		getResponse, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Error sending get request: %v", err)
		defer getResponse.Body.Close()

		// t.Logf("Get response: %v", getResponse)

		// 4. Check the response
		assert.Equal(t, http.StatusOK, getResponse.StatusCode, "Expected status code 200 OK for get. Got %d. Message: %s", getResponse.StatusCode, readResponseBody(t, getResponse))
		getAPIResp, err := parserResponseBody[payload.PolicyResponse](t, getResponse)
		assert.NoError(t, err, "Failed to parse get response body", err)

		assert.Equal(t, policyID, getAPIResp.ID, "Expected policy ID to match")
		assert.Equal(t, policyName, getAPIResp.Name, "Expected policy name to match")
		assert.Equal(t, policyDesc, getAPIResp.Description, "Expected policy description to match")
		assert.Equal(t, policyAction, getAPIResp.AllowedAction, "Expected policy action to match")
		assert.Equal(t, policyResource, getAPIResp.AllowedResource, "Expected policy resource to match")

		// 5. Cleanup
		t.Cleanup(func() {
			deletePolicyByIDFromDB(t, policyID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test retrieving a non-existent policy
	t.Run("get_policy_not_found", func(t *testing.T) {
		t.Parallel()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Generate a random UUID that doesn't exist in the database
		nonExistentPolicyID := uuid.NewV7()

		// 3. Try to get the non-existent policy
		getEndpoint := policyGetEndpoint.RewriteSlugs(nonExistentPolicyID.String())
		ctx := t.Context()
		getResponse, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to get non-existent policy")
		defer getResponse.Body.Close()

		// 4. Check that we get a 404 Not Found response
		assert.Equal(t, http.StatusNotFound, getResponse.StatusCode, "Expected status code 404 for non-existent policy. Got %d. Message: %s", getResponse.StatusCode, readResponseBody(t, getResponse))

		// 5. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, getResponse)
		assert.NoError(t, err, "Failed to parse error response")

		// Verify error message contains information about the policy not being found
		assert.Contains(t, errorResp.Message, "not found", "Error message should indicate that the policy was not found")
		assert.Equal(t, getEndpoint.method, errorResp.Method, "Expected method to be set")
		assert.Equal(t, getEndpoint.Path(), errorResp.Path, "Expected path to be set")

		// 6. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test retrieving a policy with an invalid ID format
	t.Run("get_policy_bad_request", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Try to get a policy with an invalid ID format (not a UUID)
		invalidPolicyID := "not-a-valid-uuid"
		getEndpoint := policyGetEndpoint.RewriteSlugs(invalidPolicyID)
		getResponse, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to get policy with invalid ID")
		defer getResponse.Body.Close()

		// 3. Check that we get a 400 Bad Request response
		assert.Equal(t, http.StatusBadRequest, getResponse.StatusCode, "Expected status code 400 for invalid policy ID format. Got %d. Message: %s", getResponse.StatusCode, readResponseBody(t, getResponse))

		// 4. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, getResponse)
		assert.NoError(t, err, "Failed to parse error response")

		// Verify error message contains information about the invalid UUID format
		assert.Contains(t, errorResp.Message, "invalid", "Error message should indicate that the policy ID format is invalid")
		assert.Equal(t, getEndpoint.method, errorResp.Method, "Expected method to be set")
		assert.Equal(t, getEndpoint.Path(), errorResp.Path, "Expected path to be set")

		// 5. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("require_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		getEndpoint := policyGetEndpoint.RewriteSlugs("00000000-0000-0000-0000-000000000000")
		resp, err := sendHTTPRequest(t, ctx, getEndpoint, nil)
		assert.NoError(t, err, "Failed to send request without authentication")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

func TestPolicyDelete(t *testing.T) {
	// Test policy deletion
	t.Run("delete_policy", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a new policy
		policyID := uuid.NewV7()
		policy := map[string]any{
			"id":               policyID.String(),
			"name":             "test_policy_delete_" + policyID.String(),
			"description":      "This is a test policy for delete " + policyID.String(),
			"allowed_action":   "GET",
			"allowed_resource": "/roles/*",
		}

		createResponse, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, policy, accessTokenHeader)
		assert.NoError(t, err, "Error sending create request: %v", err)
		defer createResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, createResponse.StatusCode, "Expected status code 201 Created for setup")

		// 3. Delete the policy
		deleteEndpoint := policyDeleteEndpoint.RewriteSlugs(policyID.String())
		deleteResponse, err := sendHTTPRequest(t, ctx, deleteEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Error sending delete request: %v", err)
		defer deleteResponse.Body.Close()

		// 4. Check the delete response
		assert.Equal(t, http.StatusOK, deleteResponse.StatusCode, "Expected status code 200 OK for delete")
		deleteAPIResp, err := parserResponseBody[payload.HTTPMessage](t, deleteResponse)
		assert.NoError(t, err, "Failed to parse delete response body")

		assert.Equal(t, domain.PoliciesPolicyDeletedSuccessfully, deleteAPIResp.Message, "Unexpected delete response message")
		assert.Equal(t, deleteEndpoint.method, deleteAPIResp.Method, "Expected method to be set for delete")
		assert.Equal(t, deleteEndpoint.Path(), deleteAPIResp.Path, "Expected path to be set for delete")

		// 5. Verify policy is actually deleted (try to get it)
		getEndpoint := policyGetEndpoint.RewriteSlugs(policyID.String())
		getResponse, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Error sending get request after delete: %v", err)
		defer getResponse.Body.Close()
		assert.Equal(t, http.StatusNotFound, getResponse.StatusCode, "Expected status code 404 Not Found after deletion")

		// 6. Cleanup admin user
		t.Cleanup(func() {
			// Policy should already be deleted by the test, but try again just in case
			deletePolicyByIDFromDB(t, policyID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test deleting a policy with an invalid ID format
	t.Run("delete_policy_bad_request", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Try to delete a policy with an invalid ID format (not a UUID)
		invalidPolicyID := "not-a-valid-uuid"
		deleteEndpoint := policyDeleteEndpoint.RewriteSlugs(invalidPolicyID)

		deleteResponse, err := sendHTTPRequest(t, ctx, deleteEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to delete policy with invalid ID")
		defer deleteResponse.Body.Close()

		// 3. Check that we get a 400 Bad Request response
		assert.Equal(t, http.StatusBadRequest, deleteResponse.StatusCode, "Expected status code 400 for invalid policy ID format. Got %d. Message: %s", deleteResponse.StatusCode, readResponseBody(t, deleteResponse))

		// 4. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, deleteResponse)
		assert.NoError(t, err, "Failed to parse error response")

		// 5. Verify error message contains information about the invalid UUID format
		assert.Contains(t, errorResp.Message, "invalid", "Error message should indicate that the policy ID format is invalid")
		assert.Equal(t, deleteEndpoint.method, errorResp.Method, "Expected method to be set")
		assert.Equal(t, deleteEndpoint.Path(), errorResp.Path, "Expected path to be set")

		// 6. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test deleting a non-existent policy
	t.Run("delete_policy_not_found", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Generate a UUID that doesn't exist in the database
		nonExistentPolicyID := uuid.NewV7()
		deleteEndpoint := policyDeleteEndpoint.RewriteSlugs(nonExistentPolicyID.String())

		deleteResponse, err := sendHTTPRequest(t, ctx, deleteEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to delete non-existent policy")
		defer deleteResponse.Body.Close()

		// 3. Check the response - this should still return StatusOK even though the policy doesn't exist
		// This is because deleting a non-existent resource is considered idempotent in RESTful APIs
		assert.Equal(t, http.StatusOK, deleteResponse.StatusCode, "Expected status code 200 OK for deleting non-existent policy. Got %d. Message: %s", deleteResponse.StatusCode, readResponseBody(t, deleteResponse))

		// 4. Parse and verify the success response
		deleteAPIResp, err := parserResponseBody[payload.HTTPMessage](t, deleteResponse)
		assert.NoError(t, err, "Failed to parse response body")

		// 5. Verify success message for deletion
		assert.Equal(t, domain.PoliciesPolicyDeletedSuccessfully, deleteAPIResp.Message, "Expected success message")
		assert.Equal(t, deleteEndpoint.method, deleteAPIResp.Method, "Expected method to be set")
		assert.Equal(t, deleteEndpoint.Path(), deleteAPIResp.Path, "Expected path to be set")

		// 6. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("require_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		deleteEndpoint := policyDeleteEndpoint.RewriteSlugs("00000000-0000-0000-0000-000000000000")
		resp, err := sendHTTPRequest(t, ctx, deleteEndpoint, nil)
		assert.NoError(t, err, "Failed to send request without authentication")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

func TestPolicyUpdate(t *testing.T) {
	// Test policy update
	t.Run("update_policy", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a new policy
		policyID := uuid.NewV7()
		originalName := "test_policy_update_" + policyID.String()
		originalDesc := "Original description " + policyID.String()
		originalAction := "GET"
		originalResource := "/roles/*"

		policy := map[string]any{
			"id":               policyID.String(),
			"name":             originalName,
			"description":      originalDesc,
			"allowed_action":   originalAction,
			"allowed_resource": originalResource,
		}

		createResponse, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, policy, accessTokenHeader)
		assert.NoError(t, err, "Error sending create request: %v", err)
		defer createResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, createResponse.StatusCode, "Expected status code 201 Created for setup")

		// 3. Update the policy
		updatedName := "updated_" + originalName
		updatedDesc := "Updated description " + policyID.String()
		updatedAction := "PUT"
		updatedResource := "/roles/*"

		updatedPolicy := map[string]any{
			"name":             updatedName,
			"description":      updatedDesc,
			"allowed_action":   updatedAction,
			"allowed_resource": updatedResource,
		}

		updateEndpoint := policyUpdateEndpoint.RewriteSlugs(policyID.String())
		updateResponse, err := sendHTTPRequest(t, ctx, updateEndpoint, updatedPolicy, accessTokenHeader)
		assert.NoError(t, err, "Error sending update request: %v", err)
		defer updateResponse.Body.Close()

		// 4. Check the update response
		assert.Equal(t, http.StatusOK, updateResponse.StatusCode, "Expected status code 200 OK for update", updateResponse.StatusCode, readResponseBody(t, updateResponse))
		updateAPIResp, err := parserResponseBody[payload.HTTPMessage](t, updateResponse)
		assert.NoError(t, err, "Failed to parse update response body")

		assert.Equal(t, domain.PoliciesPolicyUpdatedSuccessfully, updateAPIResp.Message, "Unexpected update response message")
		assert.Equal(t, updateEndpoint.method, updateAPIResp.Method, "Expected method to be set for update")
		assert.Equal(t, updateEndpoint.Path(), updateAPIResp.Path, "Expected path to be set for update")

		// 5. Verify policy is actually updated (get it again)
		getEndpoint := policyGetEndpoint.RewriteSlugs(policyID.String())
		getResponse, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Error sending get request after update: %v", err)
		defer getResponse.Body.Close()

		assert.Equal(t, http.StatusOK, getResponse.StatusCode, "Expected status code 200 OK when getting updated policy", getResponse.StatusCode, readResponseBody(t, getResponse))

		getAPIResp, err := parserResponseBody[payload.PolicyResponse](t, getResponse)
		assert.NoError(t, err, "Failed to parse get response body for updated policy")

		assert.Equal(t, policyID, getAPIResp.ID, "Expected policy ID to remain the same")
		assert.Equal(t, updatedName, getAPIResp.Name, "Expected policy name to be updated")
		assert.Equal(t, updatedDesc, getAPIResp.Description, "Expected policy description to be updated")
		assert.Equal(t, updatedAction, getAPIResp.AllowedAction, "Expected policy action to be updated")
		assert.Equal(t, updatedResource, getAPIResp.AllowedResource, "Expected policy resource to be updated")

		// 6. Cleanup
		t.Cleanup(func() {
			deletePolicyByIDFromDB(t, policyID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test updating a policy with invalid data
	t.Run("update_policy_bad_request", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a valid policy first that we'll try to update with invalid data
		policyID := uuid.NewV7()
		policy := map[string]any{
			"id":               policyID.String(),
			"name":             "test_policy_update_invalid_" + policyID.String(),
			"description":      "This is a test policy for update with invalid data",
			"allowed_action":   "GET",
			"allowed_resource": "/users/*",
		}

		createResponse, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, policy, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to create policy")
		defer createResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, createResponse.StatusCode, "Expected status code 201 for initial policy creation. Got %d. Message: %s", createResponse.StatusCode, readResponseBody(t, createResponse))

		// 3. Set up test cases for various invalid inputs
		testCases := []struct {
			name          string
			invalidUpdate map[string]any
			expectedError string
		}{
			{
				name: "Empty name",
				invalidUpdate: map[string]any{
					"name": "",
				},
				expectedError: "cannot be empty",
			},
			{
				name: "Invalid action",
				invalidUpdate: map[string]any{
					"allowed_action": "INVALID_ACTION",
				},
				expectedError: "invalid action",
			},
			{
				name: "Empty resource",
				invalidUpdate: map[string]any{
					"allowed_resource": "",
				},
				expectedError: "cannot be empty",
			},
		}

		// 4. Run each test case
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Try to update the policy with invalid data
				updateEndpoint := policyUpdateEndpoint.RewriteSlugs(policyID.String())
				updateResponse, err := sendHTTPRequest(t, ctx, updateEndpoint, tc.invalidUpdate, accessTokenHeader)
				assert.NoError(t, err, "Failed to send request for %s", tc.name)
				defer updateResponse.Body.Close()

				// Verify we get a 400 Bad Request response
				assert.Equal(t, http.StatusBadRequest, updateResponse.StatusCode,
					"Expected status code 400 Bad Request for %s. Got %d. Message: %s", tc.name, updateResponse.StatusCode, readResponseBody(t, updateResponse))

				// Parse and verify the error response
				errorResp, err := parserResponseBody[payload.HTTPMessage](t, updateResponse)
				assert.NoError(t, err, "Failed to parse error response for %s", tc.name)

				// Verify error message contains information specific to this validation failure
				assert.Contains(t, strings.ToLower(errorResp.Message), strings.ToLower(tc.expectedError),
					"Error message should indicate %s validation failure", tc.name)
				assert.Equal(t, updateEndpoint.method, errorResp.Method, "Expected method to be set")
				assert.Equal(t, updateEndpoint.Path(), errorResp.Path, "Expected path to be set")
			})
		}

		// 5. Cleanup
		t.Cleanup(func() {
			deletePolicyByIDFromDB(t, policyID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test updating a non-existent policy
	t.Run("update_policy_not_found", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Generate a random UUID that doesn't exist
		nonExistentPolicyID := uuid.NewV7()
		updateEndpoint := policyUpdateEndpoint.RewriteSlugs(nonExistentPolicyID.String())

		// Create update data
		updatedPolicy := map[string]any{
			"name":             "Updated Policy Name",
			"description":      "Updated policy description",
			"allowed_action":   "PUT",
			"allowed_resource": "/roles/*",
		}

		// 3. Try to update the non-existent policy
		updateResponse, err := sendHTTPRequest(t, ctx, updateEndpoint, updatedPolicy, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to update non-existent policy")
		defer updateResponse.Body.Close()

		// 4. Check that we get a 404 Not Found response
		assert.Equal(t, http.StatusNotFound, updateResponse.StatusCode, "Expected status code 404 for non-existent policy. Got %d. Message: %s", updateResponse.StatusCode, readResponseBody(t, updateResponse))

		// 5. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, updateResponse)
		assert.NoError(t, err, "Failed to parse error response")

		// Verify error message contains information about the policy not being found
		assert.Contains(t, errorResp.Message, "not found", "Error message should indicate that the policy was not found")
		assert.Equal(t, updateEndpoint.method, errorResp.Method, "Expected method to be set")
		assert.Equal(t, updateEndpoint.Path(), errorResp.Path, "Expected path to be set")

		// 6. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test updating a policy with a name that already exists
	t.Run("update_policy_conflict_name", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create two policies with different names
		// First policy - this is the one we'll try to update
		policyID1 := uuid.NewV7()
		policyName1 := "test_conflict_1_" + policyID1.String()
		policy1 := map[string]any{
			"id":               policyID1.String(),
			"name":             policyName1,
			"description":      "First test policy for conflict check",
			"allowed_action":   "GET",
			"allowed_resource": "/users/*",
		}

		createResponse1, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, policy1, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to create first policy")
		defer createResponse1.Body.Close()
		assert.Equal(t, http.StatusCreated, createResponse1.StatusCode, "Expected status code 201 for first policy creation. Got %d. Message: %s", createResponse1.StatusCode, readResponseBody(t, createResponse1))

		// Second policy - we'll try to use this policy's name when updating the first policy
		policyID2 := uuid.NewV7()
		policyName2 := "test_conflict_2_" + policyID2.String()
		policy2 := map[string]any{
			"id":               policyID2.String(),
			"name":             policyName2,
			"description":      "Second test policy for conflict check",
			"allowed_action":   "GET",
			"allowed_resource": "/roles/*",
		}

		createResponse2, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, policy2, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to create second policy")
		defer createResponse2.Body.Close()
		assert.Equal(t, http.StatusCreated, createResponse2.StatusCode, "Expected status code 201 for second policy creation. Got %d. Message: %s", createResponse2.StatusCode, readResponseBody(t, createResponse2))

		// 3. Try to update the first policy with the second policy's name
		updatePolicy1WithConflict := map[string]any{
			"name":             policyName2, // This will cause a conflict because policyName2, allows the same action and resource as policyName1
			"allowed_action":   "GET",
			"allowed_resource": "/roles/*",
		}

		updateEndpoint := policyUpdateEndpoint.RewriteSlugs(policyID1.String())
		updateResponse, err := sendHTTPRequest(t, ctx, updateEndpoint, updatePolicy1WithConflict, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to update policy with conflicting name")
		defer updateResponse.Body.Close()

		// 4. Check that we get a 409 Conflict response
		assert.Equal(t, http.StatusConflict, updateResponse.StatusCode, "Expected status code 409 Conflict for update with already used name. Got %d. Message: %s", updateResponse.StatusCode, readResponseBody(t, updateResponse))

		// 5. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, updateResponse)
		assert.NoError(t, err, "Failed to parse error response")

		// 6. Verify error message contains information about the name being already in use
		assert.Contains(t, strings.ToLower(errorResp.Message), "already exists", "Error message should indicate that the name already exists")
		assert.Equal(t, updateEndpoint.method, errorResp.Method, "Expected method to be set")
		assert.Equal(t, updateEndpoint.Path(), errorResp.Path, "Expected path to be set")

		// 7. Cleanup
		t.Cleanup(func() {
			deletePolicyByIDFromDB(t, policyID1)
			deletePolicyByIDFromDB(t, policyID2)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}

func TestPolicyList(t *testing.T) {
	// Test policy listing
	t.Run("list_policies", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a couple of new policies
		policyIDs := []uuid.UUID{uuid.NewV7(), uuid.NewV7(), uuid.NewV7(), uuid.NewV7(), uuid.NewV7()}
		policiesToCreate := map[string]map[string]any{}

		// validActions := strings.ReplaceAll(domain.GetValidActions(), " ", "")
		// actionsAllowed := strings.Split(validActions, ",")
		actionsAllowed := []string{"GET"}
		resourcesAllowed := []string{
			"/roles/*",
			"/users/*",
			"/policies/*",
		}

		for i, policyID := range policyIDs {
			randomAction := actionsAllowed[rand.Intn(len(actionsAllowed))]
			randomResource := resourcesAllowed[rand.Intn(len(resourcesAllowed))]

			policy := map[string]any{
				"id":               policyID.String(),
				"name":             "test_policy_list_" + policyID.String(),
				"description":      "This is a test policy for list " + policyID.String(),
				"allowed_action":   randomAction,
				"allowed_resource": randomResource,
			}
			policiesToCreate[policyID.String()] = policy

			createResponse, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, policy, accessTokenHeader)
			assert.NoError(t, err, "Failed to send request to create policy %d", i+1)
			if createResponse != nil {
				defer createResponse.Body.Close()

				createResponseMessage, err := parserResponseBody[payload.HTTPMessage](t, createResponse)
				assert.NoError(t, err, "Failed to parse create response body for policy %d", i+1)

				assert.Equal(t, domain.PoliciesPolicyCreatedSuccessfully, createResponseMessage.Message, "Unexpected response message for policy %d", i+1)
				assert.Equal(t, http.StatusCreated, createResponse.StatusCode, "Expected status code 201 for policy.")
			}
		}

		// 3. List the policies
		listResponse, err := sendHTTPRequest(t, ctx, policyListEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to list policies")
		defer listResponse.Body.Close()

		// 4. Check the list response
		assert.Equal(t, http.StatusOK, listResponse.StatusCode, "Expected status code 200 for list. Got %d. Message: %s", listResponse.StatusCode, readResponseBody(t, listResponse))
		// Assuming domain.ListPoliciesOutput exists and has an Items field []domain.Policy
		listAPIResp, err := parserResponseBody[payload.ListPoliciesResponse](t, listResponse)
		assert.NoError(t, err, "Failed to parse list response body")

		// 5. Verify the created policies are in the list

		for _, listedPolicy := range listAPIResp.Items {
			if _, ok := policiesToCreate[listedPolicy.Name]; ok {
				// Optionally assert other fields match
				for _, createdPolicy := range policiesToCreate {
					if createdPolicy["name"] == listedPolicy.Name {
						assert.Equal(t, createdPolicy["description"], listedPolicy.Description)
						assert.Equal(t, createdPolicy["action"], listedPolicy.AllowedAction)
						assert.Equal(t, createdPolicy["resource"], listedPolicy.AllowedResource)
						break
					}
				}
			}
		}
		assert.GreaterOrEqual(t, len(listAPIResp.Items), len(policiesToCreate), "Expected to find at least the created policies in the list")

		// 6. Cleanup
		t.Cleanup(func() {
			for _, policyID := range policyIDs {
				deletePolicyByIDFromDB(t, policyID)
			}
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}

func TestPolicyLinkRoles(t *testing.T) {
	t.Run("link_roles_to_policy", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Setup: Create admin, policy, and roles
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		policyID := createTestPolicy(t, adminToken.AccessToken, "link_test_policy_")
		roleID1 := createTestRole(t, adminToken.AccessToken, "link_test_role_1_")
		roleID2 := createTestRole(t, adminToken.AccessToken, "link_test_role_2_")

		// 2. Link roles to the policy
		linkEndpoint := policyLinkRolesEndpoint.RewriteSlugs(policyID.String())
		linkPayload := map[string]any{
			"role_ids": []string{roleID1.String(), roleID2.String()},
		}

		linkResponse, err := sendHTTPRequest(t, ctx, linkEndpoint, linkPayload, accessTokenHeader) // Pass struct directly
		assert.NoError(t, err, "Error sending link roles request: %v", err)
		defer linkResponse.Body.Close()

		// 3. Check link response
		assert.Equal(t, http.StatusOK, linkResponse.StatusCode, "Expected status code 200 OK for linking roles. Got %d. Message: %s", linkResponse.StatusCode, readResponseBody(t, linkResponse))
		linkAPIResp, err := parserResponseBody[payload.HTTPMessage](t, linkResponse)
		assert.NoError(t, err, "Failed to parse link roles response body")

		assert.Equal(t, domain.PoliciesRolesLinkedSuccessfully, linkAPIResp.Message, "Unexpected link roles response message")

		// 4. Verify roles are linked (by getting one of the roles)
		getRoleEndpoint := rolesGetEndpoint.RewriteSlugs(roleID1.String())
		getRoleResponse, err := sendHTTPRequest(t, ctx, getRoleEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Error sending get role request after linking: %v", err)
		defer getRoleResponse.Body.Close()

		assert.Equal(t, http.StatusOK, getRoleResponse.StatusCode, "Expected status code 200 OK when getting role after linking. Got %d. Message: %s", getRoleResponse.StatusCode, readResponseBody(t, getRoleResponse))
		assert.NoError(t, err, "Failed to parse get role response body after linking")

		t.Cleanup(func() {
			// Attempt cleanup even if tests fail
			deletePolicyByIDFromDB(t, policyID)
			deleteRoleByIDFromDB(t, roleID1)
			deleteRoleByIDFromDB(t, roleID2)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}

func TestPolicyUnlinkRoles(t *testing.T) {
	t.Run("unlink_roles_from_policy", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Setup: Create admin, policy, roles, and link them
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		policyID := createTestPolicy(t, adminToken.AccessToken, "unlink_test_policy_")
		roleID1 := createTestRole(t, adminToken.AccessToken, "unlink_test_role_1_")
		roleID2 := createTestRole(t, adminToken.AccessToken, "unlink_test_role_2_")

		// Link roles first
		linkEndpoint := policyLinkRolesEndpoint.RewriteSlugs(policyID.String())
		linkPayload := map[string]any{
			"role_ids": []string{roleID1.String(), roleID2.String()},
		}

		linkResponse, err := sendHTTPRequest(t, ctx, linkEndpoint, linkPayload, accessTokenHeader)
		assert.NoError(t, err, "Error sending link roles request during setup: %v", err)
		defer linkResponse.Body.Close()

		assert.Equal(t, http.StatusOK, linkResponse.StatusCode, "Expected status code 200 OK for linking roles during setup. Got %d. Message: %s", linkResponse.StatusCode, readResponseBody(t, linkResponse))

		// 2. Unlink one role
		time.Sleep(500 * time.Millisecond) // Ensure different timestamps for unlinking
		unlinkEndpoint := policyUnlinkRolesEndpoint.RewriteSlugs(policyID.String())
		unlinkPayload := map[string]any{
			"role_ids": []string{roleID1.String()},
		}

		unlinkResponse, err := sendHTTPRequest(t, ctx, unlinkEndpoint, unlinkPayload, accessTokenHeader)
		assert.NoError(t, err, "Error sending unlink role request: %v", err)
		defer unlinkResponse.Body.Close()

		// 3. Check unlink response
		assert.Equal(t, http.StatusOK, unlinkResponse.StatusCode, "Expected status code 200 OK for unlinking role. Got %d. Message: %s", unlinkResponse.StatusCode, readResponseBody(t, unlinkResponse))
		unlinkAPIResp, err := parserResponseBody[payload.HTTPMessage](t, unlinkResponse)
		assert.NoError(t, err, "Failed to parse unlink role response body")

		assert.Equal(t, domain.PoliciesRolesUnlinkedSuccessfully, unlinkAPIResp.Message, "Unexpected unlink role response message")

		// 4. Verify role is unlinked (by getting the roles)
		rolesGetEndpoint := newAPIEndpoint(http.MethodGet, "/roles/{role_id}") // Define locally or ensure global access

		// Check Role 1 (should be unlinked)
		getRole1Endpoint := rolesGetEndpoint.RewriteSlugs(roleID1.String())
		getRole1Response, err := sendHTTPRequest(t, ctx, getRole1Endpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Error sending get role 1 request after unlinking: %v", err)
		defer getRole1Response.Body.Close()
		assert.Equal(t, http.StatusOK, getRole1Response.StatusCode, "Expected status code 200 OK when getting role 1 after unlinking. Got %d. Message: %s", getRole1Response.StatusCode, readResponseBody(t, getRole1Response))

		// Check Role 2 (should still be linked)
		getRole2Endpoint := rolesGetEndpoint.RewriteSlugs(roleID2.String())
		getRole2Response, err := sendHTTPRequest(t, ctx, getRole2Endpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Error sending get role 2 request after unlinking: %v", err)
		defer getRole2Response.Body.Close()
		assert.Equal(t, http.StatusOK, getRole2Response.StatusCode, "Expected status code 200 OK when getting role 2 after unlinking")

		t.Cleanup(func() {
			// Attempt cleanup even if tests fail
			deletePolicyByIDFromDB(t, policyID)
			deleteRoleByIDFromDB(t, roleID1)
			deleteRoleByIDFromDB(t, roleID2)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}

// TestPolicyCreate_EdgeCases tests policy creation with various edge cases
func TestPolicyCreate_EdgeCases(t *testing.T) {
	t.Run("create_policy_without_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		policyID := uuid.NewV7()
		policy := map[string]any{
			"id":               policyID.String(),
			"name":             "test-policy-" + policyID.String(),
			"description":      "Test policy",
			"allowed_action":   "GET",
			"allowed_resource": "/api/v1/test",
		}

		response, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, policy)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, response.StatusCode, "Expected status code 401. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})

	t.Run("create_policy_with_invalid_token", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		invalidTokenHeader := map[string]string{
			"Authorization": "Bearer invalid.token.here",
		}

		policyID := uuid.NewV7()
		policy := map[string]any{
			"id":               policyID.String(),
			"name":             "test-policy-" + policyID.String(),
			"description":      "Test policy",
			"allowed_action":   "GET",
			"allowed_resource": "/api/v1/test",
		}

		response, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, policy, invalidTokenHeader)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, response.StatusCode, "Expected status code 401. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})

	t.Run("create_policy_with_empty_name", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		policyID := uuid.NewV7()
		policy := map[string]any{
			"id":               policyID.String(),
			"name":             "",
			"description":      "Test policy",
			"allowed_action":   "GET",
			"allowed_resource": "/api/v1/test",
		}

		response, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, policy, accessTokenHeader)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusBadRequest, response.StatusCode, "Expected status code 400. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})

	t.Run("create_policy_with_invalid_effect", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		policyID := uuid.NewV7()
		policy := map[string]any{
			"id":          policyID.String(),
			"name":        "test-policy-" + policyID.String(),
			"description": "Test policy",
			// "effect" was never a field of this API. It used to be dropped
			// silently; it is refused by name now, which is the 400 asserted.
			"allowed_action":   "GET",
			"allowed_resource": "/api/v1/test",
			"effect":           "invalid",
		}

		response, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, policy, accessTokenHeader)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusBadRequest, response.StatusCode, "Expected status code 400. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})

	t.Run("create_policy_with_invalid_uuid", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		policy := map[string]any{
			"id":               "invalid-uuid",
			"name":             "test-policy-invalid",
			"description":      "Test policy",
			"allowed_action":   "GET",
			"allowed_resource": "/api/v1/test",
		}

		response, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, policy, accessTokenHeader)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusBadRequest, response.StatusCode, "Expected status code 400. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})
}

// TestPolicyGet_EdgeCases tests policy retrieval with edge cases
func TestPolicyGet_EdgeCases(t *testing.T) {
	t.Run("get_policy_without_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		policyID := uuid.NewV7()

		getEndpoint := policyGetEndpoint.RewriteSlugs(policyID.String())
		response, err := sendHTTPRequest(t, ctx, getEndpoint, nil)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, response.StatusCode, "Expected status code 401. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})

	t.Run("get_policy_with_invalid_uuid", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		getEndpoint := policyGetEndpoint.RewriteSlugs("invalid-uuid")
		response, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusBadRequest, response.StatusCode, "Expected status code 400. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})

	t.Run("get_non_existent_policy", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		nonExistentPolicyID := uuid.NewV7()

		getEndpoint := policyGetEndpoint.RewriteSlugs(nonExistentPolicyID.String())
		response, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusNotFound, response.StatusCode, "Expected status code 404. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})
}

// TestPolicyUpdate_EdgeCases tests policy update with edge cases
func TestPolicyUpdate_EdgeCases(t *testing.T) {
	t.Run("update_policy_without_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		policyID := uuid.NewV7()

		updatePayload := map[string]any{
			"name": "updated-policy",
		}

		updateEndpoint := policyUpdateEndpoint.RewriteSlugs(policyID.String())
		response, err := sendHTTPRequest(t, ctx, updateEndpoint, updatePayload)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, response.StatusCode, "Expected status code 401. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})

	t.Run("update_non_existent_policy", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		nonExistentPolicyID := uuid.NewV7()

		updatePayload := map[string]any{
			"name": "updated-policy",
		}

		updateEndpoint := policyUpdateEndpoint.RewriteSlugs(nonExistentPolicyID.String())
		response, err := sendHTTPRequest(t, ctx, updateEndpoint, updatePayload, accessTokenHeader)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusNotFound, response.StatusCode, "Expected status code 404. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})

	t.Run("update_policy_with_invalid_effect", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// Create a policy first
		policyID := createTestPolicyWithParams(t, adminToken.AccessToken, "test-policy-", "/users", "GET", "allow")

		// Try to update with invalid effect
		updatePayload := map[string]any{
			"effect": "invalid-effect",
		}

		updateEndpoint := policyUpdateEndpoint.RewriteSlugs(policyID.String())
		response, err := sendHTTPRequest(t, ctx, updateEndpoint, updatePayload, accessTokenHeader)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusBadRequest, response.StatusCode, "Expected status code 400. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		t.Cleanup(func() {
			deletePolicyByIDFromDB(t, policyID)
		})
	})
}

// TestPolicyDelete_EdgeCases tests policy deletion with edge cases
func TestPolicyDelete_EdgeCases(t *testing.T) {
	t.Run("delete_policy_without_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		policyID := uuid.NewV7()

		deleteEndpoint := policyDeleteEndpoint.RewriteSlugs(policyID.String())
		response, err := sendHTTPRequest(t, ctx, deleteEndpoint, nil)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, response.StatusCode, "Expected status code 401. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})

	t.Run("delete_non_existent_policy", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		nonExistentPolicyID := uuid.NewV7()

		deleteEndpoint := policyDeleteEndpoint.RewriteSlugs(nonExistentPolicyID.String())
		response, err := sendHTTPRequest(t, ctx, deleteEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200 (graceful delete for security). Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		// Verify success message
		apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse response body")
		assert.Equal(t, domain.PoliciesPolicyDeletedSuccessfully, apiResp.Message, "Expected success message even for non-existent policy (security pattern)")
	})

	t.Run("delete_policy_with_invalid_uuid", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		deleteEndpoint := policyDeleteEndpoint.RewriteSlugs("invalid-uuid")
		response, err := sendHTTPRequest(t, ctx, deleteEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusBadRequest, response.StatusCode, "Expected status code 400. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})
}

// TestPolicyList_EdgeCases tests policy listing with edge cases
func TestPolicyList_EdgeCases(t *testing.T) {
	t.Run("list_policies_without_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		response, err := sendHTTPRequest(t, ctx, policyListEndpoint, nil)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, response.StatusCode, "Expected status code 401. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})

	t.Run("list_policies_with_invalid_pagination", func(t *testing.T) {
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
				endpoint: newAPIEndpoint(http.MethodGet, "/policies?limit=-1"),
			},
			{
				name:     "limit_too_large",
				endpoint: newAPIEndpoint(http.MethodGet, "/policies?limit=1001"),
			},
			{
				name:     "invalid_cursor",
				endpoint: newAPIEndpoint(http.MethodGet, "/policies?cursor=invalid-cursor"),
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

// TestPolicyLinkRoles_EdgeCases tests linking roles to policies with edge cases
func TestPolicyLinkRoles_EdgeCases(t *testing.T) {
	t.Run("link_roles_to_policy_without_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		policyID := uuid.NewV7()

		linkPayload := map[string]any{
			"role_ids": []string{uuid.NewV7().String()},
		}

		linkEndpoint := policyLinkRolesEndpoint.RewriteSlugs(policyID.String())
		response, err := sendHTTPRequest(t, ctx, linkEndpoint, linkPayload)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, response.StatusCode, "Expected status code 401. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})

	t.Run("link_roles_to_non_existent_policy", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		nonExistentPolicyID := uuid.NewV7()

		linkPayload := map[string]any{
			"role_ids": []string{uuid.NewV7().String()},
		}

		linkEndpoint := policyLinkRolesEndpoint.RewriteSlugs(nonExistentPolicyID.String())
		response, err := sendHTTPRequest(t, ctx, linkEndpoint, linkPayload, accessTokenHeader)
		assert.NoError(t, err)
		defer response.Body.Close()

		// The foreign-key violation is mapped to the row that is missing. It
		// used to fall through as a 500 carrying the driver's text.
		assert.Equal(t, http.StatusNotFound, response.StatusCode, "Expected status code 404 for a link to a missing policy. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse response body")
		assert.Contains(t, apiResp.Message, "not found", "Expected a not-found message")
		assert.NotContains(t, apiResp.Message, "foreign key", "The driver's text must not reach the client")
	})

	t.Run("link_roles_with_empty_array", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// Create a policy
		policyID := createTestPolicyWithParams(t, adminToken.AccessToken, "test-policy-", "/users", "GET", "allow")

		linkPayload := map[string]any{
			"role_ids": []string{},
		}

		linkEndpoint := policyLinkRolesEndpoint.RewriteSlugs(policyID.String())
		response, err := sendHTTPRequest(t, ctx, linkEndpoint, linkPayload, accessTokenHeader)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusBadRequest, response.StatusCode, "Expected status code 400. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		t.Cleanup(func() {
			deletePolicyByIDFromDB(t, policyID)
		})
	})

	t.Run("link_roles_with_invalid_role_id", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// Create a policy
		policyID := createTestPolicyWithParams(t, adminToken.AccessToken, "test-policy-", "/users", "GET", "allow")

		linkPayload := map[string]any{
			"role_ids": []string{"invalid-uuid"},
		}

		linkEndpoint := policyLinkRolesEndpoint.RewriteSlugs(policyID.String())
		response, err := sendHTTPRequest(t, ctx, linkEndpoint, linkPayload, accessTokenHeader)
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusBadRequest, response.StatusCode, "Expected status code 400. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		t.Cleanup(func() {
			deletePolicyByIDFromDB(t, policyID)
		})
	})
}

// TestPolicyJSONSerialization tests that all policy endpoints return JSON responses with snake_case field names
func TestPolicyJSONSerialization(t *testing.T) {
	// Test that PolicyResponse uses snake_case in JSON serialization
	t.Run("get_policy_response_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test policy
		policyID := uuid.NewV7()

		policy := map[string]any{
			"id":               policyID.String(),
			"name":             generateRandomName(t, ""),
			"description":      "Test policy for JSON serialization",
			"allowed_action":   "GET",
			"allowed_resource": "/users",
		}

		createResponse, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, policy, accessTokenHeader)
		require.NoError(t, err, "Failed to create policy")
		defer createResponse.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse.StatusCode)

		// 3. Get the policy
		getPolicyEndpoint := policyGetEndpoint.RewriteSlugs(policyID.String())
		response, err := sendHTTPRequest(t, ctx, getPolicyEndpoint, nil, accessTokenHeader)
		require.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		// 4. Verify all PolicyResponse fields are in snake_case
		expectedFields := []string{
			"id",
			"name",
			"description",
			"system",
			"resource",
			"allowed_action",
			"allowed_resource",
			"created_at",
			"updated_at",
		}

		assertJSONFieldsAreSnakeCase(t, response, expectedFields)

		t.Cleanup(func() {
			deletePolicyByIDFromDB(t, policyID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test that ListPoliciesOutput uses snake_case including nested objects
	t.Run("list_policies_response_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create test policies
		policy1ID := uuid.NewV7()

		policy1 := map[string]any{
			"id":               policy1ID.String(),
			"name":             generateRandomName(t, ""),
			"description":      "Test policy 1",
			"allowed_action":   "GET",
			"allowed_resource": "/users",
		}

		createResponse1, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, policy1, accessTokenHeader)
		require.NoError(t, err, "Failed to create policy 1")
		defer createResponse1.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse1.StatusCode)

		policy2ID := uuid.NewV7()

		policy2 := map[string]any{
			"id":               policy2ID.String(),
			"name":             generateRandomName(t, ""),
			"description":      "Test policy 2",
			"allowed_action":   "POST",
			"allowed_resource": "/projects",
		}

		createResponse2, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, policy2, accessTokenHeader)
		require.NoError(t, err, "Failed to create policy 2")
		defer createResponse2.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse2.StatusCode)

		// 3. List policies
		response, err := sendHTTPRequest(t, ctx, policyListEndpoint, nil, accessTokenHeader)
		require.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		// 4. Verify ListPoliciesOutput top-level fields are snake_case
		expectedTopLevelFields := []string{
			"items",
			"paginator",
		}

		assertJSONFieldsAreSnakeCase(t, response, expectedTopLevelFields)

		t.Cleanup(func() {
			deletePolicyByIDFromDB(t, policy1ID)
			deletePolicyByIDFromDB(t, policy2ID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test that HTTPMessage response uses snake_case
	t.Run("create_policy_http_message_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a new policy
		policyID := uuid.NewV7()

		policy := map[string]any{
			"id":               policyID.String(),
			"name":             generateRandomName(t, ""),
			"description":      "Test policy for HTTP message",
			"allowed_action":   "GET",
			"allowed_resource": "/users",
		}

		response, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, policy, accessTokenHeader)
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
			deletePolicyByIDFromDB(t, policyID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test that update response uses snake_case
	t.Run("update_policy_response_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test policy
		policyID := uuid.NewV7()

		policy := map[string]any{
			"id":               policyID.String(),
			"name":             generateRandomName(t, ""),
			"description":      "Test policy for update",
			"allowed_action":   "GET",
			"allowed_resource": "/users",
		}

		createResponse, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, policy, accessTokenHeader)
		require.NoError(t, err, "Failed to create policy")
		defer createResponse.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse.StatusCode)

		// 3. Update the policy
		newName := generateRandomName(t, "")
		updatePayload := map[string]any{
			"name": newName,
		}

		updateEndpoint := policyUpdateEndpoint.RewriteSlugs(policyID.String())
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
			deletePolicyByIDFromDB(t, policyID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test that delete response uses snake_case
	t.Run("delete_policy_response_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test policy
		policyID := uuid.NewV7()

		policy := map[string]any{
			"id":               policyID.String(),
			"name":             generateRandomName(t, ""),
			"description":      "Test policy for delete",
			"allowed_action":   "GET",
			"allowed_resource": "/users",
		}

		createResponse, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, policy, accessTokenHeader)
		require.NoError(t, err, "Failed to create policy")
		defer createResponse.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse.StatusCode)

		// 3. Delete the policy
		deleteEndpoint := policyDeleteEndpoint.RewriteSlugs(policyID.String())
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

	// Test that link roles response uses snake_case
	t.Run("link_roles_response_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test policy
		policyID := uuid.NewV7()

		policy := map[string]any{
			"id":               policyID.String(),
			"name":             generateRandomName(t, ""),
			"description":      "Test policy for link roles",
			"allowed_action":   "GET",
			"allowed_resource": "/users",
		}

		createResponse, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, policy, accessTokenHeader)
		require.NoError(t, err, "Failed to create policy")
		defer createResponse.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse.StatusCode)

		// 3. Get a role ID from database (assuming roles exist from migrations)
		var roleID uuid.UUID
		query := `SELECT id FROM roles LIMIT 1`
		err = testDBPool.QueryRow(ctx, query).Scan(&roleID)
		require.NoError(t, err, "Failed to get role from database")

		// 4. Link role to policy
		linkPayload := map[string]any{
			"role_ids": []uuid.UUID{roleID},
		}

		linkEndpoint := policyLinkRolesEndpoint.RewriteSlugs(policyID.String())
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
			unlinkPoliciesFromRoleViaDB(t, roleID, []uuid.UUID{policyID})
			deletePolicyByIDFromDB(t, policyID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test that unlink roles response uses snake_case
	t.Run("unlink_roles_response_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test policy
		policyID := uuid.NewV7()

		policy := map[string]any{
			"id":               policyID.String(),
			"name":             generateRandomName(t, ""),
			"description":      "Test policy for unlink roles",
			"allowed_action":   "GET",
			"allowed_resource": "/users",
		}

		createResponse, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, policy, accessTokenHeader)
		require.NoError(t, err, "Failed to create policy")
		defer createResponse.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse.StatusCode)

		// 3. Get a role ID and link it first
		var roleID uuid.UUID
		query := `SELECT id FROM roles LIMIT 1`
		err = testDBPool.QueryRow(ctx, query).Scan(&roleID)
		require.NoError(t, err, "Failed to get role from database")

		// Link the role
		linkQuery := `INSERT INTO roles_policies (roles_id, policies_id) VALUES ($1, $2)`
		_, err = testDBPool.Exec(ctx, linkQuery, roleID, policyID)
		require.NoError(t, err, "Failed to link role to policy")

		// 4. Unlink role from policy
		unlinkPayload := map[string]any{
			"role_ids": []uuid.UUID{roleID},
		}

		unlinkEndpoint := policyUnlinkRolesEndpoint.RewriteSlugs(policyID.String())
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
			unlinkPoliciesFromRoleViaDB(t, roleID, []uuid.UUID{policyID})
			deletePolicyByIDFromDB(t, policyID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test that list policies by role response uses snake_case
	t.Run("list_policies_by_role_uses_snake_case", func(t *testing.T) {
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
			"description": "Test role for list policies",
		}

		rolesCreateEndpoint := newAPIEndpoint(http.MethodPost, "/roles")
		createRoleResponse, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role, accessTokenHeader)
		require.NoError(t, err, "Failed to create role")
		defer createRoleResponse.Body.Close()
		require.Equal(t, http.StatusCreated, createRoleResponse.StatusCode)

		// 3. Create test policies and link them
		policy1ID := uuid.NewV7()

		policy1 := map[string]any{
			"id":               policy1ID.String(),
			"name":             generateRandomName(t, ""),
			"description":      "Test policy 1",
			"allowed_action":   "GET",
			"allowed_resource": "/users",
		}

		createResponse1, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, policy1, accessTokenHeader)
		require.NoError(t, err, "Failed to create policy 1")
		defer createResponse1.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse1.StatusCode)

		policy2ID := uuid.NewV7()

		policy2 := map[string]any{
			"id":               policy2ID.String(),
			"name":             generateRandomName(t, ""),
			"description":      "Test policy 2",
			"allowed_action":   "POST",
			"allowed_resource": "/projects",
		}

		createResponse2, err := sendHTTPRequest(t, ctx, policyCreateEndpoint, policy2, accessTokenHeader)
		require.NoError(t, err, "Failed to create policy 2")
		defer createResponse2.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse2.StatusCode)

		// Link policies to role
		linkQuery := `INSERT INTO roles_policies (roles_id, policies_id) VALUES ($1, $2)`
		_, err = testDBPool.Exec(ctx, linkQuery, roleID, policy1ID)
		require.NoError(t, err, "Failed to link policy 1 to role")
		_, err = testDBPool.Exec(ctx, linkQuery, roleID, policy2ID)
		require.NoError(t, err, "Failed to link policy 2 to role")

		// 4. List policies by role ID
		listEndpoint := policyListPoliciesByRoleEndpoint.RewriteSlugs(roleID.String())
		response, err := sendHTTPRequest(t, ctx, listEndpoint, nil, accessTokenHeader)
		require.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		// 5. Verify ListPoliciesOutput top-level fields are snake_case
		expectedTopLevelFields := []string{
			"items",
			"paginator",
		}

		assertJSONFieldsAreSnakeCase(t, response, expectedTopLevelFields)

		t.Cleanup(func() {
			unlinkAllPoliciesFromRoleViaDB(t, roleID)
			deletePolicyByIDFromDB(t, policy1ID)
			deletePolicyByIDFromDB(t, policy2ID)
			deleteRoleByIDFromDB(t, roleID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}

// TestPolicyUnlinkRoles_Multiple is a regression test for PoliciesRepository.UnlinkRoles.
//
// The existing unlink test above unlinks exactly ONE role, which is why this
// went unnoticed. The repository built `('<policy>', '<role>')` per role — the
// tuple shape copied from LinkRoles, where it is correct for INSERT ... VALUES —
// and substituted the joined result into `roles_id IN %s`:
//
//	two+ roles → ... roles_id IN ('P','R1'), ('P','R2')
//	             ERROR: syntax error at or near "," (verified against Postgres)
//
//	one role   → ... roles_id IN ('P','R1')
//	             valid, but matches the POLICY id as though it were a role id;
//	             correct only because the two never collide
//
// This exercises both halves: unlinking two roles at once must succeed, and only
// the named roles may be removed.
func TestPolicyUnlinkRoles_Multiple(t *testing.T) {
	t.Run("unlink_two_roles_at_once", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		require.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		policyID := createTestPolicy(t, adminToken.AccessToken, "unlink_multi_policy_")
		roleID1 := createTestRole(t, adminToken.AccessToken, "unlink_multi_role_1_")
		roleID2 := createTestRole(t, adminToken.AccessToken, "unlink_multi_role_2_")
		roleID3 := createTestRole(t, adminToken.AccessToken, "unlink_multi_role_3_")

		t.Cleanup(func() {
			unlinkAllRolesFromPolicyViaDB(t, policyID)
			deletePolicyByIDFromDB(t, policyID)
			deleteRoleByIDFromDB(t, roleID1)
			deleteRoleByIDFromDB(t, roleID2)
			deleteRoleByIDFromDB(t, roleID3)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})

		// Link all three.
		linkEndpoint := policyLinkRolesEndpoint.RewriteSlugs(policyID.String())
		linkResponse, err := sendHTTPRequest(t, ctx, linkEndpoint, map[string]any{
			"role_ids": []string{roleID1.String(), roleID2.String(), roleID3.String()},
		}, accessTokenHeader)
		require.NoError(t, err, "Error linking roles during setup")
		defer linkResponse.Body.Close()
		require.Equal(t, http.StatusOK, linkResponse.StatusCode,
			"Expected 200 OK linking roles. Got %d. Message: %s",
			linkResponse.StatusCode, readResponseBody(t, linkResponse))

		require.Equal(t, 3, countRolesLinkedToPolicy(t, policyID), "setup should link three roles")

		// Unlink TWO at once — this is what used to fail with a syntax error.
		unlinkEndpoint := policyUnlinkRolesEndpoint.RewriteSlugs(policyID.String())
		unlinkResponse, err := sendHTTPRequest(t, ctx, unlinkEndpoint, map[string]any{
			"role_ids": []string{roleID1.String(), roleID2.String()},
		}, accessTokenHeader)
		require.NoError(t, err, "Error unlinking roles")
		defer unlinkResponse.Body.Close()

		require.Equal(t, http.StatusOK, unlinkResponse.StatusCode,
			"unlinking two roles at once must succeed. Got %d. Message: %s",
			unlinkResponse.StatusCode, readResponseBody(t, unlinkResponse))

		// Exactly the third role should remain.
		assert.Equal(t, 1, countRolesLinkedToPolicy(t, policyID),
			"only the two named roles should have been unlinked")
		assert.True(t, isRoleLinkedToPolicy(t, policyID, roleID3),
			"the role that was not named must still be linked")
		assert.False(t, isRoleLinkedToPolicy(t, policyID, roleID1), "role 1 should be unlinked")
		assert.False(t, isRoleLinkedToPolicy(t, policyID, roleID2), "role 2 should be unlinked")
	})
}
