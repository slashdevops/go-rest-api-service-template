//go:build integration

package integration

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

var (
	meAuthzEndpoint  = newAPIEndpoint(http.MethodGet, "/me/authz")
	meGetEndpoint    = newAPIEndpoint(http.MethodGet, "/me")
	meUpdateEndpoint = newAPIEndpoint(http.MethodPut, "/me")
)

func TestMeAuthz(t *testing.T) {
	// Test getting authorization information for authenticated user
	t.Run("get_authenticated_user_authz", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Call the /me/authz endpoint
		response, err := sendHTTPRequest(t, ctx, meAuthzEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		// 3. Parse and verify the response
		authzResponse, err := parserResponseBody[payload.GetAuthenticatedUserResponse](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		// 4. Verify the account information
		assert.Equal(t, adminToken.UserID, authzResponse.Account.ID, "User ID should match the authenticated user")
		assert.NotEmpty(t, authzResponse.Account.Email, "Email should not be empty")
		assert.NotEmpty(t, authzResponse.Account.FirstName, "First name should not be empty")
		assert.NotEmpty(t, authzResponse.Account.LastName, "Last name should not be empty")
		assert.NotZero(t, authzResponse.Account.CreatedAt, "Created at should not be zero")
		assert.NotZero(t, authzResponse.Account.UpdatedAt, "Updated at should not be zero")

		// 5. Verify the permissions structure exists
		assert.NotNil(t, authzResponse.Permissions, "Permissions should not be nil")
		// Assert the CONTENT, not the Go type.
		//
		// This was `assert.IsType(t, map[string]any{}, ...)`, written when the
		// field was an untyped map. #400 gave it a real type --
		// `payload.AuthzPermissions = map[string]AuthzSubjects` -- so the
		// assertion started failing with:
		//
		//   expected map[string]interface {}, but was map[string]map[string]map[string][]string
		//
		// It could not have caught anything even before that: the field is
		// DECLARED as its type, so an IsType check against it is a tautology
		// the compiler already guarantees. What is worth asserting is that the
		// permissions actually describe something.
		require.NotEmpty(t, authzResponse.Permissions, "Permissions should describe at least one resource")

		for resource, subjects := range authzResponse.Permissions {
			assert.NotEmpty(t, resource, "Permission resource name should not be empty")
			assert.NotEmpty(t, subjects, "Permission resource %q should name at least one subject", resource)
		}

		// 6. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("get_authenticated_user_authz_with_regular_user", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token to create another user
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		adminAccessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a regular user
		firstName, lastName, email := generateUserData(t)
		userID := uuid.NewV7()

		user := map[string]any{
			"id":         userID.String(),
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   generatePassword(t),
		}

		userCreateResponse, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, user, adminAccessTokenHeader)
		assert.NoError(t, err, "Failed to create user")
		defer userCreateResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, userCreateResponse.StatusCode, "Expected status code 201. Got %d. Message: %s", userCreateResponse.StatusCode, readResponseBody(t, userCreateResponse))

		// 3. enable the user to login in db
		enableUserByEmailFromDB(t, email)

		// 4. Login as the regular user to get their token
		loginRequest := map[string]any{
			"email":    email,
			"password": user["password"],
		}

		loginResponse, err := sendHTTPRequest(t, ctx, authLoginEndpoint, loginRequest)
		assert.NoError(t, err, "Failed to login user")
		defer loginResponse.Body.Close()
		assert.Equal(t, http.StatusOK, loginResponse.StatusCode, "Expected status code 200. Got %d. Message: %s", loginResponse.StatusCode, readResponseBody(t, loginResponse))

		userLoginResponse, err := parserResponseBody[payload.LoginUserResponse](t, loginResponse)
		assert.NoError(t, err, "Failed to parse login response")

		assert.NotEmpty(t, userLoginResponse.AccessToken, "Access token should not be empty")

		if userLoginResponse.AccessToken == "" {
			t.Fatal("Access token should not be empty after login")
		}

		userAccessTokenHeader := map[string]string{
			"Authorization": "Bearer " + userLoginResponse.AccessToken,
		}

		// 5. Call the /me/authz endpoint as the regular user
		response, err := sendHTTPRequest(t, ctx, meAuthzEndpoint, nil, userAccessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		// 6. Parse and verify the response
		authzResponse, err := parserResponseBody[payload.GetAuthenticatedUserResponse](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		// 7. Verify the account information matches the regular user
		assert.Equal(t, userID, authzResponse.Account.ID, "User ID should match the authenticated user")
		assert.Equal(t, email, authzResponse.Account.Email, "Email should match")
		assert.Equal(t, firstName, authzResponse.Account.FirstName, "First name should match")
		assert.Equal(t, lastName, authzResponse.Account.LastName, "Last name should match")
		assert.NotZero(t, authzResponse.Account.CreatedAt, "Created at should not be zero")
		assert.NotZero(t, authzResponse.Account.UpdatedAt, "Updated at should not be zero")

		// 8. Verify the permissions structure exists (regular users should have permissions too)
		assert.NotNil(t, authzResponse.Permissions, "Permissions should not be nil")
		// Assert the CONTENT, not the Go type.
		//
		// This was `assert.IsType(t, map[string]any{}, ...)`, written when the
		// field was an untyped map. #400 gave it a real type --
		// `payload.AuthzPermissions = map[string]AuthzSubjects` -- so the
		// assertion started failing with:
		//
		//   expected map[string]interface {}, but was map[string]map[string]map[string][]string
		//
		// It could not have caught anything even before that: the field is
		// DECLARED as its type, so an IsType check against it is a tautology
		// the compiler already guarantees. What is worth asserting is that the
		// permissions actually describe something.
		require.NotEmpty(t, authzResponse.Permissions, "Permissions should describe at least one resource")

		for resource, subjects := range authzResponse.Permissions {
			assert.NotEmpty(t, resource, "Permission resource name should not be empty")
			assert.NotEmpty(t, subjects, "Permission resource %q should name at least one subject", resource)
		}

		// 9. Verify the user is not an admin
		assert.NotNil(t, authzResponse.Account.Admin, "Admin field should not be nil")
		assert.False(t, *authzResponse.Account.Admin, "Regular user should not be an admin")

		// 10. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, userID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("require_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Try to call /me/authz without authentication
		response, err := sendHTTPRequest(t, ctx, meAuthzEndpoint, nil)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 2. Check that we get a 401 Unauthorized response
		assert.Equal(t, http.StatusUnauthorized, response.StatusCode, "Expected status code 401. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})

	t.Run("invalid_authentication_token", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Try to call /me/authz with an invalid token
		invalidTokenHeader := map[string]string{
			"Authorization": "Bearer invalid-token-here",
		}

		response, err := sendHTTPRequest(t, ctx, meAuthzEndpoint, nil, invalidTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 2. Check that we get a 401 Unauthorized response
		assert.Equal(t, http.StatusUnauthorized, response.StatusCode, "Expected status code 401. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})

	t.Run("user_not_found", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		// 2. Delete the user from the database to simulate a deleted user
		deleteUserByIDFromDB(t, adminToken.UserID)

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 3. Try to call /me/authz with the token of a deleted user
		response, err := sendHTTPRequest(t, ctx, meAuthzEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 4. Check that we get a 404 Not Found response (middleware handles auth before handler)
		assert.Equal(t, http.StatusNotFound, response.StatusCode, "Expected status code 404. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		// 5. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse error response")

		// Verify error message contains information about unauthorized access
		assert.Contains(t, strings.ToLower(errorResp.Message), "not found",
			"Error message should indicate user not found")
	})

	// Test response headers
	t.Run("verify_response_headers", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Call the /me/authz endpoint
		response, err := sendHTTPRequest(t, ctx, meAuthzEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		// 3. Verify Content-Type header
		contentType := response.Header.Get("Content-Type")
		assert.Contains(t, contentType, "application/json", "Content-Type should be application/json")

		// 4. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test permissions structure for admin users
	t.Run("verify_admin_permissions_structure", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Call the /me/authz endpoint
		response, err := sendHTTPRequest(t, ctx, meAuthzEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		// 3. Parse the response
		authzResponse, err := parserResponseBody[payload.GetAuthenticatedUserResponse](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		// 4. Verify the user is an admin
		assert.NotNil(t, authzResponse.Account.Admin, "Admin field should not be nil")
		assert.True(t, *authzResponse.Account.Admin, "User should be an admin")

		// 5. Verify permissions structure is not empty for admin
		assert.NotNil(t, authzResponse.Permissions, "Permissions should not be nil")
		assert.NotEmpty(t, authzResponse.Permissions, "Admin should have permissions")

		// 6. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test /me/authz with a user that has a role with multiple policies including multiple actions on the same resource
	t.Run("authz_with_role_multiple_policies_multiple_actions_same_resource", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user to set up the test
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		adminAccessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a regular user
		firstName, lastName, email := generateUserData(t)
		userID := uuid.NewV7()

		user := map[string]any{
			"id":         userID.String(),
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   generatePassword(t),
		}

		userCreateResponse, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, user, adminAccessTokenHeader)
		assert.NoError(t, err, "Failed to create user")
		defer userCreateResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, userCreateResponse.StatusCode, "Expected status code 201. Got %d. Message: %s", userCreateResponse.StatusCode, readResponseBody(t, userCreateResponse))

		// 3. Create a role for the test
		roleID := uuid.NewV7()

		role := map[string]any{
			"id":          roleID.String(),
			"name":        "test-role-" + roleID.String(),
			"description": "Test role with multiple policies and actions",
		}

		rolesCreateEndpoint := newAPIEndpoint(http.MethodPost, "/roles")
		roleCreateResponse, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, role, adminAccessTokenHeader)
		assert.NoError(t, err, "Failed to create role")
		defer roleCreateResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, roleCreateResponse.StatusCode, "Failed to create role")

		// 4. Create multiple policies with different actions, including multiple actions on the same resource
		// Using the helper function from api_roles_test.go to create policies
		// Policy 1: GET on /me
		policyGETMe := createTestPolicyWithAction(t, adminToken.AccessToken, "get-me-", "GET", "/me")
		slog.Info("Created GET /me policy", "policy_id", policyGETMe.String())

		// Policy 2: PUT on /me (demonstrates multiple actions on same resource)
		policyPUTMe := createTestPolicyWithAction(t, adminToken.AccessToken, "put-me-", "PUT", "/me")
		slog.Info("Created PUT /me policy", "policy_id", policyPUTMe.String())

		// Policy 3: GET on /users
		policyGETUsers := createTestPolicyWithAction(t, adminToken.AccessToken, "get-users-", "GET", "/users")
		slog.Info("Created GET /users policy", "policy_id", policyGETUsers.String())

		// 5. Link all policies to the role
		rolesLinkPoliciesEndpoint := newAPIEndpoint(http.MethodPost, "/roles/{role_id}/policies")
		rolesLinkPoliciesEndpoint = rolesLinkPoliciesEndpoint.RewriteSlugs(roleID.String())

		linkPayload := map[string]any{
			"policy_ids": []string{
				policyGETMe.String(),
				policyPUTMe.String(),
				policyGETUsers.String(),
			},
		}

		linkResponse, err := sendHTTPRequest(t, ctx, rolesLinkPoliciesEndpoint, linkPayload, adminAccessTokenHeader)
		assert.NoError(t, err, "Failed to link policies to role")
		defer linkResponse.Body.Close()
		assert.Equal(t, http.StatusOK, linkResponse.StatusCode, "Failed to link policies to role")

		// 6. Link the role to the user
		usersLinkRolesEndpoint := newAPIEndpoint(http.MethodPost, "/users/{user_id}/roles")
		usersLinkRolesEndpoint = usersLinkRolesEndpoint.RewriteSlugs(userID.String())

		linkUserRolePayload := map[string]any{
			"role_ids": []string{roleID.String()},
		}

		linkUserRoleResponse, err := sendHTTPRequest(t, ctx, usersLinkRolesEndpoint, linkUserRolePayload, adminAccessTokenHeader)
		assert.NoError(t, err, "Failed to link role to user")
		defer linkUserRoleResponse.Body.Close()
		assert.Equal(t, http.StatusOK, linkUserRoleResponse.StatusCode, "Failed to link role to user")

		// 7. Enable the user to login
		enableUserByEmailFromDB(t, email)

		// 8. Login as the regular user to get their token
		loginRequest := map[string]any{
			"email":    email,
			"password": user["password"],
		}

		loginResponse, err := sendHTTPRequest(t, ctx, authLoginEndpoint, loginRequest)
		assert.NoError(t, err, "Failed to login user")
		defer loginResponse.Body.Close()
		assert.Equal(t, http.StatusOK, loginResponse.StatusCode, "Expected status code 200. Got %d. Message: %s", loginResponse.StatusCode, readResponseBody(t, loginResponse))

		userLoginResponse, err := parserResponseBody[payload.LoginUserResponse](t, loginResponse)
		assert.NoError(t, err, "Failed to parse login response")

		assert.NotEmpty(t, userLoginResponse.AccessToken, "Access token should not be empty")

		userAccessTokenHeader := map[string]string{
			"Authorization": "Bearer " + userLoginResponse.AccessToken,
		}

		// 9. Call the /me/authz endpoint as the regular user
		response, err := sendHTTPRequest(t, ctx, meAuthzEndpoint, nil, userAccessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		// 10. Parse and verify the response
		authzResponse, err := parserResponseBody[payload.GetAuthenticatedUserResponse](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		// 11. Verify the account information matches the regular user
		assert.Equal(t, userID, authzResponse.Account.ID, "User ID should match the authenticated user")
		assert.Equal(t, email, authzResponse.Account.Email, "Email should match")
		assert.Equal(t, firstName, authzResponse.Account.FirstName, "First name should match")
		assert.Equal(t, lastName, authzResponse.Account.LastName, "Last name should match")

		// 12. Verify the permissions structure exists
		assert.NotNil(t, authzResponse.Permissions, "Permissions should not be nil")

		// 13. Navigate the permissions structure: permissions -> users -> {user_id} -> resources -> actions
		//
		// The type assertions this used to need are gone: Permissions is
		// payload.AuthzPermissions now rather than map[string]any, so the shape
		// is checked by the compiler and by json.Unmarshal when the body is
		// parsed. A body that does not match no longer reaches these lines --
		// parserResponseBody fails first, which is a better place to find out.
		permissionsMap := authzResponse.Permissions["users"]
		assert.NotNil(t, permissionsMap, "Expected permissions to have a 'users' key")

		userPermissions := permissionsMap[userID.String()]
		assert.NotNil(t, userPermissions, "Expected permissions.users to have the user ID key")

		// 14. Verify /me resource has both GET and PUT actions (multiple actions on same resource - THIS IS THE KEY TEST)
		meActionsStr := userPermissions["/me"]
		assert.NotEmpty(t, meActionsStr, "Expected /me resource to have at least one action")

		// Verify both GET and PUT are present for /me (THIS PROVES MULTIPLE ACTIONS ON SAME RESOURCE WORK)
		assert.Contains(t, meActionsStr, "GET", "Expected /me resource to have GET action")
		assert.Contains(t, meActionsStr, "PUT", "Expected /me resource to have PUT action")
		assert.Equal(t, 2, len(meActionsStr), "Expected /me resource to have exactly 2 actions (GET and PUT)")

		// 15. Verify /users resource has GET action
		usersActionsStr := userPermissions["/users"]
		assert.NotEmpty(t, usersActionsStr, "Expected /users resource to have at least one action")

		assert.Contains(t, usersActionsStr, "GET", "Expected /users resource to have GET action")

		// 16. Verify we have at least the 2 resources we created (may have more from system roles)
		assert.GreaterOrEqual(t, len(userPermissions), 2, "Expected at least 2 resources in permissions: /me, /users")

		// 17. Log the permissions structure for debugging
		slog.Info("✅ User permissions verified - multiple actions on same resource works correctly!",
			"user_id", userID.String(),
			"resources_count", len(userPermissions),
			"/me_actions", meActionsStr,
			"/users_actions", usersActionsStr,
			"KEY_TEST_PASSED", "Multiple actions (GET and PUT) correctly aggregated on /me resource")

		// 18. Cleanup - Register in reverse order (LIFO execution)
		t.Cleanup(func() { deleteUserByIDFromDB(t, adminToken.UserID) })
		t.Cleanup(func() { deletePolicyByIDFromDB(t, policyGETUsers) })
		t.Cleanup(func() { deletePolicyByIDFromDB(t, policyPUTMe) })
		t.Cleanup(func() { deletePolicyByIDFromDB(t, policyGETMe) })
		t.Cleanup(func() { deleteRoleByIDFromDB(t, roleID) })
		t.Cleanup(func() { deleteUserByIDFromDB(t, userID) })
		// Unlink relationships - runs FIRST due to LIFO
		t.Cleanup(func() { unlinkPoliciesFromRoleViaDB(t, roleID, []uuid.UUID{policyGETMe, policyPUTMe, policyGETUsers}) })
		t.Cleanup(func() { unlinkRolesFromUserViaDB(t, userID, []uuid.UUID{roleID}) })
	})

	// Test authz endpoint with expired token simulation (invalid token)
	t.Run("expired_or_malformed_bearer_token", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		testCases := []struct {
			name        string
			authHeader  map[string]string
			description string
		}{
			{
				name: "Missing Bearer prefix",
				authHeader: map[string]string{
					"Authorization": "InvalidTokenHere",
				},
				description: "Authorization header without Bearer prefix",
			},
			{
				name: "Empty token after Bearer",
				authHeader: map[string]string{
					"Authorization": "Bearer ",
				},
				description: "Authorization header with empty token",
			},
			{
				name: "Malformed JWT token",
				authHeader: map[string]string{
					"Authorization": "Bearer not.a.valid.jwt.token.structure",
				},
				description: "Authorization header with malformed JWT",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				response, err := sendHTTPRequest(t, ctx, meAuthzEndpoint, nil, tc.authHeader)
				assert.NoError(t, err, "Failed to send request for %s", tc.name)
				defer response.Body.Close()

				// Should get 401 Unauthorized for all invalid token cases
				assert.Equal(t, http.StatusUnauthorized, response.StatusCode,
					"Expected status code 401 for %s, got %d", tc.name, response.StatusCode)
			})
		}
	})
}

func TestMeGet(t *testing.T) {
	// Test getting authenticated user information
	t.Run("get_authenticated_user", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Call the /me endpoint
		response, err := sendHTTPRequest(t, ctx, meGetEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		// 3. Parse and verify the response
		userResponse, err := parserResponseBody[payload.UserResponse](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		// 4. Verify the user information
		assert.Equal(t, adminToken.UserID, userResponse.ID, "User ID should match the authenticated user")
		assert.NotEmpty(t, userResponse.Email, "Email should not be empty")
		assert.NotEmpty(t, userResponse.FirstName, "First name should not be empty")
		assert.NotEmpty(t, userResponse.LastName, "Last name should not be empty")
		assert.NotZero(t, userResponse.CreatedAt, "Created at should not be zero")
		assert.NotZero(t, userResponse.UpdatedAt, "Updated at should not be zero")

		// 5. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// The GET /me and PUT /me policies were seeded from the start and linked to
	// no role, so this case was commented out with "ISSUE DETECTED" instead of
	// failing. The seed links them to AuthenticatedUser now.
	t.Run("get_authenticated_user_with_regular_user", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token to create another user
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		adminAccessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a regular user
		firstName, lastName, email := generateUserData(t)
		userID := uuid.NewV7()

		user := map[string]any{
			"id":         userID.String(),
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   generatePassword(t),
		}

		userCreateResponse, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, user, adminAccessTokenHeader)
		assert.NoError(t, err, "Failed to create user")
		defer userCreateResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, userCreateResponse.StatusCode, "Expected status code 201. Got %d. Message: %s", userCreateResponse.StatusCode, readResponseBody(t, userCreateResponse))

		// 3. enable the user to login in db
		enableUserByEmailFromDB(t, email)

		// 4. Login as the regular user to get their token
		loginRequest := map[string]any{
			"email":    email,
			"password": user["password"],
		}

		loginResponse, err := sendHTTPRequest(t, ctx, authLoginEndpoint, loginRequest)
		assert.NoError(t, err, "Failed to login user")
		defer loginResponse.Body.Close()
		assert.Equal(t, http.StatusOK, loginResponse.StatusCode, "Expected status code 200. Got %d. Message: %s", loginResponse.StatusCode, readResponseBody(t, loginResponse))

		userLoginResponse, err := parserResponseBody[payload.LoginUserResponse](t, loginResponse)
		assert.NoError(t, err, "Failed to parse login response")

		assert.NotEmpty(t, userLoginResponse.AccessToken, "Access token should not be empty")

		if userLoginResponse.AccessToken == "" {
			t.Fatal("Access token should not be empty after login")
		}

		userAccessTokenHeader := map[string]string{
			"Authorization": "Bearer " + userLoginResponse.AccessToken,
		}

		// 5. Call the /me endpoint as the regular user
		response, err := sendHTTPRequest(t, ctx, meGetEndpoint, nil, userAccessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		// 6. Parse and verify the response
		userResponse, err := parserResponseBody[payload.UserResponse](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		// 7. Verify the user information matches the regular user
		assert.Equal(t, userID, userResponse.ID, "User ID should match the authenticated user")
		assert.Equal(t, email, userResponse.Email, "Email should match")
		assert.Equal(t, firstName, userResponse.FirstName, "First name should match")
		assert.Equal(t, lastName, userResponse.LastName, "Last name should match")
		assert.NotZero(t, userResponse.CreatedAt, "Created at should not be zero")
		assert.NotZero(t, userResponse.UpdatedAt, "Updated at should not be zero")

		// 8. Verify the user is not an admin
		assert.NotNil(t, userResponse.Admin, "Admin field should not be nil")
		assert.False(t, *userResponse.Admin, "Regular user should not be an admin")

		// 9. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, userID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("require_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Try to call /me without authentication
		response, err := sendHTTPRequest(t, ctx, meGetEndpoint, nil)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 2. Check that we get a 401 Unauthorized response (middleware handles missing auth)
		assert.Equal(t, http.StatusUnauthorized, response.StatusCode, "Expected status code 401. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})

	t.Run("invalid_authentication_token", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Try to call /me with an invalid token
		invalidTokenHeader := map[string]string{
			"Authorization": "Bearer invalid-token-here",
		}

		response, err := sendHTTPRequest(t, ctx, meGetEndpoint, nil, invalidTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 2. Check that we get a 401 Unauthorized response (middleware handles invalid auth)
		assert.Equal(t, http.StatusUnauthorized, response.StatusCode, "Expected status code 401. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})

	t.Run("user_not_found", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		// 2. Delete the user from the database to simulate a deleted user
		deleteUserByIDFromDB(t, adminToken.UserID)

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 3. Try to call /me with the token of a deleted user
		response, err := sendHTTPRequest(t, ctx, meGetEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 4. Check that we get a 404 Not Found response (middleware handles auth before handler)
		assert.Equal(t, http.StatusNotFound, response.StatusCode, "Expected status code 404. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		// 5. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse error response")

		// Verify error message contains information about user not found
		assert.Contains(t, strings.ToLower(errorResp.Message), "not found",
			"Error message should indicate user not found")
	})

	// Test response structure and field types
	t.Run("verify_response_structure", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Call the /me endpoint
		response, err := sendHTTPRequest(t, ctx, meGetEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		// 3. Parse and verify the response structure
		userResponse, err := parserResponseBody[payload.UserResponse](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		// 4. Verify all required fields are present and non-zero
		assert.NotEqual(t, uuid.Nil(), userResponse.ID, "User ID should not be nil UUID")
		assert.NotEmpty(t, userResponse.Email, "Email should not be empty")
		assert.NotEmpty(t, userResponse.FirstName, "First name should not be empty")
		assert.NotEmpty(t, userResponse.LastName, "Last name should not be empty")
		assert.NotZero(t, userResponse.CreatedAt, "Created at should not be zero")
		assert.NotZero(t, userResponse.UpdatedAt, "Updated at should not be zero")

		// 5. Verify boolean fields have valid values (they are pointers)
		assert.NotNil(t, userResponse.Disabled, "Disabled should not be nil")
		assert.NotNil(t, userResponse.Admin, "Admin should not be nil")
		assert.NotNil(t, userResponse.LocalAccount, "LocalAccount should not be nil")
		assert.IsType(t, new(bool), userResponse.Disabled, "Disabled should be a boolean pointer")
		assert.IsType(t, new(bool), userResponse.Admin, "Admin should be a boolean pointer")
		assert.IsType(t, new(bool), userResponse.LocalAccount, "LocalAccount should be a boolean pointer")

		// 6. Verify timestamps are in correct order
		assert.True(t, userResponse.UpdatedAt.After(userResponse.CreatedAt) || userResponse.UpdatedAt.Equal(userResponse.CreatedAt),
			"Updated at should be after or equal to created at")

		// 7. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test disabled user cannot access endpoint
	t.Run("disabled_user_cannot_access", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token to create another user
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		adminAccessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a regular user
		firstName, lastName, email := generateUserData(t)
		userID := uuid.NewV7()

		user := map[string]any{
			"id":         userID.String(),
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   generatePassword(t),
		}

		userCreateResponse, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, user, adminAccessTokenHeader)
		assert.NoError(t, err, "Failed to create user")
		defer userCreateResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, userCreateResponse.StatusCode, "Expected status code 201")

		// 3. Enable the user first to get a token
		enableUserByEmailFromDB(t, email)

		// 4. Login to get a valid token
		loginRequest := map[string]any{
			"email":    email,
			"password": user["password"],
		}

		loginResponse, err := sendHTTPRequest(t, ctx, authLoginEndpoint, loginRequest)
		assert.NoError(t, err, "Failed to login user")
		defer loginResponse.Body.Close()

		userLoginResponse, err := parserResponseBody[payload.LoginUserResponse](t, loginResponse)
		assert.NoError(t, err, "Failed to parse login response")

		userAccessTokenHeader := map[string]string{
			"Authorization": "Bearer " + userLoginResponse.AccessToken,
		}

		// 5. Now disable the user in the database (simulating admin disabling the user)
		query := `UPDATE users SET disabled = true WHERE id = $1;`
		_, err = testDBPool.Exec(context.Background(), query, userID)
		assert.NoError(t, err, "Failed to disable user in database")

		// 6. Try to access /me with the disabled user's token
		response, err := sendHTTPRequest(t, ctx, meGetEndpoint, nil, userAccessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// The middleware should reject disabled users
		// Note: This might return 401 or 403 depending on your middleware implementation
		assert.True(t, response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden,
			"Disabled user should not be able to access the endpoint. Got status %d", response.StatusCode)

		// 7. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, userID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test response headers
	t.Run("verify_response_headers", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Call the /me endpoint
		response, err := sendHTTPRequest(t, ctx, meGetEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		// 3. Verify Content-Type header
		contentType := response.Header.Get("Content-Type")
		assert.Contains(t, contentType, "application/json", "Content-Type should be application/json")

		// 4. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}

func TestMeUpdate(t *testing.T) {
	// Test updating authenticated user information
	t.Run("update_authenticated_user", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Update the user information
		newFirstName := "UpdatedFirstName"
		newLastName := "UpdatedLastName"
		updateRequest := map[string]any{
			"first_name": newFirstName,
			"last_name":  newLastName,
		}

		response, err := sendHTTPRequest(t, ctx, meUpdateEndpoint, updateRequest, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		// 3. Parse and verify the response
		updateResponse, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		assert.Equal(t, domain.UsersUserUpdatedSuccessfully, updateResponse.Message, "Expected success message")
		assert.Equal(t, meUpdateEndpoint.method, updateResponse.Method, "Expected method to be set")
		assert.Equal(t, meUpdateEndpoint.Path(), updateResponse.Path, "Expected path to be set")

		// 4. Verify the update by getting the user information
		getUserResponse, err := sendHTTPRequest(t, ctx, meGetEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to get updated user")
		defer getUserResponse.Body.Close()

		assert.Equal(t, http.StatusOK, getUserResponse.StatusCode, "Expected status code 200 when getting updated user")

		updatedUser, err := parserResponseBody[payload.UserResponse](t, getUserResponse)
		assert.NoError(t, err, "Failed to parse updated user response")

		assert.Equal(t, newFirstName, updatedUser.FirstName, "First name should be updated")
		assert.Equal(t, newLastName, updatedUser.LastName, "Last name should be updated")

		// 5. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("update_authenticated_user_password", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Update the user password
		newPassword := generatePassword(t)
		updateRequest := map[string]any{
			"password": newPassword,
		}

		response, err := sendHTTPRequest(t, ctx, meUpdateEndpoint, updateRequest, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		// 3. Parse and verify the response
		updateResponse, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		assert.Equal(t, domain.UsersUserUpdatedSuccessfully, updateResponse.Message, "Expected success message")

		// 4. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("update_with_invalid_data", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Define test cases for invalid data
		testCases := []struct {
			name          string
			updateData    map[string]any
			expectedError string
		}{
			{
				name: "Empty first name",
				updateData: map[string]any{
					"first_name": "",
				},
				expectedError: "cannot be empty",
			},
			{
				name: "Empty last name",
				updateData: map[string]any{
					"last_name": "",
				},
				expectedError: "cannot be empty",
			},
			{
				name: "Password too short",
				updateData: map[string]any{
					"password": "short",
				},
				expectedError: "password must be at least",
			},
			{
				name: "Password too long",
				updateData: map[string]any{
					"password": strings.Repeat("A1!a", 70), // Very long password
				},
				expectedError: "password must be at most",
			},
			{
				name: "First name too long",
				updateData: map[string]any{
					"first_name": string(make([]byte, 300)), // Very long first name
				},
				expectedError: "must be between",
			},
			{
				name: "Last name too long",
				updateData: map[string]any{
					"last_name": string(make([]byte, 300)), // Very long last name
				},
				expectedError: "must be between",
			},
		}

		// 3. Run each test case
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Send request with invalid data
				response, err := sendHTTPRequest(t, ctx, meUpdateEndpoint, tc.updateData, accessTokenHeader)
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
			})
		}

		// 4. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("update_with_empty_request", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Send empty update request
		emptyRequest := map[string]any{}

		response, err := sendHTTPRequest(t, ctx, meUpdateEndpoint, emptyRequest, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 3. Check that we get a 400 Bad Request response
		assert.Equal(t, http.StatusBadRequest, response.StatusCode, "Expected status code 400. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		// 4. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse error response")

		// Verify error message indicates that at least one field must be updated
		assert.Contains(t, strings.ToLower(errorResp.Message), "at least one field must be updated",
			"Error message should indicate that at least one field must be updated")

		// 5. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("require_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Try to update without authentication
		updateRequest := map[string]any{
			"first_name": "NewName",
		}

		response, err := sendHTTPRequest(t, ctx, meUpdateEndpoint, updateRequest)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 2. Check that we get a 401 Unauthorized response (middleware handles missing auth)
		assert.Equal(t, http.StatusUnauthorized, response.StatusCode, "Expected status code 401. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})

	t.Run("invalid_authentication_token", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Try to update with an invalid token
		invalidTokenHeader := map[string]string{
			"Authorization": "Bearer invalid-token-here",
		}

		updateRequest := map[string]any{
			"first_name": "NewName",
		}

		response, err := sendHTTPRequest(t, ctx, meUpdateEndpoint, updateRequest, invalidTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 2. Check that we get a 401 Unauthorized response (middleware handles invalid auth)
		assert.Equal(t, http.StatusUnauthorized, response.StatusCode, "Expected status code 401. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})

	t.Run("user_not_found", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		// 2. Delete the user from the database to simulate a deleted user
		deleteUserByIDFromDB(t, adminToken.UserID)

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 3. Try to update the deleted user
		updateRequest := map[string]any{
			"first_name": "NewName",
		}

		response, err := sendHTTPRequest(t, ctx, meUpdateEndpoint, updateRequest, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 4. Check that we get a 404 Not Found response (middleware handles auth before handler)
		assert.Equal(t, http.StatusNotFound, response.StatusCode, "Expected status code 404. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		// 5. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse error response")

		// Verify error message contains information about not found access
		assert.Contains(t, strings.ToLower(errorResp.Message), "not found",
			"Error message should indicate user not found")
	})

	t.Run("invalid_json_request", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Send a request with invalid data that will cause JSON parsing error
		// Using a non-string value for first_name should cause validation error
		invalidRequest := map[string]any{
			"first_name": 12345, // Invalid type - should be string
		}

		response, err := sendHTTPRequest(t, ctx, meUpdateEndpoint, invalidRequest, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 3. Check that we get a 400 Bad Request response
		assert.Equal(t, http.StatusBadRequest, response.StatusCode, "Expected status code 400. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		// 4. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test updating only the first name
	t.Run("update_only_first_name", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Get the current user information
		getUserResponse, err := sendHTTPRequest(t, ctx, meGetEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to get user")
		defer getUserResponse.Body.Close()
		assert.Equal(t, http.StatusOK, getUserResponse.StatusCode, "Expected status code 200")

		originalUser, err := parserResponseBody[payload.UserResponse](t, getUserResponse)
		assert.NoError(t, err, "Failed to parse user response")

		// 3. Update only the first name
		newFirstName := "UpdatedOnlyFirstName"
		updateRequest := map[string]any{
			"first_name": newFirstName,
		}

		response, err := sendHTTPRequest(t, ctx, meUpdateEndpoint, updateRequest, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		// 4. Verify the update
		getUserResponse2, err := sendHTTPRequest(t, ctx, meGetEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to get updated user")
		defer getUserResponse2.Body.Close()

		updatedUser, err := parserResponseBody[payload.UserResponse](t, getUserResponse2)
		assert.NoError(t, err, "Failed to parse updated user response")

		assert.Equal(t, newFirstName, updatedUser.FirstName, "First name should be updated")
		assert.Equal(t, originalUser.LastName, updatedUser.LastName, "Last name should remain unchanged")
		assert.Equal(t, originalUser.Email, updatedUser.Email, "Email should remain unchanged")

		// 5. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test updating only the last name
	t.Run("update_only_last_name", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Get the current user information
		getUserResponse, err := sendHTTPRequest(t, ctx, meGetEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to get user")
		defer getUserResponse.Body.Close()

		originalUser, err := parserResponseBody[payload.UserResponse](t, getUserResponse)
		assert.NoError(t, err, "Failed to parse user response")

		// 3. Update only the last name
		newLastName := "UpdatedOnlyLastName"
		updateRequest := map[string]any{
			"last_name": newLastName,
		}

		response, err := sendHTTPRequest(t, ctx, meUpdateEndpoint, updateRequest, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		// 4. Verify the update
		getUserResponse2, err := sendHTTPRequest(t, ctx, meGetEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to get updated user")
		defer getUserResponse2.Body.Close()

		updatedUser, err := parserResponseBody[payload.UserResponse](t, getUserResponse2)
		assert.NoError(t, err, "Failed to parse updated user response")

		assert.Equal(t, originalUser.FirstName, updatedUser.FirstName, "First name should remain unchanged")
		assert.Equal(t, newLastName, updatedUser.LastName, "Last name should be updated")
		assert.Equal(t, originalUser.Email, updatedUser.Email, "Email should remain unchanged")

		// 5. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test updating all fields at once
	t.Run("update_all_fields", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Update all fields
		newFirstName := "AllFieldsFirst"
		newLastName := "AllFieldsLast"
		newPassword := generatePassword(t)

		updateRequest := map[string]any{
			"first_name": newFirstName,
			"last_name":  newLastName,
			"password":   newPassword,
		}

		response, err := sendHTTPRequest(t, ctx, meUpdateEndpoint, updateRequest, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		// 3. Verify the update by getting the user information
		getUserResponse, err := sendHTTPRequest(t, ctx, meGetEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to get updated user")
		defer getUserResponse.Body.Close()

		updatedUser, err := parserResponseBody[payload.UserResponse](t, getUserResponse)
		assert.NoError(t, err, "Failed to parse updated user response")

		assert.Equal(t, newFirstName, updatedUser.FirstName, "First name should be updated")
		assert.Equal(t, newLastName, updatedUser.LastName, "Last name should be updated")

		// 4. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test updating with valid boundary values
	t.Run("update_with_boundary_values", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		testCases := []struct {
			name        string
			updateData  map[string]any
			description string
		}{
			{
				name: "Minimum length first name",
				updateData: map[string]any{
					"first_name": "Ab", // minimum is 2 characters
				},
				description: "First name with minimum allowed length",
			},
			{
				name: "Maximum length first name",
				updateData: map[string]any{
					"first_name": strings.Repeat("A", 25), // maximum is 25 characters
				},
				description: "First name with maximum allowed length",
			},
			{
				name: "Minimum length last name",
				updateData: map[string]any{
					"last_name": "Ab", // minimum is 2 characters
				},
				description: "Last name with minimum allowed length",
			},
			{
				name: "Maximum length last name",
				updateData: map[string]any{
					"last_name": strings.Repeat("B", 25), // maximum is 25 characters
				},
				description: "Last name with maximum allowed length",
			},
			{
				name: "First and last name with special characters",
				updateData: map[string]any{
					"first_name": "Jean-Pierre",
					"last_name":  "O'Connor",
				},
				description: "Names with hyphens and apostrophes",
			},
			{
				name: "Names with unicode characters",
				updateData: map[string]any{
					"first_name": "José",
					"last_name":  "Müller",
				},
				description: "Names with accented characters",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				response, err := sendHTTPRequest(t, ctx, meUpdateEndpoint, tc.updateData, accessTokenHeader)
				assert.NoError(t, err, "Failed to send request for %s", tc.name)
				defer response.Body.Close()

				// These should all succeed as they're valid boundary cases
				assert.Equal(t, http.StatusOK, response.StatusCode,
					"Expected status code 200 for %s, got %d. Message: %s",
					tc.name, response.StatusCode, readResponseBody(t, response))
			})
		}

		// 2. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test malformed/edge case request bodies
	t.Run("update_with_malformed_requests", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		testCases := []struct {
			name        string
			updateData  map[string]any
			expectedMsg string
		}{
			{
				name: "Null first name",
				updateData: map[string]any{
					"first_name": nil,
				},
				expectedMsg: "at least one field must be updated",
			},
			{
				name: "Null last name",
				updateData: map[string]any{
					"last_name": nil,
				},
				expectedMsg: "at least one field must be updated",
			},
			{
				name: "Null password",
				updateData: map[string]any{
					"password": nil,
				},
				expectedMsg: "at least one field must be updated",
			},
			{
				name: "Whitespace-only first name",
				updateData: map[string]any{
					"first_name": "   ",
				},
				expectedMsg: "cannot be empty",
			},
			{
				name: "Whitespace-only last name",
				updateData: map[string]any{
					"last_name": "   ",
				},
				expectedMsg: "cannot be empty",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				response, err := sendHTTPRequest(t, ctx, meUpdateEndpoint, tc.updateData, accessTokenHeader)
				assert.NoError(t, err, "Failed to send request for %s", tc.name)
				defer response.Body.Close()

				assert.Equal(t, http.StatusBadRequest, response.StatusCode,
					"Expected status code 400 for %s, got %d", tc.name, response.StatusCode)

				errorResp, err := parserResponseBody[payload.HTTPMessage](t, response)
				assert.NoError(t, err, "Failed to parse error response for %s", tc.name)

				assert.Contains(t, strings.ToLower(errorResp.Message), strings.ToLower(tc.expectedMsg),
					"Error message should contain '%s' for %s", tc.expectedMsg, tc.name)
			})
		}

		// 2. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test password validation edge cases
	t.Run("update_password_edge_cases", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		testCases := []struct {
			name             string
			password         string
			shouldSucceed    bool
			expectedErrorMsg string
		}{
			{
				name:          "Valid password with special characters",
				password:      "P@ssw0rd!#$%",
				shouldSucceed: true,
			},
			{
				name:          "Valid password with minimum length",
				password:      "Pass123!",
				shouldSucceed: true,
			},
			{
				name:             "Password with only lowercase",
				password:         "passwordonly",
				shouldSucceed:    false,
				expectedErrorMsg: "password must contain",
			},
			{
				name:             "Password with only uppercase",
				password:         "PASSWORDONLY",
				shouldSucceed:    false,
				expectedErrorMsg: "password must contain",
			},
			{
				name:             "Password with only numbers",
				password:         "12345678901",
				shouldSucceed:    false,
				expectedErrorMsg: "password must contain",
			},
			{
				name:             "Password without digits but with special chars",
				password:         "PasswordOnly!",
				shouldSucceed:    true, // Has upper, lower, and special = score 3 (passes)
				expectedErrorMsg: "",
			},
			{
				name:             "Password without special chars or digits but long",
				password:         "PasswordOnlyLetters", // Has upper+lower+length bonus = score 3 (passes)
				shouldSucceed:    true,
				expectedErrorMsg: "",
			},
			{
				name:             "Password with only uppercase and lowercase (short)",
				password:         "AbcdEfgh", // Only upper+lower = score 2 (fails)
				shouldSucceed:    false,
				expectedErrorMsg: "password must contain",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				updateRequest := map[string]any{
					"password": tc.password,
				}

				response, err := sendHTTPRequest(t, ctx, meUpdateEndpoint, updateRequest, accessTokenHeader)
				assert.NoError(t, err, "Failed to send request for %s", tc.name)
				defer response.Body.Close()

				if tc.shouldSucceed {
					assert.Equal(t, http.StatusOK, response.StatusCode,
						"Expected status code 200 for %s, got %d. Message: %s",
						tc.name, response.StatusCode, readResponseBody(t, response))
				} else {
					assert.Equal(t, http.StatusBadRequest, response.StatusCode,
						"Expected status code 400 for %s, got %d", tc.name, response.StatusCode)

					errorResp, err := parserResponseBody[payload.HTTPMessage](t, response)
					assert.NoError(t, err, "Failed to parse error response for %s", tc.name)

					assert.Contains(t, strings.ToLower(errorResp.Message), strings.ToLower(tc.expectedErrorMsg),
						"Error message should contain '%s' for %s", tc.expectedErrorMsg, tc.name)
				}
			})
		}

		// 2. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}

func TestMeJSONSerialization(t *testing.T) {
	// Test that all /me endpoints return JSON with snake_case field names
	t.Run("get_me_response_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// Get /me endpoint
		response, err := sendHTTPRequest(t, ctx, meGetEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		// Verify all JSON fields use snake_case
		expectedFields := []string{"id", "email", "first_name", "last_name", "disabled", "admin", "local_account", "created_at", "updated_at"}
		assertJSONFieldsAreSnakeCase(t, response, expectedFields)

		// Parse and verify specific values
		meResp, err := parserResponseBody[payload.UserResponse](t, response)
		assert.NoError(t, err, "Failed to parse response body")
		assert.Equal(t, adminToken.UserID, meResp.ID)

		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("get_me_authz_response_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// Get /me/authz endpoint
		response, err := sendHTTPRequest(t, ctx, meAuthzEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		// Verify all JSON fields use snake_case
		expectedFields := []string{"account", "permissions"}
		assertJSONFieldsAreSnakeCase(t, response, expectedFields)

		// Parse and verify specific values
		authzResp, err := parserResponseBody[payload.GetAuthenticatedUserResponse](t, response)
		assert.NoError(t, err, "Failed to parse response body")
		assert.Equal(t, adminToken.UserID, authzResp.Account.ID)
		assert.NotNil(t, authzResp.Permissions)

		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("update_me_response_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// Update /me endpoint
		updateRequest := map[string]any{
			"first_name": "UpdatedFirstName",
			"last_name":  "UpdatedLastName",
		}

		response, err := sendHTTPRequest(t, ctx, meUpdateEndpoint, updateRequest, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		// Verify all JSON fields use snake_case
		expectedFields := []string{"message", "method", "path", "status_code"}
		assertJSONFieldsAreSnakeCase(t, response, expectedFields)

		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}
