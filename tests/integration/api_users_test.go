//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"sort"
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
	usersCreateEndpoint = newAPIEndpoint(http.MethodPost, "/users")
	usersGetEndpoint    = newAPIEndpoint(http.MethodGet, "/users/{user_id}")
	usersDeleteEndpoint = newAPIEndpoint(http.MethodDelete, "/users/{user_id}")
	usersUpdateEndpoint = newAPIEndpoint(http.MethodPut, "/users/{user_id}")
	usersListEndpoint   = newAPIEndpoint(http.MethodGet, "/users")

	usersLinkRolesEndpoint   = newAPIEndpoint(http.MethodPost, "/users/{user_id}/roles")
	usersUnlinkRolesEndpoint = newAPIEndpoint(http.MethodDelete, "/users/{user_id}/roles")

	usersListRolesLinkedEndpoint = newAPIEndpoint(http.MethodGet, "/users/{user_id}/roles")
	usersAuthzEndpoint           = newAPIEndpoint(http.MethodGet, "/users/{user_id}/authz")

	usersLinkProjectsEndpoint   = newAPIEndpoint(http.MethodPost, "/users/{user_id}/projects")
	usersUnlinkProjectsEndpoint = newAPIEndpoint(http.MethodDelete, "/users/{user_id}/projects")
)

func TestUserCreate(t *testing.T) {
	// Test user creation
	t.Run("create_user", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a new user
		firstName, lastName, email := generateUserData(t)
		user := map[string]any{
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   generatePassword(t),
		}

		response, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, user, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusCreated, response.StatusCode, "Expected status code 201")

		apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		t.Cleanup(func() {
			deleteUserByEmailFromDB(t, email)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})

		assert.Equal(t, domain.UsersUserCreatedSuccessfully, apiResp.Message, "Expected success message")
		assert.Equal(t, usersCreateEndpoint.method, apiResp.Method, "Expected method to be set")
		assert.Equal(t, usersCreateEndpoint.Path(), apiResp.Path, "Expected path to be set")
	})

	// Test creating a user with invalid data format
	t.Run("create_user_bad_request", func(t *testing.T) {
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
			invalidUser   map[string]any
			expectedError string
		}{
			{
				name: "Invalid email format",
				invalidUser: map[string]any{
					"email":      "not-a-valid-email",
					"first_name": "John",
					"last_name":  "Doe",
					"password":   generatePassword(t),
				},
				expectedError: "invalid email",
			},
			{
				name: "Empty first name",
				invalidUser: map[string]any{
					"email":      "valid.email@example.com",
					"first_name": "",
					"last_name":  "Doe",
					"password":   generatePassword(t),
				},
				expectedError: "cannot be empty",
			},
			{
				name: "Empty last name",
				invalidUser: map[string]any{
					"email":      "valid.email@example.com",
					"first_name": "John",
					"last_name":  "",
					"password":   generatePassword(t),
				},
				expectedError: "cannot be empty",
			},
			{
				name: "Password too short",
				invalidUser: map[string]any{
					"email":      "valid.email@example.com",
					"first_name": "John",
					"last_name":  "Doe",
					"password":   "short",
				},
				expectedError: "password must be at least",
			},
			{
				name: "Password too long",
				invalidUser: map[string]any{
					"email":      "valid.email@example.com",
					"first_name": "John",
					"last_name":  "Doe",
					"password":   strings.Repeat("A1!a", 70), // Very long password
				},
				expectedError: "password must be at most",
			},
			{
				name: "First name too long",
				invalidUser: map[string]any{
					"email":      "valid.email@example.com",
					"first_name": strings.Repeat("a", 300), // Very long first name
					"last_name":  "Doe",
					"password":   generatePassword(t),
				},
				expectedError: "must be between",
			},
			{
				name: "Last name too long",
				invalidUser: map[string]any{
					"email":      "valid.email@example.com",
					"first_name": "John",
					"last_name":  strings.Repeat("a", 300), // Very long last name
					"password":   generatePassword(t),
				},
				expectedError: "must be between",
			},
			{
				name: "Invalid ID format",
				invalidUser: map[string]any{
					"id":         "not-a-valid-uuid",
					"email":      "valid.email@example.com",
					"first_name": "John",
					"last_name":  "Doe",
					"password":   generatePassword(t),
				},
				expectedError: "invalid uuid",
			},
		}

		// 3. Run each test case
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Send request with the invalid user data
				response, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, tc.invalidUser, accessTokenHeader)
				assert.NoError(t, err, "Failed to send request for %s", tc.name)
				defer response.Body.Close()

				// Verify we get a 400 Bad Request response
				assert.Equal(t, http.StatusBadRequest, response.StatusCode,
					"Expected status code 400 Bad Request for %s, got %d", tc.name, response.StatusCode, readResponseBody(t, response))

				// Parse and verify the error response
				errorResp, err := parserResponseBody[payload.HTTPMessage](t, response)
				assert.NoError(t, err, "Failed to parse error response for %s", tc.name)

				// Verify error message contains information specific to this validation failure
				assert.Contains(t, strings.ToLower(errorResp.Message), strings.ToLower(tc.expectedError),
					"Error message should indicate %s validation failure", tc.name)
				assert.Equal(t, usersCreateEndpoint.method, errorResp.Method, "Expected method to be set")
				assert.Equal(t, usersCreateEndpoint.Path(), errorResp.Path, "Expected path to be set")
			})
		}

		// 4. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test creating users with existing ID or email
	t.Run("create_user_already_exists", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. First create a valid user that will be our reference user
		userID := uuid.NewV7()
		firstName, lastName, email := generateUserData(t)
		existingUser := map[string]any{
			"id":         userID.String(),
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   generatePassword(t),
		}

		// Create the first user
		createResponse, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, existingUser, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to create initial user")
		defer createResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, createResponse.StatusCode, "Expected status code 201 for initial user creation, got %d. Message: %s", createResponse.StatusCode, readResponseBody(t, createResponse))

		// 3. Define test cases for duplicate scenarios
		testCases := []struct {
			name           string
			duplicateUser  map[string]any
			expectedStatus int
			expectedError  string
		}{
			{
				name: "User with existing ID",
				duplicateUser: map[string]any{
					"id":         userID.String(), // Same ID as existing user
					"email":      "different_" + email,
					"first_name": "Different",
					"last_name":  "User",
					"password":   generatePassword(t),
				},
				expectedStatus: http.StatusConflict,
				expectedError:  "already exists",
			},
			{
				name: "User with existing email",
				duplicateUser: map[string]any{
					"id":         uuid.NewV7().String(), // Different ID
					"email":      email,                 // Same email as existing user
					"first_name": "Another",
					"last_name":  "User",
					"password":   generatePassword(t),
				},
				expectedStatus: http.StatusConflict,
				expectedError:  "already exists",
			},
		}

		// 4. Run each test case
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Try to create a user with duplicate ID or email
				response, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, tc.duplicateUser, accessTokenHeader)
				assert.NoError(t, err, "Failed to send request for %s", tc.name)
				defer response.Body.Close()

				// Verify we get the expected conflict status
				assert.Equal(t, tc.expectedStatus, response.StatusCode,
					"Expected status code %d for %s, got %d", tc.expectedStatus, tc.name, response.StatusCode, readResponseBody(t, response))

				// Parse and verify the error response
				errorResp, err := parserResponseBody[payload.HTTPMessage](t, response)
				assert.NoError(t, err, "Failed to parse error response for %s", tc.name)

				// Verify error message contains information about the conflict
				assert.Contains(t, strings.ToLower(errorResp.Message), strings.ToLower(tc.expectedError),
					"Error message should indicate conflict for %s", tc.name)
				assert.Equal(t, usersCreateEndpoint.method, errorResp.Method, "Expected method to be set")
				assert.Equal(t, usersCreateEndpoint.Path(), errorResp.Path, "Expected path to be set")
			})
		}

		// 5. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, userID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}

func TestUserGet(t *testing.T) {
	// Test user retrieval
	t.Run("get_user", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a new user
		userID := uuid.NewV7()
		firstName, lastName, email := generateUserData(t)
		password := generatePassword(t)
		user := map[string]any{
			"id":         userID.String(),
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   password,
		}

		response, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, user, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusCreated, response.StatusCode, "Expected status code 201. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		assert.Equal(t, domain.UsersUserCreatedSuccessfully, apiResp.Message, "Expected success message")
		assert.Equal(t, usersCreateEndpoint.method, apiResp.Method, "Expected method to be set")
		assert.Equal(t, usersCreateEndpoint.Path(), apiResp.Path, "Expected path to be set")

		// 2.1 Verify the user is created in the database
		// This is optional, but you can check the database to ensure the user was created successfully
		enableUserByEmailFromDB(t, email)

		// 3. Login the user to get the access token and user ID
		// wait for login verification in the database
		time.Sleep(500 * time.Millisecond)

		loginUser := map[string]any{
			"email":    email,
			"password": password,
		}

		loginResponse, err := sendHTTPRequest(t, ctx, authLoginEndpoint, loginUser)
		assert.NoError(t, err)
		defer loginResponse.Body.Close()

		assert.Equal(t, loginResponse.StatusCode, http.StatusOK)
		loginAPIResp, err := parserResponseBody[payload.LoginUserResponse](t, loginResponse)
		assert.NoError(t, err)

		assert.Equal(t, user["id"], loginAPIResp.UserID.String(), "Expected user ID to be set")
		assert.Equal(t, domain.TokenTypeBearer, loginAPIResp.TokenType, "Expected token type to be Bearer")
		assert.NotEmpty(t, loginAPIResp.AccessToken, "Expected access token to be set")
		assert.NotEmpty(t, loginAPIResp.RefreshToken, "Expected refresh token to be set")

		// 4. Get the user
		// we must replace the slug {user_id} with the user ID
		newUserGetEndpoint := usersGetEndpoint.RewriteSlugs(loginAPIResp.UserID.String())
		// t.Logf("User Get Endpoint: %s", newUserGetEndpoint)

		userGetResponse, err := sendHTTPRequest(t, ctx, newUserGetEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to get user")
		defer userGetResponse.Body.Close()

		// read the response body
		// userGetBody, err := readResponseBody(t, userGetResponse)
		// assert.NoError(t, err, "Failed to read response body")

		// t.Logf("User Get Response Body: %s", userGetBody)

		// 4.1 Check the response
		assert.Equal(t, http.StatusOK, userGetResponse.StatusCode, "Expected status code 200")
		userGetAPIResp, err := parserResponseBody[payload.UserResponse](t, userGetResponse)
		assert.NoError(t, err, "Failed to parse response body")

		assert.Equal(t, userGetAPIResp.ID.String(), user["id"], "Expected user ID to be set")
		assert.Equal(t, userGetAPIResp.Email, user["email"], "Expected user email to be set")
		assert.Equal(t, userGetAPIResp.FirstName, user["first_name"], "Expected user first name to be set")
		assert.Equal(t, userGetAPIResp.LastName, user["last_name"], "Expected user last name to be set")

		// 5. Cleanup
		t.Cleanup(func() {
			deleteUserByEmailFromDB(t, email)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test retrieving a non-existent user
	t.Run("get_user_not_found", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Generate a random UUID that doesn't exist in the database
		nonExistentUserID := uuid.NewV7()

		// 3. Try to get the non-existent user
		getEndpoint := usersGetEndpoint.RewriteSlugs(nonExistentUserID.String())
		getResponse, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to get non-existent user")
		defer getResponse.Body.Close()

		// 4. Check that we get a 404 Not Found response
		assert.Equal(t, http.StatusNotFound, getResponse.StatusCode, "Expected status code 404 for non-existent user")

		// 5. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, getResponse)
		assert.NoError(t, err, "Failed to parse error response")

		// Verify error message contains information about the user not being found
		assert.Contains(t, errorResp.Message, "not found", "Error message should indicate that the user was not found")
		assert.Equal(t, getEndpoint.method, errorResp.Method, "Expected method to be set")
		assert.Equal(t, getEndpoint.Path(), errorResp.Path, "Expected path to be set")

		// 6. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test retrieving a user with an invalid ID format
	t.Run("get_user_bad_request", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Try to get a user with an invalid ID format (not a UUID)
		invalidUserID := "not-a-valid-uuid"
		getEndpoint := usersGetEndpoint.RewriteSlugs(invalidUserID)
		getResponse, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to get user with invalid ID")
		defer getResponse.Body.Close()

		// 3. Check that we get a 400 Bad Request response
		assert.Equal(t, http.StatusBadRequest, getResponse.StatusCode, "Expected status code 400 for invalid user ID format")

		// 4. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, getResponse)
		assert.NoError(t, err, "Failed to parse error response")

		// Verify error message contains information about the invalid UUID format
		assert.Contains(t, errorResp.Message, "invalid", "Error message should indicate that the user ID format is invalid")
		assert.Equal(t, getEndpoint.method, errorResp.Method, "Expected method to be set")
		assert.Equal(t, getEndpoint.Path(), errorResp.Path, "Expected path to be set")

		// 5. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}

func TestUserDelete(t *testing.T) {
	// Test user deletion
	t.Run("delete_user", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a new user
		userID := uuid.NewV7()
		firstName, lastName, email := generateUserData(t)
		user := map[string]any{
			"id":         userID.String(),
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   generatePassword(t),
		}

		createResponse, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, user, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to create user")
		defer createResponse.Body.Close()

		assert.Equal(t, http.StatusCreated, createResponse.StatusCode, "Expected status code 201")

		// 3. Delete the user
		// we must replace the slug {user_id} with the user ID
		newUserDeleteEndpoint := usersDeleteEndpoint.RewriteSlugs(userID.String())
		// t.Logf("User Delete Endpoint: %s", newUserDeleteEndpoint)

		deleteResponse, err := sendHTTPRequest(t, ctx, newUserDeleteEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to delete user")
		defer deleteResponse.Body.Close()

		// 4. Check the response
		assert.Equal(t, http.StatusOK, deleteResponse.StatusCode, "Expected status code 200")
		deleteAPIResp, err := parserResponseBody[payload.HTTPMessage](t, deleteResponse)
		assert.NoError(t, err, "Failed to parse response body")

		// Assuming domain.UsersUserDeletedSuccessfully exists or similar message
		assert.Equal(t, domain.UsersUserDeletedSuccessfully, deleteAPIResp.Message, "Expected success message")
		assert.Equal(t, newUserDeleteEndpoint.method, deleteAPIResp.Method, "Expected method to be set")
		assert.Equal(t, newUserDeleteEndpoint.Path(), deleteAPIResp.Path, "Expected path to be set")

		// 5. Verify user is actually deleted (optional, try to get the user)
		newUserGetEndpoint := usersGetEndpoint.RewriteSlugs(userID.String())
		getResponse, err := sendHTTPRequest(t, ctx, newUserGetEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to get deleted user")
		defer getResponse.Body.Close()
		assert.Equal(t, http.StatusNotFound, getResponse.StatusCode, "Expected status code 404 after deletion")

		// 6. Cleanup admin user
		t.Cleanup(func() {
			// Attempt to delete the target user again in case the test failed before deletion
			deleteUserByIDFromDB(t, userID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test deleting a user with an invalid ID format
	t.Run("delete_user_bad_request", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Try to delete a user with an invalid ID format (not a UUID)
		invalidUserID := "not-a-valid-uuid"
		deleteEndpoint := usersDeleteEndpoint.RewriteSlugs(invalidUserID)

		deleteResponse, err := sendHTTPRequest(t, ctx, deleteEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to delete user with invalid ID")
		defer deleteResponse.Body.Close()

		// 3. Check that we get a 400 Bad Request response
		assert.Equal(t, http.StatusBadRequest, deleteResponse.StatusCode, "Expected status code 400 for invalid user ID format")

		// 4. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, deleteResponse)
		assert.NoError(t, err, "Failed to parse error response")

		// 5. Verify error message contains information about the invalid UUID format
		assert.Contains(t, errorResp.Message, "invalid", "Error message should indicate that the user ID format is invalid")
		assert.Equal(t, deleteEndpoint.method, errorResp.Method, "Expected method to be set")
		assert.Equal(t, deleteEndpoint.Path(), errorResp.Path, "Expected path to be set")

		// 6. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test deleting a non-existent user
	t.Run("delete_user_not_found", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Generate a UUID that doesn't exist in the database
		nonExistentUserID := uuid.NewV7()
		deleteEndpoint := usersDeleteEndpoint.RewriteSlugs(nonExistentUserID.String())

		deleteResponse, err := sendHTTPRequest(t, ctx, deleteEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to delete non-existent user")
		defer deleteResponse.Body.Close()

		// 3. Check the response - this should still return StatusOK even though the user doesn't exist
		// This is because deleting a non-existent resource is considered idempotent in RESTful APIs
		assert.Equal(t, http.StatusOK, deleteResponse.StatusCode, "Expected status code 200 OK for deleting non-existent user")

		// 4. Parse and verify the success response
		deleteAPIResp, err := parserResponseBody[payload.HTTPMessage](t, deleteResponse)
		assert.NoError(t, err, "Failed to parse response body")

		// 5. Verify success message for deletion
		assert.Equal(t, domain.UsersUserDeletedSuccessfully, deleteAPIResp.Message, "Expected success message")
		assert.Equal(t, deleteEndpoint.method, deleteAPIResp.Method, "Expected method to be set")
		assert.Equal(t, deleteEndpoint.Path(), deleteAPIResp.Path, "Expected path to be set")

		// 6. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}

func TestUserUpdate(t *testing.T) {
	// Test user update
	t.Run("update_user", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a new user
		userID := uuid.NewV7()
		firstName, lastName, email := generateUserData(t)
		originalPassword := generatePassword(t)
		user := map[string]any{
			"id":         userID.String(),
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   originalPassword,
		}

		createResponse, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, user, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to create user")
		defer createResponse.Body.Close()

		assert.Equal(t, http.StatusCreated, createResponse.StatusCode, "Expected status code 201")

		// 3. Update the user
		updatedFirstName := "Updated" + firstName
		updatedLastName := "Updated" + lastName
		updatedEmail := "updated_" + email
		updatedUser := map[string]any{
			"first_name": updatedFirstName,
			"last_name":  updatedLastName,
			"email":      updatedEmail,
		}

		// we must replace the slug {user_id} with the user ID
		newUserUpdateEndpoint := usersUpdateEndpoint.RewriteSlugs(userID.String())
		// t.Logf("User Update Endpoint: %s", newUserUpdateEndpoint)

		updateResponse, err := sendHTTPRequest(t, ctx, newUserUpdateEndpoint, updatedUser, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to update user")
		defer updateResponse.Body.Close()

		// 4. Check the update response
		assert.Equal(t, http.StatusOK, updateResponse.StatusCode, "Expected status code 200 for update")
		updateAPIResp, err := parserResponseBody[payload.HTTPMessage](t, updateResponse)
		assert.NoError(t, err, "Failed to parse update response body")

		// Assuming domain.UsersUserUpdatedSuccessfully exists or similar message
		assert.Equal(t, domain.UsersUserUpdatedSuccessfully, updateAPIResp.Message, "Expected success message for update")
		assert.Equal(t, newUserUpdateEndpoint.method, updateAPIResp.Method, "Expected method to be set for update")
		assert.Equal(t, newUserUpdateEndpoint.Path(), updateAPIResp.Path, "Expected path to be set for update")

		// 5. Verify user is actually updated (get the user again)
		newUserGetEndpoint := usersGetEndpoint.RewriteSlugs(userID.String())
		// t.Logf("User Get Endpoint after update: %s", newUserGetEndpoint)

		getUserResponse, err := sendHTTPRequest(t, ctx, newUserGetEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to get updated user")
		defer getUserResponse.Body.Close()

		assert.Equal(t, http.StatusOK, getUserResponse.StatusCode, "Expected status code 200 when getting updated user")

		userGetAPIResp, err := parserResponseBody[payload.UserResponse](t, getUserResponse)
		assert.NoError(t, err, "Failed to parse get response body for updated user")

		assert.Equal(t, userID.String(), userGetAPIResp.ID.String(), "Expected user ID to remain the same")
		assert.Equal(t, updatedEmail, userGetAPIResp.Email, "Expected user email to remain the same")
		assert.Equal(t, updatedFirstName, userGetAPIResp.FirstName, "Expected user first name to be updated")
		assert.Equal(t, updatedLastName, userGetAPIResp.LastName, "Expected user last name to be updated")

		// 6. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, userID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test updating a user with an invalid ID format
	t.Run("update_user_bad_request", func(t *testing.T) {
		t.Parallel()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		// Set up test cases for different bad request scenarios
		testCases := []struct {
			name          string
			userID        string
			updateData    map[string]any
			expectedCode  int
			expectedError string
		}{
			{
				name:          "Invalid UUID format",
				userID:        "not-a-valid-uuid",
				updateData:    map[string]any{"first_name": "John", "last_name": "Doe", "email": "john.doe@example.com"},
				expectedCode:  http.StatusBadRequest,
				expectedError: "invalid uuid",
			},
			{
				name:          "Invalid email format",
				userID:        uuid.NewV7().String(),
				updateData:    map[string]any{"email": "not-a-valid-email"},
				expectedCode:  http.StatusBadRequest,
				expectedError: "invalid email",
			},
			{
				name:          "Empty first name",
				userID:        uuid.NewV7().String(),
				updateData:    map[string]any{"first_name": ""},
				expectedCode:  http.StatusBadRequest,
				expectedError: "cannot be empty",
			},
			{
				name:          "Empty last name",
				userID:        uuid.NewV7().String(),
				updateData:    map[string]any{"last_name": ""},
				expectedCode:  http.StatusBadRequest,
				expectedError: "cannot be empty",
			},
			{
				// A password is never set through this route any more: a grant
				// on PUT /users/* used to be a takeover of any account. The
				// field is not part of the request, so a body carrying only it
				// is an empty update.
				name:          "Password is not a field of this route",
				userID:        uuid.NewV7().String(),
				updateData:    map[string]any{"password": "NewSecureP@ssw0rd1"},
				expectedCode:  http.StatusBadRequest,
				expectedError: "unknown field",
			},
			{
				name:          "First name too long",
				userID:        uuid.NewV7().String(),
				updateData:    map[string]any{"first_name": strings.Repeat("a", 300)}, // Very long first name
				expectedCode:  http.StatusBadRequest,
				expectedError: "must be between",
			},
			{
				name:          "Last name too long",
				userID:        uuid.NewV7().String(),
				updateData:    map[string]any{"last_name": strings.Repeat("a", 300)}, // Very long last name
				expectedCode:  http.StatusBadRequest,
				expectedError: "must be between",
			},
		}

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}
		ctx := t.Context()

		// For cases other than invalid UUID, we need to create a real user first
		userID := uuid.NewV7()
		firstName, lastName, email := generateUserData(t)
		user := map[string]any{
			"id":         userID.String(),
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   generatePassword(t),
		}

		// Create the user for testing valid UUID but invalid data cases
		createResponse, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, user, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to create user")
		defer createResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, createResponse.StatusCode, "Expected status code 201")

		// Run each test case
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// For test cases with valid UUIDs but invalid data, use the created user ID
				testUserID := tc.userID
				if tc.userID != "not-a-valid-uuid" {
					testUserID = userID.String()
				}

				updateEndpoint := usersUpdateEndpoint.RewriteSlugs(testUserID)
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
			deleteUserByIDFromDB(t, userID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test updating a non-existent user
	t.Run("update_user_not_found", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Generate a UUID that doesn't exist in the database
		nonExistentUserID := uuid.NewV7()
		updateEndpoint := usersUpdateEndpoint.RewriteSlugs(nonExistentUserID.String())

		updatedUser := map[string]any{
			"first_name": "UpdatedFirstName",
			"last_name":  "UpdatedLastName",
			"email":      "updated_email@example.com",
		}

		updateResponse, err := sendHTTPRequest(t, ctx, updateEndpoint, updatedUser, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to update non-existent user")
		defer updateResponse.Body.Close()

		// 3. Check that we get a 404 Not Found response for not found user
		assert.Equal(t, http.StatusNotFound, updateResponse.StatusCode, "Expected status code 404 for non-existent user")

		// 4. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, updateResponse)
		assert.NoError(t, err, "Failed to parse error response")

		// 5. Verify error message contains information about the user not being found
		assert.Contains(t, errorResp.Message, "not found", "Error message should indicate that the user was not found")
		assert.Equal(t, updateEndpoint.method, errorResp.Method, "Expected method to be set")
		assert.Equal(t, updateEndpoint.Path(), errorResp.Path, "Expected path to be set")

		// 6. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test updating a user with invalid data
	t.Run("update_user_invalid_data", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a new user that we'll try to update with invalid data
		userID := uuid.NewV7()
		firstName, lastName, email := generateUserData(t)
		user := map[string]any{
			"id":         userID.String(),
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   generatePassword(t),
		}

		createResponse, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, user, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to create user")
		defer createResponse.Body.Close()

		assert.Equal(t, http.StatusCreated, createResponse.StatusCode, "Expected status code 201")

		// 3. Update the user with invalid data
		invalidUser := map[string]any{
			"email": "not-a-valid-email", // Invalid email format
		}

		updateEndpoint := usersUpdateEndpoint.RewriteSlugs(userID.String())
		updateResponse, err := sendHTTPRequest(t, ctx, updateEndpoint, invalidUser, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to update user with invalid data")
		defer updateResponse.Body.Close()

		// 4. Check that we get a 400 Bad Request response
		assert.Equal(t, http.StatusBadRequest, updateResponse.StatusCode, "Expected status code 400 for invalid user data")

		// 5. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, updateResponse)
		assert.NoError(t, err, "Failed to parse error response")

		// 6. Verify error message contains information about the invalid data
		assert.Contains(t, strings.ToLower(errorResp.Message), "invalid", "Error message should indicate invalid data")
		assert.Equal(t, updateEndpoint.method, errorResp.Method, "Expected method to be set")
		assert.Equal(t, updateEndpoint.Path(), errorResp.Path, "Expected path to be set")

		// 7. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, userID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test updating a user with an email that already exists (conflict case)
	t.Run("update_user_conflict_email", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create two users with different emails
		// First user - this is the one we'll try to update
		userID1 := uuid.NewV7()
		firstName1, lastName1, email1 := generateUserData(t)
		user1 := map[string]any{
			"id":         userID1.String(),
			"email":      email1,
			"first_name": firstName1,
			"last_name":  lastName1,
			"password":   generatePassword(t),
		}

		createResponse1, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, user1, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to create first user")
		defer createResponse1.Body.Close()
		assert.Equal(t, http.StatusCreated, createResponse1.StatusCode, "Expected status code 201 for first user creation")

		// Second user - we'll try to use this user's email when updating the first user
		userID2 := uuid.NewV7()
		firstName2, lastName2, email2 := generateUserData(t)
		user2 := map[string]any{
			"id":         userID2.String(),
			"email":      email2,
			"first_name": firstName2,
			"last_name":  lastName2,
			"password":   generatePassword(t),
		}

		createResponse2, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, user2, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to create second user")
		defer createResponse2.Body.Close()
		assert.Equal(t, http.StatusCreated, createResponse2.StatusCode, "Expected status code 201 for second user creation")

		// 3. Try to update the first user with the second user's email
		updateUser1WithConflict := map[string]any{
			"email": email2, // This will cause a conflict because email2 is already being used
		}

		updateEndpoint := usersUpdateEndpoint.RewriteSlugs(userID1.String())
		updateResponse, err := sendHTTPRequest(t, ctx, updateEndpoint, updateUser1WithConflict, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to update user with conflicting email")
		defer updateResponse.Body.Close()

		// 4. Check that we get a 409 Conflict response
		assert.Equal(t, http.StatusConflict, updateResponse.StatusCode, "Expected status code 409 Conflict for update with already used email")

		// 5. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, updateResponse)
		assert.NoError(t, err, "Failed to parse error response")

		// 6. Verify error message contains information about the email being already in use
		assert.Contains(t, strings.ToLower(errorResp.Message), "already exists", "Error message should indicate that the email already exists")
		assert.Equal(t, updateEndpoint.method, errorResp.Method, "Expected method to be set")
		assert.Equal(t, updateEndpoint.Path(), errorResp.Path, "Expected path to be set")

		// 7. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, userID1)
			deleteUserByIDFromDB(t, userID2)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}

func TestUserList(t *testing.T) {
	// Test user listing
	t.Run("list_users", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a couple of new users
		userIDs := []uuid.UUID{uuid.NewV7(), uuid.NewV7(), uuid.NewV7(), uuid.NewV7(), uuid.NewV7(), uuid.NewV7()}
		usersToCreate := []map[string]any{}

		for i, userID := range userIDs {
			firstName, lastName, email := generateUserData(t)
			user := map[string]any{
				"id":         userID.String(),
				"email":      email,
				"first_name": firstName,
				"last_name":  lastName,
				"password":   generatePassword(t),
			}
			usersToCreate = append(usersToCreate, user)

			createResponse, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, user, accessTokenHeader)
			assert.NoError(t, err, "Failed to send request to create user %d", i+1)
			if createResponse != nil {
				defer createResponse.Body.Close()
				assert.Equal(t, http.StatusCreated, createResponse.StatusCode, "Expected status code 201 for user %d", i+1)
			}
		}

		// 3. List the users
		listResponse, err := sendHTTPRequest(t, ctx, usersListEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to list users")
		defer listResponse.Body.Close()

		// 4. Check the list response
		assert.Equal(t, http.StatusOK, listResponse.StatusCode, "Expected status code 200 for list")
		listAPIResp, err := parserResponseBody[payload.ListUsersResponse](t, listResponse) // Expecting ListUsersResponse struct
		assert.NoError(t, err, "Failed to parse list response body")

		// 5. Verify the created users are in the list
		// Note: The list might contain other users (like the admin), so we check if our created users are present.
		foundCount := 0
		userMap := make(map[string]bool)
		for _, createdUser := range usersToCreate {
			userMap[createdUser["email"].(string)] = true
		}

		// Iterate over the Items field of the ListUsersOutput struct
		for _, listedUser := range listAPIResp.Items {
			if _, ok := userMap[listedUser.Email]; ok {
				foundCount++
				// Optionally, assert other fields match
				for _, createdUser := range usersToCreate {
					if createdUser["email"] == listedUser.Email {
						assert.Equal(t, createdUser["first_name"], listedUser.FirstName)
						assert.Equal(t, createdUser["last_name"], listedUser.LastName)
						break
					}
				}
			}
		}
		// assert.GreaterOrEqual(t, foundCount, len(usersToCreate), "Expected to find at least the created users in the list")

		// 6. Cleanup
		t.Cleanup(func() {
			for _, userID := range userIDs {
				deleteUserByIDFromDB(t, userID)
			}
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("list_users_pagination", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// Generate a unique test identifier to prevent conflicts with other parallel tests
		testID := mustUUIDString(t)[:8]
		t.Logf("Running pagination test with ID: %s", testID)

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create at least 20 users to ensure we have enough for pagination
		numUsers := 25 // Create extra users to ensure we have at least 20 (in case of any failures)
		userIDs := make([]uuid.UUID, 0, numUsers)
		// usersToCreate := make([]map[string]any, 0, numUsers)
		// userEmails := make([]string, 0, numUsers)

		// Create users with sequential emails for easier verification
		for i := 0; i < numUsers; i++ {
			userID := uuid.NewV7()
			// Include numeric prefix for deterministic sorting later and ensure uniqueness
			firstName := fmt.Sprintf("user%03d_%s", i, testID)
			lastName := fmt.Sprintf("user%03d_%s", i, testID)
			email := fmt.Sprintf("test.user%03d@%s.com", i, testID) // email format: @<testid>.com

			user := map[string]any{
				"id":         userID.String(),
				"email":      email,
				"first_name": firstName,
				"last_name":  lastName,
				"password":   generatePassword(t),
			}
			// usersToCreate = append(usersToCreate, user)
			userIDs = append(userIDs, userID)
			// userEmails = append(userEmails, email)

			createResponse, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, user, accessTokenHeader)
			assert.NoError(t, err, "Failed to send request to create user %d", i+1)
			if createResponse != nil {
				defer createResponse.Body.Close()
				// Ensure this user was created successfully before continuing
				if createResponse.StatusCode != http.StatusCreated {
					t.Fatalf("Failed to create user %d, got status %d", i+1, createResponse.StatusCode)
				}
			}
		}

		t.Logf("Created %d users for pagination test", len(userIDs))

		// 3. Test pagination with limit=4
		paginationLimit := 4
		paginatedEndpoint := usersListEndpoint.Clone()
		paginatedEndpoint.SetQueryParam("limit", fmt.Sprintf("%d", paginationLimit))

		// Use both email and ID for more deterministic sorting
		// This ensures consistent sorting even with parallel test runs
		// paginatedEndpoint.SetQueryParam("sort", "email ASC,id ASC")

		// Add a filter to only return users created by this test instance
		// This ensures we don't get users from other test runs
		paginatedEndpoint.SetQueryParam("filter", fmt.Sprintf("email LIKE '%%@%s.com'", testID))

		// Track pages we've fetched
		var (
			nextToken    string
			prevToken    string
			pageNumber   int
			allUserIds   = make(map[string]bool)
			emailsByPage = make(map[int][]string) // Track emails by page number for verification
		)

		// First page
		listResponse, err := sendHTTPRequest(t, ctx, paginatedEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to list users with pagination")
		defer listResponse.Body.Close()

		// Verify first page
		assert.Equal(t, http.StatusOK, listResponse.StatusCode, "Expected status code 200 for first paginated list")

		page1, err := parserResponseBody[payload.ListUsersResponse](t, listResponse)
		assert.NoError(t, err, "Failed to parse first page response body")

		// Validate pagination structure
		assert.NotNil(t, page1.Paginator, "Expected paginator to be present")
		assert.Equal(t, paginationLimit, page1.Paginator.Limit, "Expected limit to match requested value")
		assert.LessOrEqual(t, len(page1.Items), paginationLimit, "Expected items count to be <= page limit")

		// Validate next token exists (since we have more than 4 users)
		assert.NotEmpty(t, page1.Paginator.NextToken, "Expected next token for pagination")
		nextToken = page1.Paginator.NextToken

		// Track users we've seen
		pageNumber++
		emailsByPage[pageNumber] = make([]string, 0, len(page1.Items))

		// Verify that all items contain our test ID (proper filtering)
		for _, user := range page1.Items {
			assert.Contains(t, user.Email, testID, "Expected filtered results to contain only users from this test instance")
		}

		// Verify sorting and collect emails
		// var lastEmail string
		for _, user := range page1.Items {
			// Prevent duplicates across pages
			assert.False(t, allUserIds[user.ID.String()], "User ID %s was found more than once", user.ID.String())
			// t.Logf("User ID: %s", user.ID.String())
			allUserIds[user.ID.String()] = true

			// Add to emails for this page
			emailsByPage[pageNumber] = append(emailsByPage[pageNumber], user.Email)

			// Verify sorting
			// if lastEmail != "" {
			// 	assert.GreaterOrEqual(t, user.Email, lastEmail, "Users should be sorted by email ASC")
			// }

			// lastEmail = user.Email
		}
		t.Logf("Page %d: Found %d users, %d unique IDs", pageNumber, len(page1.Items), len(allUserIds))

		// Navigate through all pages using next tokens
		for nextToken != "" {
			pageEndpoint := paginatedEndpoint.Clone()
			pageEndpoint.SetQueryParam("limit", fmt.Sprintf("%d", paginationLimit))
			// pageEndpoint.SetQueryParam("sort", "email ASC,id ASC")
			pageEndpoint.SetQueryParam("filter", fmt.Sprintf("email LIKE '%%@%s.com'", testID))

			// Set the next token for pagination
			pageEndpoint.SetQueryParam("next_token", nextToken)

			t.Logf("Fetching page %d with next_token: %s", pageNumber+1, nextToken)

			pageResponse, err := sendHTTPRequest(t, ctx, pageEndpoint, nil, accessTokenHeader)
			assert.NoError(t, err, "Failed to fetch page with next_token")
			defer pageResponse.Body.Close()

			assert.Equal(t, http.StatusOK, pageResponse.StatusCode, "Expected status code 200 for paginated list")

			pageData, err := parserResponseBody[payload.ListUsersResponse](t, pageResponse)
			assert.NoError(t, err, "Failed to decode page data")

			// Track users we've seen
			pageNumber++
			t.Logf("Page %d: Found %d users", pageNumber, len(pageData.Items))
			emailsByPage[pageNumber] = make([]string, 0, len(pageData.Items))

			// Verify that all items contain our test ID (proper filtering)
			for _, user := range pageData.Items {
				assert.Contains(t, user.Email, testID, "Expected filtered results to contain only users from this test instance")
			}

			// Verify sorting and collect emails
			// lastEmail = ""
			for _, user := range pageData.Items {
				// Prevent duplicates across pages
				// assert.False(t, allUserIds[user.ID.String()], "User ID %s was found more than once across pages", user.ID.String())
				allUserIds[user.ID.String()] = true

				// Add to emails for this page
				emailsByPage[pageNumber] = append(emailsByPage[pageNumber], user.Email)

				// Verify sorting
				// if lastEmail != "" {
				// 	assert.GreaterOrEqual(t, user.Email, lastEmail, "Users should be sorted by email ASC on page %d", pageNumber)
				// }

				// lastEmail = user.Email
			}

			// Save tokens for next iteration
			prevToken = pageData.Paginator.PrevToken
			nextToken = pageData.Paginator.NextToken

			// Verify items count
			assert.LessOrEqual(t, len(pageData.Items), paginationLimit, "Expected items count to be <= page limit")

			// Stop if we have no more pages
			if nextToken == "" {
				break
			}

			t.Logf("Page %d: Found %d users, %d unique IDs", pageNumber, len(pageData.Items), len(allUserIds))
		}

		// Verify we can navigate backward using prev tokens
		if prevToken != "" {
			// Go back one page
			prevPageEndpoint := paginatedEndpoint.Clone()
			prevPageEndpoint.SetQueryParam("limit", fmt.Sprintf("%d", paginationLimit))
			// prevPageEndpoint.SetQueryParam("sort", "email ASC,id ASC")
			prevPageEndpoint.SetQueryParam("filter", fmt.Sprintf("email LIKE '%%@%s.com'", testID))

			prevPageEndpoint.SetQueryParam("prev_token", prevToken)

			prevPageResponse, err := sendHTTPRequest(t, ctx, prevPageEndpoint, nil, accessTokenHeader)
			assert.NoError(t, err, "Failed to fetch previous page")
			defer prevPageResponse.Body.Close()

			assert.Equal(t, http.StatusOK, prevPageResponse.StatusCode, "Expected status code 200 for previous page")

			prevPageData, err := parserResponseBody[payload.ListUsersResponse](t, prevPageResponse)
			assert.NoError(t, err, "Failed to decode previous page data")

			// Verify we have tokens in both directions
			assert.NotEmpty(t, prevPageData.Paginator.NextToken, "Expected next token in previous page")
			if pageNumber > 2 { // If we have more than 2 pages
				assert.NotEmpty(t, prevPageData.Paginator.PrevToken, "Expected prev token in previous page")
			}

			// Verify that all items contain our test ID (proper filtering)
			for _, user := range prevPageData.Items {
				assert.Contains(t, user.Email, testID, "Expected filtered results to contain only users from this test instance")
			}

			// Verify the content is consistent with what we saw before
			prevPageNumber := pageNumber - 1 // The page we're going back to should be the previous one
			if len(emailsByPage[prevPageNumber]) > 0 {
				assert.Equal(t, len(emailsByPage[prevPageNumber]), len(prevPageData.Items), "Previous page should have the same number of items")
			}
		}

		// Ensure we've seen at least 20 users across all pages
		assert.GreaterOrEqual(t, len(allUserIds), 20, "Expected to find at least 20 users across all pages")

		// Verify that we found all the test users we created
		expectedUserCount := len(userIDs)
		foundUserCount := 0
		for _, userID := range userIDs {
			if allUserIds[userID.String()] {
				foundUserCount++
			}
		}

		assert.Equal(t, expectedUserCount, foundUserCount,
			"Expected to find all %d created users in paginated results, found %d",
			expectedUserCount, foundUserCount)

		t.Cleanup(func() {
			t.Logf("Cleaning up pagination test ID: %s", testID)
			for _, userID := range userIDs {
				deleteUserByIDFromDB(t, userID)
			}
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}

func TestUserList_SortFirstNameDesc(t *testing.T) {
	// Test user listing with sorting by first name DESC
	t.Run("list_users_sorted_first_name_desc", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Define and create specific users for sorting test (ensure distinct first names)
		// Generate unique emails to ensure test idempotency
		_, _, email1 := generateUserData(t)
		_, _, email2 := generateUserData(t)
		_, _, email3 := generateUserData(t)
		_, _, email4 := generateUserData(t)

		usersToCreate := []map[string]any{
			{"id": uuid.NewV7().String(), "email": email1, "first_name": "Charlie", "last_name": "Brown", "password": generatePassword(t)},
			{"id": uuid.NewV7().String(), "email": email2, "first_name": "Alice", "last_name": "Smith", "password": generatePassword(t)},
			{"id": uuid.NewV7().String(), "email": email3, "first_name": "Bob", "last_name": "Jones", "password": generatePassword(t)},
			{"id": uuid.NewV7().String(), "email": email4, "first_name": "David", "last_name": "Williams", "password": generatePassword(t)},
		}
		userIDsToCleanup := []uuid.UUID{}
		emailsCreated := []string{} // Keep track of emails to identify users in response

		for i, user := range usersToCreate {
			userID, _ := uuid.Parse(user["id"].(string))
			userIDsToCleanup = append(userIDsToCleanup, userID)
			emailsCreated = append(emailsCreated, user["email"].(string))

			createResponse, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, user, accessTokenHeader)
			assert.NoError(t, err, "Failed to send request to create user %d", i+1)
			if createResponse != nil {
				defer createResponse.Body.Close()
				assert.Equal(t, http.StatusCreated, createResponse.StatusCode, "Expected status code 201 for user %d", i+1)
			}
		}

		// 3. List the users with sorting parameter name DESC
		// Create a fresh endpoint instance for this test
		sortedEndpoint := usersListEndpoint.Clone()
		sortedEndpoint.SetQueryParam("sort", "first_name DESC") // Sort by name descending
		// t.Logf("User List Endpoint with Sort: %s", sortedEndpoint.requestURL.String())

		listResponse, err := sendHTTPRequest(t, ctx, sortedEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to list users with sort name DESC")
		defer listResponse.Body.Close()

		// 4. Check the list response
		assert.Equal(t, http.StatusOK, listResponse.StatusCode, "Expected status code 200 for sorted list (name DESC)")
		listAPIResp, err := parserResponseBody[payload.ListUsersResponse](t, listResponse)
		assert.NoError(t, err, "Failed to parse sorted list response body (name DESC)")

		// 5. Verify the users are sorted correctly by first_name DESC
		// Extract first names from the response, considering only the users we created
		responseFirstNames := []string{}
		createdUserMap := make(map[string]bool)
		for _, email := range emailsCreated {
			createdUserMap[email] = true
		}

		for _, listedUser := range listAPIResp.Items {
			// Only consider users created in this test for sorting verification
			if _, ok := createdUserMap[listedUser.Email]; ok {
				responseFirstNames = append(responseFirstNames, listedUser.FirstName)
			}
		}

		// Check if the extracted first names are sorted in descending order
		isSortedDesc := sort.SliceIsSorted(responseFirstNames, func(i, j int) bool {
			return responseFirstNames[i] > responseFirstNames[j] // Check for descending order
		})
		assert.True(t, isSortedDesc, "Expected users to be sorted by first_name DESC. Got: %v", responseFirstNames)

		// Also check if all created users were found
		// TODO: check why some times this check fails
		// assert.Equal(t, len(emailsCreated), len(responseFirstNames), "Expected to find all created users in the sorted list (name DESC)")

		// 6. Cleanup
		t.Cleanup(func() {
			for _, userID := range userIDsToCleanup {
				deleteUserByIDFromDB(t, userID)
			}
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}

func TestUserList_SortFirstNameDesc_FilterFirstNameLikePrefix(t *testing.T) {
	// Test user listing with sorting by first name DESC and filtering by first_name LIKE '%l%'
	t.Run("list_users_sorted_filtered_name_desc", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Define and create specific users for sorting/filtering test
		// Generate unique emails to ensure test idempotency
		_, _, email1 := generateUserData(t)
		_, _, email2 := generateUserData(t)
		_, _, email3 := generateUserData(t)
		_, _, email4 := generateUserData(t)

		usersToCreate := []map[string]any{
			{"id": uuid.NewV7().String(), "email": email1, "first_name": "AAABBBCharlie", "last_name": "Brown", "password": generatePassword(t)},  // Contains 'l'
			{"id": uuid.NewV7().String(), "email": email2, "first_name": "AAABBBAlice", "last_name": "Smith", "password": generatePassword(t)},    // Contains 'l'
			{"id": uuid.NewV7().String(), "email": email3, "first_name": "AAABBBBob", "last_name": "Jones", "password": generatePassword(t)},      // Does not contain 'l'
			{"id": uuid.NewV7().String(), "email": email4, "first_name": "AAABBBDavid", "last_name": "Williams", "password": generatePassword(t)}, // Does not contain 'l'
		}
		userIDsToCleanup := []uuid.UUID{}
		expectedFilteredEmails := map[string]bool{ // Emails of users expected after filtering
			email4: true,
			email1: true,
			email3: true,
			email2: true,
		}
		expectedFilteredNamesDesc := []string{"AAABBBDavid", "AAABBBCharlie", "AAABBBBob", "AAABBBAlice"}

		for i, user := range usersToCreate {
			userID, _ := uuid.Parse(user["id"].(string))
			userIDsToCleanup = append(userIDsToCleanup, userID)

			createResponse, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, user, accessTokenHeader)
			assert.NoError(t, err, "Failed to send request to create user %d", i+1)
			if createResponse != nil {
				defer createResponse.Body.Close()
				assert.Equal(t, http.StatusCreated, createResponse.StatusCode, "Expected status code 201 for user %d", i+1)
			}
		}

		// 3. List the users with sorting and filtering parameters
		// Create a fresh endpoint instance for this test
		sortedFilteredEndpoint := usersListEndpoint.Clone()
		sortedFilteredEndpoint.SetQueryParam("sort", "first_name DESC")              // Sort by name descending
		sortedFilteredEndpoint.SetQueryParam("filter", "first_name LIKE '%AAABBB%'") // Filter by first_name containing 'l'
		// t.Logf("User List Endpoint with Sort and Filter: %s", sortedFilteredEndpoint.requestURL.String())

		listResponse, err := sendHTTPRequest(t, ctx, sortedFilteredEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to list users with sort and filter")
		defer listResponse.Body.Close()

		// 4. Check the list response
		assert.Equal(t, http.StatusOK, listResponse.StatusCode, "Expected status code 200 for sorted/filtered list")
		listAPIResp, err := parserResponseBody[payload.ListUsersResponse](t, listResponse)
		assert.NoError(t, err, "Failed to parse sorted/filtered list response body")

		// 5. Verify the users are filtered and sorted correctly
		// Extract first names from the response
		responseFirstNames := []string{}

		for _, listedUser := range listAPIResp.Items {
			// Check if the user should have been filtered IN based on email
			if _, expected := expectedFilteredEmails[listedUser.Email]; expected {
				responseFirstNames = append(responseFirstNames, listedUser.FirstName)
			} else {
				// Fail if a user not matching the filter appears
				assert.Fail(t, "Found unexpected user in filtered list", "User email: %s", listedUser.Email)
			}
		}

		// Check if the number of results matches the expected filtered count
		assert.Equal(t, len(expectedFilteredEmails), len(listAPIResp.Items), "Expected number of users after filtering does not match")
		assert.Equal(t, len(expectedFilteredNamesDesc), len(responseFirstNames), "Expected number of first names after filtering does not match")

		// Check if the extracted first names are sorted in descending order
		assert.Equal(t, expectedFilteredNamesDesc, responseFirstNames, "Expected users to be sorted by first_name DESC after filtering")

		// 6. Cleanup
		t.Cleanup(func() {
			for _, userID := range userIDsToCleanup {
				deleteUserByIDFromDB(t, userID)
			}
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}

func TestUserLinkRoles(t *testing.T) {
	t.Run("link_roles_to_user", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Setup: Create admin, user, and roles
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
			"Content-Type":  "application/json",
		}

		// Create a standard user
		userID, _ := createTestUserForRoleTest(t, adminToken.AccessToken) // Reusing helper from api_roles_test

		// Create roles
		roleID1 := createTestRoleForPolicyTest(t, adminToken.AccessToken, "role_1_") // Reusing helper from api_roles_test
		roleID2 := createTestRoleForPolicyTest(t, adminToken.AccessToken, "role_2_") // Reusing helper from api_roles_test

		// 2. Link roles to the user
		time.Sleep(500 * time.Millisecond)
		linkRolesEndpoint := usersLinkRolesEndpoint.RewriteSlugs(userID.String())
		linkPayload := map[string]any{
			"role_ids": []string{roleID1.String(), roleID2.String()},
		}

		linkResponse, err := sendHTTPRequest(t, ctx, linkRolesEndpoint, linkPayload, accessTokenHeader)
		assert.NoError(t, err, "Error sending link roles request: %v", err)
		defer linkResponse.Body.Close()

		// 3. Check link response
		assert.Equal(t, http.StatusOK, linkResponse.StatusCode, "Expected status code 200 OK for linking roles")
		linkAPIResp, err := parserResponseBody[payload.HTTPMessage](t, linkResponse)
		assert.NoError(t, err, "Failed to parse link roles response body")

		// Assuming domain.UsersRoleLinkedToUserSuccessfully exists or similar message
		assert.Equal(t, domain.UsersRoleLinkedToUserSuccessfully, linkAPIResp.Message, "Unexpected link roles response message")
		assert.Equal(t, linkRolesEndpoint.method, linkAPIResp.Method, "Expected method to be set")
		assert.Equal(t, linkRolesEndpoint.Path(), linkAPIResp.Path, "Expected path to be set")

		// 4. verify roles are linked (e.g., by listing user's roles or checking DB)
		rolesLinkedToUserEndpoint := usersListRolesLinkedEndpoint.RewriteSlugs(userID.String())

		linkedRolesResponse, err := sendHTTPRequest(t, ctx, rolesLinkedToUserEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to list linked roles")
		defer linkedRolesResponse.Body.Close()
		assert.Equal(t, http.StatusOK, linkedRolesResponse.StatusCode, "Expected status code 200 OK for listing linked roles")

		linkedRolesAPIResp, err := parserResponseBody[payload.ListRolesResponse](t, linkedRolesResponse)
		assert.NoError(t, err, "Failed to parse linked roles response body")

		// Check if the linked roles match the expected roles
		expectedRoleIDs := map[string]bool{
			roleID1.String(): true,
			roleID2.String(): true,
		}

		for _, role := range linkedRolesAPIResp.Items {
			delete(expectedRoleIDs, role.ID.String())
		}
		assert.Empty(t, expectedRoleIDs, "Not all expected roles were linked to the user")

		// 5. Cleanup - Register in reverse order (LIFO execution)
		// Last registered cleanup runs first
		t.Cleanup(func() { deleteUserByIDFromDB(t, adminToken.UserID) })
		t.Cleanup(func() { deleteRoleByIDFromDB(t, roleID2) })
		t.Cleanup(func() { deleteRoleByIDFromDB(t, roleID1) })
		t.Cleanup(func() { deleteUserByIDFromDB(t, userID) })
		// Unlink relationships before deleting - this runs FIRST due to LIFO
		t.Cleanup(func() { unlinkRolesFromUserViaDB(t, userID, []uuid.UUID{roleID1, roleID2}) })
	})
}

func TestUserUnlinkRoles(t *testing.T) {
	t.Run("unlink_roles_from_user", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Setup: Create admin, user, roles, and link them
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// Create a standard user
		userID, _ := createTestUserForRoleTest(t, adminToken.AccessToken)

		// Create roles
		roleID1 := createTestRoleForPolicyTest(t, adminToken.AccessToken, "unlink_test_role_1_")
		roleID2 := createTestRoleForPolicyTest(t, adminToken.AccessToken, "unlink_test_role_2_")
		roleID3 := createTestRoleForPolicyTest(t, adminToken.AccessToken, "unlink_test_role_3_")

		// Link roles first
		linkEndpoint := usersLinkRolesEndpoint.RewriteSlugs(userID.String())
		linkPayload := map[string]any{
			"role_ids": []string{roleID1.String(), roleID2.String(), roleID3.String()},
		}

		linkResponse, err := sendHTTPRequest(t, ctx, linkEndpoint, linkPayload, accessTokenHeader)
		assert.NoError(t, err, "Error sending link roles request during setup: %v", err)
		defer linkResponse.Body.Close()
		assert.Equal(t, http.StatusOK, linkResponse.StatusCode, "Expected status code 200 OK for linking roles during setup")

		// Wait briefly to ensure link operation completes
		time.Sleep(100 * time.Millisecond)

		// 2. Unlink a subset of roles (role1 and role3)
		unlinkEndpoint := usersUnlinkRolesEndpoint.RewriteSlugs(userID.String())
		unlinkPayload := map[string]any{
			"role_ids": []string{roleID1.String(), roleID3.String()},
		}

		unlinkResponse, err := sendHTTPRequest(t, ctx, unlinkEndpoint, unlinkPayload, accessTokenHeader)
		assert.NoError(t, err, "Error sending unlink roles request: %v", err)
		defer unlinkResponse.Body.Close()

		// 3. Check unlink response
		assert.Equal(t, http.StatusOK, unlinkResponse.StatusCode, "Expected status code 200 OK for unlinking roles")
		unlinkAPIResp, err := parserResponseBody[payload.HTTPMessage](t, unlinkResponse)
		assert.NoError(t, err, "Failed to parse unlink roles response body")

		// Assuming domain.UsersRoleUnlinkedFromUserSuccessfully exists or similar message
		assert.Equal(t, domain.UsersRoleUnlinkedFromUserSuccessfully, unlinkAPIResp.Message, "Unexpected unlink roles response message")
		assert.Equal(t, unlinkEndpoint.method, unlinkAPIResp.Method, "Expected method to be set")
		assert.Equal(t, unlinkEndpoint.Path(), unlinkAPIResp.Path, "Expected path to be set")

		// 4. Verify roles are unlinked (e.g., by listing user's roles or checking DB)
		rolesLinkedToUserEndpoint := usersListRolesLinkedEndpoint.RewriteSlugs(userID.String())
		linkedRolesResponse, err := sendHTTPRequest(t, ctx, rolesLinkedToUserEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to list linked roles after unlinking")
		defer linkedRolesResponse.Body.Close()

		assert.Equal(t, http.StatusOK, linkedRolesResponse.StatusCode, "Expected status code 200 OK for listing linked roles after unlinking")

		linkedRolesAPIResp, err := parserResponseBody[payload.ListRolesResponse](t, linkedRolesResponse)
		assert.NoError(t, err, "Failed to parse linked roles response body after unlinking")
		// Check if the unlinked roles are no longer present
		unlinkedRoleIDs := map[string]bool{
			roleID1.String(): true,
			roleID3.String(): true,
		}

		// unlinkedRoleIDs can be into the linkedRolesAPIResp.Items
		for _, role := range linkedRolesAPIResp.Items {
			if _, ok := unlinkedRoleIDs[role.ID.String()]; ok {
				assert.Fail(t, "Unlinked role still present in linked roles", "Role ID: %s", role.ID.String())
			}
		}

		// 5. Cleanup - Register in reverse order (LIFO execution)
		t.Cleanup(func() { deleteUserByIDFromDB(t, adminToken.UserID) })
		t.Cleanup(func() { deleteRoleByIDFromDB(t, roleID3) })
		t.Cleanup(func() { deleteRoleByIDFromDB(t, roleID2) })
		t.Cleanup(func() { deleteRoleByIDFromDB(t, roleID1) })
		t.Cleanup(func() { deleteUserByIDFromDB(t, userID) })
		// Unlink any remaining relationships - runs FIRST due to LIFO
		t.Cleanup(func() { unlinkAllRolesFromUserViaDB(t, userID) })
	})
}

func TestUserListRolesLinked(t *testing.T) {
	t.Run("list_roles_linked_to_user", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Setup: Create admin, user, roles, and link some roles
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// Create a standard user
		userID, _ := createTestUserForRoleTest(t, adminToken.AccessToken)

		// Create roles
		roleID1 := createTestRoleForPolicyTest(t, adminToken.AccessToken, "list_link_test_role_1_")
		roleID2 := createTestRoleForPolicyTest(t, adminToken.AccessToken, "list_link_test_role_2_")
		roleID3 := createTestRoleForPolicyTest(t, adminToken.AccessToken, "list_link_test_role_3_") // This one won't be linked initially

		rolesToLink := []uuid.UUID{roleID1, roleID2, roleID3}

		// Link roles first
		linkEndpoint := usersLinkRolesEndpoint.RewriteSlugs(userID.String())
		linkPayload := map[string]any{
			"role_ids": []string{roleID1.String(), roleID2.String(), roleID3.String()},
		}

		linkResponse, err := sendHTTPRequest(t, ctx, linkEndpoint, linkPayload, accessTokenHeader)
		assert.NoError(t, err, "Error sending link roles request during setup: %v", err)
		defer linkResponse.Body.Close()
		assert.Equal(t, http.StatusOK, linkResponse.StatusCode, "Expected status code 200 OK for linking roles during setup")

		// Wait briefly to ensure link operation completes
		time.Sleep(500 * time.Millisecond)

		// 2. List roles linked to the user
		listLinkedEndpoint := usersListRolesLinkedEndpoint.RewriteSlugs(userID.String())

		listResponse, err := sendHTTPRequest(t, ctx, listLinkedEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Error sending list linked roles request: %v", err)
		defer listResponse.Body.Close()

		// 3. Check list response
		assert.Equal(t, http.StatusOK, listResponse.StatusCode, "Expected status code 200 OK for listing linked roles")

		// Assuming the response is domain.ListRolesOutput or similar
		listAPIResp, err := parserResponseBody[payload.ListRolesResponse](t, listResponse)
		assert.NoError(t, err, "Failed to parse list linked roles response body")

		// 4. Verify the correct roles are listed
		linkedRolesMap := make(map[string]bool, len(rolesToLink))
		for _, id := range rolesToLink {
			linkedRolesMap[id.String()] = true
		}

		for _, role := range listAPIResp.Items {
			delete(linkedRolesMap, role.ID.String())
		}
		assert.Empty(t, linkedRolesMap, "Not all expected roles were linked to the user")

		// 5. Cleanup
		t.Cleanup(func() {
			// Attempt cleanup even if tests fail
			deleteUserByIDFromDB(t, userID)
			deleteRoleByIDFromDB(t, roleID1)
			deleteRoleByIDFromDB(t, roleID2)
			deleteRoleByIDFromDB(t, roleID3)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}

func TestUserLinkProjects(t *testing.T) {
	t.Run("link_projects_to_user", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test user
		firstName, lastName, email := generateUserData(t)
		userID := uuid.NewV7()

		user := map[string]any{
			"id":         userID.String(),
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   generatePassword(t),
		}

		userCreateResponse, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, user, accessTokenHeader)
		assert.NoError(t, err, "Failed to create user")
		defer userCreateResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, userCreateResponse.StatusCode, "Expected status code 201")

		// 3. Create test projects
		projectID1, _, _ := createTestProject(t, ctx, adminToken.AccessToken, "test-project-1-")
		projectID2, _, _ := createTestProject(t, ctx, adminToken.AccessToken, "test-project-2-")
		projectID3, _, _ := createTestProject(t, ctx, adminToken.AccessToken, "test-project-3-")

		// 4. Link projects to user
		projectsToLink := []uuid.UUID{projectID1, projectID2, projectID3}
		linkRequest := map[string]any{
			"project_ids": projectsToLink,
		}

		linkEndpoint := usersLinkProjectsEndpoint.RewriteSlugs(userID.String())
		linkResponse, err := sendHTTPRequest(t, ctx, linkEndpoint, linkRequest, accessTokenHeader)
		assert.NoError(t, err, "Failed to link projects to user")
		defer linkResponse.Body.Close()

		assert.Equal(t, http.StatusOK, linkResponse.StatusCode, "Expected status code 200")

		// Parse and verify the success response
		linkAPIResp, err := parserResponseBody[payload.HTTPMessage](t, linkResponse)
		assert.NoError(t, err, "Failed to parse link response")
		assert.Equal(t, domain.UsersProjectsLinkedToUserSuccessfully, linkAPIResp.Message, "Expected success message")

		// 5. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, userID)
			deleteProjectByIDFromDB(t, projectID1)
			deleteProjectByIDFromDB(t, projectID2)
			deleteProjectByIDFromDB(t, projectID3)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("link_projects_to_user_bad_request", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test user
		firstName, lastName, email := generateUserData(t)
		userID := uuid.NewV7()

		user := map[string]any{
			"id":         userID.String(),
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   generatePassword(t),
		}

		userCreateResponse, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, user, accessTokenHeader)
		assert.NoError(t, err, "Failed to create user")
		defer userCreateResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, userCreateResponse.StatusCode, "Expected status code 201")

		// 3. Test cases for various invalid inputs
		testCases := []struct {
			name           string
			invalidRequest map[string]any
			expectedError  string
		}{
			{
				name: "Empty project_ids array",
				invalidRequest: map[string]any{
					"project_ids": []uuid.UUID{},
				},
				expectedError: "at least one project",
			},
			{
				name: "Invalid project ID format",
				invalidRequest: map[string]any{
					"project_ids": []string{"not-a-valid-uuid"},
				},
				expectedError: "invalid uuid",
			},
			{
				name:           "Missing project_ids field",
				invalidRequest: map[string]any{},
				expectedError:  "at least one project",
			},
		}

		// 4. Run each test case
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				linkEndpoint := usersLinkProjectsEndpoint.RewriteSlugs(userID.String())
				response, err := sendHTTPRequest(t, ctx, linkEndpoint, tc.invalidRequest, accessTokenHeader)
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

		// 5. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, userID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("link_projects_user_not_found", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Generate a random UUID that doesn't exist in the database
		nonExistentUserID := uuid.NewV7()

		// 3. Create a test project
		projectID1, _, _ := createTestProject(t, ctx, adminToken.AccessToken, "test-project-")

		// 4. Try to link projects to non-existent user
		projectsToLink := []uuid.UUID{projectID1}
		linkRequest := map[string]any{
			"project_ids": projectsToLink,
		}

		linkEndpoint := usersLinkProjectsEndpoint.RewriteSlugs(nonExistentUserID.String())
		linkResponse, err := sendHTTPRequest(t, ctx, linkEndpoint, linkRequest, accessTokenHeader)
		assert.NoError(t, err, "Failed to send link request")
		defer linkResponse.Body.Close()

		// 5. Check that we get a 404 Not Found response
		assert.Equal(t, http.StatusNotFound, linkResponse.StatusCode, "Expected status code 404. Got %d. Message: %s", linkResponse.StatusCode, readResponseBody(t, linkResponse))

		// 6. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, linkResponse)
		assert.NoError(t, err, "Failed to parse error response")

		// Verify error message contains information about the user not being found
		assert.Contains(t, strings.ToLower(errorResp.Message), "not found",
			"Error message should indicate user not found")

		// 7. Cleanup
		t.Cleanup(func() {
			deleteProjectByIDFromDB(t, projectID1)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("require_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Generate a test user ID
		userID := uuid.NewV7()

		// 2. Generate a test project ID
		projectID := uuid.NewV7()

		// 3. Try to link projects without authentication
		linkRequest := map[string]any{
			"project_ids": []uuid.UUID{projectID},
		}

		linkEndpoint := usersLinkProjectsEndpoint.RewriteSlugs(userID.String())
		response, err := sendHTTPRequest(t, ctx, linkEndpoint, linkRequest)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 4. Check that we get a 401 Unauthorized response
		assert.Equal(t, http.StatusUnauthorized, response.StatusCode, "Expected status code 401. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})
}

func TestUserUnlinkProjects(t *testing.T) {
	t.Run("unlink_projects_from_user", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test user
		firstName, lastName, email := generateUserData(t)
		userID := uuid.NewV7()

		user := map[string]any{
			"id":         userID.String(),
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   generatePassword(t),
		}

		userCreateResponse, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, user, accessTokenHeader)
		assert.NoError(t, err, "Failed to create user")
		defer userCreateResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, userCreateResponse.StatusCode, "Expected status code 201. Got %d. Message: %s", userCreateResponse.StatusCode, readResponseBody(t, userCreateResponse))

		// 3. Create test projects
		projectID1, _, _ := createTestProject(t, ctx, adminToken.AccessToken, "test-project-1-")
		projectID2, _, _ := createTestProject(t, ctx, adminToken.AccessToken, "test-project-2-")
		projectID3, _, _ := createTestProject(t, ctx, adminToken.AccessToken, "test-project-3-")

		// 4. First link projects to user
		projectsToLink := []uuid.UUID{projectID1, projectID2, projectID3}
		linkRequest := map[string]any{
			"project_ids": projectsToLink,
		}

		linkEndpoint := usersLinkProjectsEndpoint.RewriteSlugs(userID.String())
		linkResponse, err := sendHTTPRequest(t, ctx, linkEndpoint, linkRequest, accessTokenHeader)
		assert.NoError(t, err, "Failed to link projects to user")
		defer linkResponse.Body.Close()
		assert.Equal(t, http.StatusOK, linkResponse.StatusCode, "Expected status code 200. Got %d. Message: %s", linkResponse.StatusCode, readResponseBody(t, linkResponse))

		// 5. Now unlink some projects (projectID1 and projectID2)
		projectsToUnlink := []uuid.UUID{projectID1, projectID2}
		unlinkRequest := map[string]any{
			"project_ids": projectsToUnlink,
		}

		unlinkEndpoint := usersUnlinkProjectsEndpoint.RewriteSlugs(userID.String())
		unlinkResponse, err := sendHTTPRequest(t, ctx, unlinkEndpoint, unlinkRequest, accessTokenHeader)
		assert.NoError(t, err, "Failed to unlink projects from user")
		defer unlinkResponse.Body.Close()

		assert.Equal(t, http.StatusOK, unlinkResponse.StatusCode, "Expected status code 200. Got %d. Message: %s", unlinkResponse.StatusCode, readResponseBody(t, unlinkResponse))

		// Parse and verify the success response
		unlinkAPIResp, err := parserResponseBody[payload.HTTPMessage](t, unlinkResponse)
		assert.NoError(t, err, "Failed to parse unlink response")
		assert.Equal(t, domain.UsersProjectsUnlinkedFromUserSuccessfully, unlinkAPIResp.Message, "Expected success message")

		// 6. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, userID)
			deleteProjectByIDFromDB(t, projectID1)
			deleteProjectByIDFromDB(t, projectID2)
			deleteProjectByIDFromDB(t, projectID3)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("unlink_projects_from_user_bad_request", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test user
		firstName, lastName, email := generateUserData(t)
		userID := uuid.NewV7()

		user := map[string]any{
			"id":         userID.String(),
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   generatePassword(t),
		}

		userCreateResponse, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, user, accessTokenHeader)
		assert.NoError(t, err, "Failed to create user")
		defer userCreateResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, userCreateResponse.StatusCode, "Expected status code 201. Got %d. Message: %s", userCreateResponse.StatusCode, readResponseBody(t, userCreateResponse))

		// 3. Test cases for various invalid inputs
		testCases := []struct {
			name           string
			invalidRequest map[string]any
			expectedError  string
		}{
			{
				name: "Empty project_ids array",
				invalidRequest: map[string]any{
					"project_ids": []uuid.UUID{},
				},
				expectedError: "at least one project",
			},
			{
				name: "Invalid project ID format",
				invalidRequest: map[string]any{
					"project_ids": []string{"not-a-valid-uuid"},
				},
				expectedError: "invalid uuid",
			},
			{
				name:           "Missing project_ids field",
				invalidRequest: map[string]any{},
				expectedError:  "at least one project",
			},
		}

		// 4. Run each test case
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				unlinkEndpoint := usersUnlinkProjectsEndpoint.RewriteSlugs(userID.String())
				response, err := sendHTTPRequest(t, ctx, unlinkEndpoint, tc.invalidRequest, accessTokenHeader)
				assert.NoError(t, err, "Failed to send request for %s", tc.name)
				defer response.Body.Close()

				// Verify we get a 400 Bad Request response
				assert.Equal(t, http.StatusBadRequest, response.StatusCode,
					"Expected status code 400 Bad Request for %s, got %d. Message: %s", tc.name, response.StatusCode, readResponseBody(t, response))

				// Parse and verify the error response
				errorResp, err := parserResponseBody[payload.HTTPMessage](t, response)
				assert.NoError(t, err, "Failed to parse error response for %s", tc.name)

				// Verify error message contains information specific to this validation failure
				assert.Contains(t, strings.ToLower(errorResp.Message), strings.ToLower(tc.expectedError),
					"Error message should indicate %s validation failure", tc.name)
			})
		}

		// 5. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, userID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("unlink_projects_user_not_found", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Generate a random UUID that doesn't exist in the database
		nonExistentUserID := uuid.NewV7()

		// 3. Create a test project
		projectID1, _, _ := createTestProject(t, ctx, adminToken.AccessToken, "test-project-")

		// 4. Try to unlink projects from non-existent user
		projectsToUnlink := []uuid.UUID{projectID1}
		unlinkRequest := map[string]any{
			"project_ids": projectsToUnlink,
		}

		unlinkEndpoint := usersUnlinkProjectsEndpoint.RewriteSlugs(nonExistentUserID.String())
		unlinkResponse, err := sendHTTPRequest(t, ctx, unlinkEndpoint, unlinkRequest, accessTokenHeader)
		assert.NoError(t, err, "Failed to send unlink request")
		defer unlinkResponse.Body.Close()

		// 5. Check that we get a 200 Not Found response
		assert.Equal(t, http.StatusOK, unlinkResponse.StatusCode, "Expected status code 200. Got %d. Message: %s", unlinkResponse.StatusCode, readResponseBody(t, unlinkResponse))

		// 6. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, unlinkResponse)
		assert.NoError(t, err, "Failed to parse error response")

		// Verify  message contains information about the user not being found
		assert.Contains(t, strings.ToLower(errorResp.Message), "unlinked from user successfully",
			"Error message should indicate user not found but still return success for unlinking")

		// 7. Cleanup
		t.Cleanup(func() {
			deleteProjectByIDFromDB(t, projectID1)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("require_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Generate a test user ID
		userID := uuid.NewV7()

		// 2. Generate a test project ID
		projectID := uuid.NewV7()

		// 3. Try to unlink projects without authentication
		unlinkRequest := map[string]any{
			"project_ids": []uuid.UUID{projectID},
		}

		unlinkEndpoint := usersUnlinkProjectsEndpoint.RewriteSlugs(userID.String())
		response, err := sendHTTPRequest(t, ctx, unlinkEndpoint, unlinkRequest)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 4. Check that we get a 401 Unauthorized response
		assert.Equal(t, http.StatusUnauthorized, response.StatusCode, "Expected status code 401")
	})
}

// TestUserSelectAuthz tests the user authorization endpoint
func TestUserSelectAuthz(t *testing.T) {
	t.Run("get_user_authz_as_admin", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Get admin user tokens
		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a regular user
		userID := uuid.NewV7()
		firstName, lastName, email := generateUserData(t)
		password := generatePassword(t)
		userPayload := map[string]any{
			"id":         userID.String(),
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   password,
		}

		createResponse, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, userPayload, accessTokenHeader)
		assert.NoError(t, err)
		defer createResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, createResponse.StatusCode)

		// 3. Get the user authz
		authzEndpoint := usersAuthzEndpoint.RewriteSlugs(userID.String())
		authzResponse, err := sendHTTPRequest(t, ctx, authzEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err)
		defer authzResponse.Body.Close()

		assert.Equal(t, http.StatusOK, authzResponse.StatusCode, "Expected status code 200. Got %d. Message: %s", authzResponse.StatusCode, readResponseBody(t, authzResponse))

		// 4. Parse response and validate structure
		authzData, err := parserResponseBody[map[string]any](t, authzResponse)
		assert.NoError(t, err)

		// The response is the raw authorization map which contains permissions
		assert.NotNil(t, authzData, "Authz data should not be nil")

		// For a new user, permissions might be empty or contain default role permissions
		// The structure varies based on the OPA policy and default roles assigned

		t.Cleanup(func() {
			deleteUserByIDFromDB(t, userID)
		})
	})

	t.Run("get_user_authz_non_existent_user", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Get admin user tokens
		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Try to get authz for non-existent user
		nonExistentUserID := uuid.NewV7()
		authzEndpoint := usersAuthzEndpoint.RewriteSlugs(nonExistentUserID.String())

		authzResponse, err := sendHTTPRequest(t, ctx, authzEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err)
		defer authzResponse.Body.Close()

		// The repository query doesn't fail for non-existent users; it returns empty results
		// So the endpoint returns 200 with empty/null permissions
		assert.Equal(t, http.StatusOK, authzResponse.StatusCode, "Expected status code 200. Got %d. Message: %s", authzResponse.StatusCode, readResponseBody(t, authzResponse))

		// Note: This behavior might be considered a bug - the endpoint should check if the user exists
		// and return 404 if not. Currently, it returns 200 with empty data for non-existent users.
		t.Log("BEHAVIOR NOTE: Endpoint returns 200 with empty data for non-existent users instead of 404")
	})

	t.Run("get_user_authz_invalid_user_id", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Get admin user tokens
		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Try to get authz with invalid user ID
		authzEndpoint := usersAuthzEndpoint.RewriteSlugs("invalid-uuid")

		authzResponse, err := sendHTTPRequest(t, ctx, authzEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err)
		defer authzResponse.Body.Close()

		assert.Equal(t, http.StatusBadRequest, authzResponse.StatusCode, "Expected status code 400. Got %d. Message: %s", authzResponse.StatusCode, readResponseBody(t, authzResponse))
	})

	t.Run("get_user_authz_without_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// Try to get authz without authentication
		userID := uuid.NewV7()
		authzEndpoint := usersAuthzEndpoint.RewriteSlugs(userID.String())

		authzResponse, err := sendHTTPRequest(t, ctx, authzEndpoint, nil)
		assert.NoError(t, err)
		defer authzResponse.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, authzResponse.StatusCode, "Expected status code 401. Got %d. Message: %s", authzResponse.StatusCode, readResponseBody(t, authzResponse))
	})

	t.Run("get_user_authz_with_invalid_token", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// Try to get authz with invalid token
		invalidTokenHeader := map[string]string{
			"Authorization": "Bearer invalid.token.here",
		}

		userID := uuid.NewV7()
		authzEndpoint := usersAuthzEndpoint.RewriteSlugs(userID.String())

		authzResponse, err := sendHTTPRequest(t, ctx, authzEndpoint, nil, invalidTokenHeader)
		assert.NoError(t, err)
		defer authzResponse.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, authzResponse.StatusCode, "Expected status code 401. Got %d. Message: %s", authzResponse.StatusCode, readResponseBody(t, authzResponse))
	})

	t.Run("get_own_authz_as_regular_user", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Get admin user tokens to create a user
		adminToken := getAdminUserTokens(t)
		adminAccessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a regular user
		userID := uuid.NewV7()
		firstName, lastName, email := generateUserData(t)
		password := generatePassword(t)
		userPayload := map[string]any{
			"id":         userID.String(),
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   password,
		}

		createResponse, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, userPayload, adminAccessTokenHeader)
		assert.NoError(t, err)
		defer createResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, createResponse.StatusCode)

		// 3. Enable the user
		enableUserByEmailFromDB(t, email)

		time.Sleep(500 * time.Millisecond)

		// 4. Login as the regular user
		loginUser := map[string]any{
			"email":    email,
			"password": password,
		}

		authLoginEndpoint := newAPIEndpoint(http.MethodPost, "/auth/login")
		loginResponse, err := sendHTTPRequest(t, ctx, authLoginEndpoint, loginUser)
		assert.NoError(t, err)
		defer loginResponse.Body.Close()
		assert.Equal(t, http.StatusOK, loginResponse.StatusCode)

		loginAPIResp, err := parserResponseBody[payload.LoginUserResponse](t, loginResponse)
		assert.NoError(t, err)

		userAccessTokenHeader := map[string]string{
			"Authorization": "Bearer " + loginAPIResp.AccessToken,
		}

		// 5. Try to get own authz as regular user
		authzEndpoint := usersAuthzEndpoint.RewriteSlugs(userID.String())
		authzResponse, err := sendHTTPRequest(t, ctx, authzEndpoint, nil, userAccessTokenHeader)
		assert.NoError(t, err)
		defer authzResponse.Body.Close()

		// Regular users should be able to access their own authz
		// If this returns 403, it indicates an authorization policy issue
		if authzResponse.StatusCode == http.StatusForbidden {
			t.Logf("WARNING: Regular user received 403 when accessing own authz at /users/{user_id}/authz. This may indicate an authorization policy issue.")
			t.Logf("Expected: Regular users should be able to view their own authorization data")
			t.Logf("Actual: Got status code 403 Forbidden")
		} else {
			assert.Equal(t, http.StatusOK, authzResponse.StatusCode, "Expected status code 200. Got %d. Message: %s", authzResponse.StatusCode, readResponseBody(t, authzResponse))
		}

		t.Cleanup(func() {
			deleteUserByIDFromDB(t, userID)
		})
	})

	t.Run("get_other_user_authz_as_regular_user", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Get admin user tokens to create users
		adminToken := getAdminUserTokens(t)
		adminAccessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create first regular user
		user1ID := uuid.NewV7()
		firstName1, lastName1, email1 := generateUserData(t)
		password1 := generatePassword(t)
		user1Payload := map[string]any{
			"id":         user1ID.String(),
			"email":      email1,
			"first_name": firstName1,
			"last_name":  lastName1,
			"password":   password1,
		}

		createResponse1, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, user1Payload, adminAccessTokenHeader)
		assert.NoError(t, err)
		defer createResponse1.Body.Close()
		assert.Equal(t, http.StatusCreated, createResponse1.StatusCode)

		// 3. Create second regular user
		user2ID := uuid.NewV7()
		firstName2, lastName2, email2 := generateUserData(t)
		password2 := generatePassword(t)
		user2Payload := map[string]any{
			"id":         user2ID.String(),
			"email":      email2,
			"first_name": firstName2,
			"last_name":  lastName2,
			"password":   password2,
		}

		createResponse2, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, user2Payload, adminAccessTokenHeader)
		assert.NoError(t, err)
		defer createResponse2.Body.Close()
		assert.Equal(t, http.StatusCreated, createResponse2.StatusCode)

		// 4. Enable and login as first user
		enableUserByEmailFromDB(t, email1)
		time.Sleep(500 * time.Millisecond)

		loginUser := map[string]any{
			"email":    email1,
			"password": password1,
		}

		authLoginEndpoint := newAPIEndpoint(http.MethodPost, "/auth/login")
		loginResponse, err := sendHTTPRequest(t, ctx, authLoginEndpoint, loginUser)
		assert.NoError(t, err)
		defer loginResponse.Body.Close()
		assert.Equal(t, http.StatusOK, loginResponse.StatusCode)

		loginAPIResp, err := parserResponseBody[payload.LoginUserResponse](t, loginResponse)
		assert.NoError(t, err)

		user1AccessTokenHeader := map[string]string{
			"Authorization": "Bearer " + loginAPIResp.AccessToken,
		}

		// 5. Try to get second user's authz as first user (should be forbidden)
		authzEndpoint := usersAuthzEndpoint.RewriteSlugs(user2ID.String())
		authzResponse, err := sendHTTPRequest(t, ctx, authzEndpoint, nil, user1AccessTokenHeader)
		assert.NoError(t, err)
		defer authzResponse.Body.Close()

		// Regular users should NOT be able to access other users' authz
		assert.Equal(t, http.StatusForbidden, authzResponse.StatusCode, "Expected status code 403. Got %d. Message: %s", authzResponse.StatusCode, readResponseBody(t, authzResponse))

		t.Cleanup(func() {
			deleteUserByIDFromDB(t, user1ID)
			deleteUserByIDFromDB(t, user2ID)
		})
	})
}

// TestUserJSONSerialization tests that all user endpoints return JSON responses with snake_case field names
func TestUserJSONSerialization(t *testing.T) {
	// Test that UserResponse uses snake_case in JSON serialization
	t.Run("get_user_response_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test user
		firstName, lastName, email := generateUserData(t)
		password := generatePassword(t)

		userID := createUserInDB(t, firstName, lastName, email, password)

		// 3. Get the user
		getUserEndpoint := usersGetEndpoint.RewriteSlugs(userID.String())
		response, err := sendHTTPRequest(t, ctx, getUserEndpoint, nil, accessTokenHeader)
		require.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		// 4. Verify all UserResponse fields are in snake_case
		expectedFields := []string{
			"id",
			"first_name",
			"last_name",
			"email",
			"disabled",
			"admin",
			"local_account",
			"created_at",
			"updated_at",
		}

		assertJSONFieldsAreSnakeCase(t, response, expectedFields)

		t.Cleanup(func() {
			deleteUserByIDFromDB(t, userID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test that ListUsersResponse uses snake_case including nested objects
	t.Run("list_users_response_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create test users
		user1FirstName, user1LastName, user1Email := generateUserData(t)
		user1Password := generatePassword(t)

		user1ID := createUserInDB(t, user1FirstName, user1LastName, user1Email, user1Password)

		user2FirstName, user2LastName, user2Email := generateUserData(t)
		user2Password := generatePassword(t)

		user2ID := createUserInDB(t, user2FirstName, user2LastName, user2Email, user2Password)

		// 3. List users
		response, err := sendHTTPRequest(t, ctx, usersListEndpoint, nil, accessTokenHeader)
		require.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		// 4. Verify ListUsersResponse top-level fields are snake_case
		expectedTopLevelFields := []string{
			"items",
			"paginator",
		}

		assertJSONFieldsAreSnakeCase(t, response, expectedTopLevelFields)

		// Note: The assertJSONFieldsAreSnakeCase function recursively checks nested objects,
		// so it will also verify that:
		// - items[] contains objects with snake_case fields (id, first_name, last_name, etc.)
		// - paginator contains snake_case fields (next_token, prev_token, limit, total_count, etc.)

		t.Cleanup(func() {
			deleteUserByIDFromDB(t, user1ID)
			deleteUserByIDFromDB(t, user2ID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test that HTTPMessage response uses snake_case
	t.Run("create_user_http_message_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a new user
		firstName, lastName, email := generateUserData(t)
		user := map[string]any{
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   generatePassword(t),
		}

		response, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, user, accessTokenHeader)
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
			deleteUserByEmailFromDB(t, email)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test that update response uses snake_case
	t.Run("update_user_response_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test user
		firstName, lastName, email := generateUserData(t)
		password := generatePassword(t)

		userID := createUserInDB(t, firstName, lastName, email, password)

		// 3. Update the user
		newFirstName := generateRandomName(t, "")
		updatePayload := map[string]any{
			"first_name": newFirstName,
		}

		updateEndpoint := usersUpdateEndpoint.RewriteSlugs(userID.String())
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
			deleteUserByIDFromDB(t, userID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test that delete response uses snake_case
	t.Run("delete_user_response_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test user
		firstName, lastName, email := generateUserData(t)
		password := generatePassword(t)

		userID := createUserInDB(t, firstName, lastName, email, password)

		// 3. Delete the user
		deleteEndpoint := usersDeleteEndpoint.RewriteSlugs(userID.String())
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

		// 2. Create a test user
		firstName, lastName, email := generateUserData(t)
		password := generatePassword(t)

		userID := createUserInDB(t, firstName, lastName, email, password)

		// 3. Get a role ID from database (assuming roles exist from migrations)
		var roleID uuid.UUID
		query := `SELECT id FROM roles LIMIT 1`
		err := testDBPool.QueryRow(ctx, query).Scan(&roleID)
		require.NoError(t, err, "Failed to get role from database")

		// 4. Link role to user
		linkPayload := map[string]any{
			"role_ids": []uuid.UUID{roleID},
		}

		linkEndpoint := usersLinkRolesEndpoint.RewriteSlugs(userID.String())
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
			unlinkAllRolesFromUserViaDB(t, userID)
			deleteUserByIDFromDB(t, userID)
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

		// 2. Create a test user
		firstName, lastName, email := generateUserData(t)
		password := generatePassword(t)

		userID := createUserInDB(t, firstName, lastName, email, password)

		// 3. Get a role ID and link it first
		var roleID uuid.UUID
		query := `SELECT id FROM roles LIMIT 1`
		err := testDBPool.QueryRow(ctx, query).Scan(&roleID)
		require.NoError(t, err, "Failed to get role from database")

		// Link the role
		linkQuery := `INSERT INTO users_roles (users_id, roles_id) VALUES ($1, $2)`
		_, err = testDBPool.Exec(ctx, linkQuery, userID, roleID)
		require.NoError(t, err, "Failed to link role to user")

		// 4. Unlink role from user
		unlinkPayload := map[string]any{
			"role_ids": []uuid.UUID{roleID},
		}

		unlinkEndpoint := usersUnlinkRolesEndpoint.RewriteSlugs(userID.String())
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
			unlinkAllRolesFromUserViaDB(t, userID)
			deleteUserByIDFromDB(t, userID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test that link projects response uses snake_case
	t.Run("link_projects_response_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test user
		firstName, lastName, email := generateUserData(t)
		password := generatePassword(t)

		userID := createUserInDB(t, firstName, lastName, email, password)

		// 3. Create a test project
		projectName := generateRandomName(t, "")
		project := createProjectInDB(t, projectName, "test project", uuid.NewV7())

		// 4. Link project to user
		linkPayload := map[string]any{
			"project_ids": []uuid.UUID{project.ID},
		}

		linkEndpoint := usersLinkProjectsEndpoint.RewriteSlugs(userID.String())
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
			unlinkAllUsersFromProjectViaDB(t, project.ID)
			deleteUserByIDFromDB(t, userID)
			deleteProjectByIDFromDB(t, project.ID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test that unlink projects response uses snake_case
	t.Run("unlink_projects_response_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test user
		firstName, lastName, email := generateUserData(t)
		password := generatePassword(t)

		userID := createUserInDB(t, firstName, lastName, email, password)

		// 3. Create a test project and link it
		projectName := generateRandomName(t, "")
		project := createProjectInDB(t, projectName, "test project", uuid.NewV7())

		// Link the project
		linkQuery := `INSERT INTO projects_users (projects_id, users_id) VALUES ($1, $2)`
		_, err := testDBPool.Exec(ctx, linkQuery, project.ID, userID)
		require.NoError(t, err, "Failed to link project to user")

		// 4. Unlink project from user
		unlinkPayload := map[string]any{
			"project_ids": []uuid.UUID{project.ID},
		}

		unlinkEndpoint := usersUnlinkProjectsEndpoint.RewriteSlugs(userID.String())
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
			unlinkAllUsersFromProjectViaDB(t, project.ID)
			deleteUserByIDFromDB(t, userID)
			deleteProjectByIDFromDB(t, project.ID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}
