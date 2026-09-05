//go:build integration

package integration

import (
	"context"
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
	projectsCreateEndpoint = newAPIEndpoint(http.MethodPost, "/projects")
	projectsListEndpoint   = newAPIEndpoint(http.MethodGet, "/projects")
	projectsGetEndpoint    = newAPIEndpoint(http.MethodGet, "/projects/{project_id}")
	projectsUpdateEndpoint = newAPIEndpoint(http.MethodPut, "/projects/{project_id}")
	projectsDeleteEndpoint = newAPIEndpoint(http.MethodDelete, "/projects/{project_id}")

	projectsLinkUsersEndpoint   = newAPIEndpoint(http.MethodPost, "/projects/{project_id}/users")
	projectsUnlinkUsersEndpoint = newAPIEndpoint(http.MethodDelete, "/projects/{project_id}/users")

	projectsListByUserIDEndpoint = newAPIEndpoint(http.MethodGet, "/users/{user_id}/projects")
)

// Helper function to create a test project
func createTestProject(t *testing.T, ctx context.Context, accessToken, namePrefix string) (uuid.UUID, string, string) {
	t.Helper()

	projectID := uuid.NewV7()
	projectName := namePrefix + projectID.String()
	projectDesc := "Test project " + projectID.String()
	project := map[string]any{
		"id":          projectID.String(),
		"name":        projectName,
		"description": projectDesc,
	}

	accessTokenHeader := map[string]string{"Authorization": "Bearer " + accessToken}

	response, err := sendHTTPRequest(t, ctx, projectsCreateEndpoint, project, accessTokenHeader)
	assert.NoError(t, err, "Failed to create test project")
	if response != nil {
		defer response.Body.Close()
		assert.Equal(t, http.StatusCreated, response.StatusCode, "Failed to create test project, status code not 201")
	}

	return projectID, projectName, projectDesc
}

func TestProjectCreate(t *testing.T) {
	t.Run("create_project", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a new project
		projectID := uuid.NewV7()

		project := map[string]any{
			"id":          projectID.String(),
			"name":        "test_project_" + projectID.String(),
			"description": "This is a test project " + projectID.String(),
		}

		response, err := sendHTTPRequest(t, ctx, projectsCreateEndpoint, project, accessTokenHeader)
		assert.NoError(t, err, "Error sending request: %v", err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusCreated, response.StatusCode, "Expected status code 201 Created. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		t.Cleanup(func() {
			deleteProjectByIDFromDB(t, projectID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})

		assert.Equal(t, domain.ProjectsProjectCreatedSuccessfully, apiResp.Message, "Unexpected response message")
		assert.Equal(t, projectsCreateEndpoint.method, apiResp.Method, "Expected method to be set")
		assert.Equal(t, projectsCreateEndpoint.Path(), apiResp.Path, "Expected path to be set")
	})

	// Test creating a project with invalid data format
	t.Run("create_project_bad_request", func(t *testing.T) {
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
			name           string
			invalidProject map[string]any
			expectedError  string
		}{
			{
				name: "Empty project name",
				invalidProject: map[string]any{
					"id":          mustUUIDString(t),
					"name":        "",
					"description": "Project with empty name",
				},
				expectedError: "cannot be empty",
			},
			{
				name: "Name too long",
				invalidProject: map[string]any{
					"id":          mustUUIDString(t),
					"name":        string(make([]byte, 256)), // Very long name
					"description": "Project with too long name",
				},
				expectedError: "must be between",
			},
			{
				name: "Invalid ID format",
				invalidProject: map[string]any{
					"id":          "not-a-valid-uuid",
					"name":        "Valid Project Name",
					"description": "Project with invalid ID",
				},
				expectedError: "invalid uuid",
			},
			{
				name: "Description too long",
				invalidProject: map[string]any{
					"id":          mustUUIDString(t),
					"name":        "Valid Name",
					"description": string(make([]byte, 5000)), // Very long description
				},
				expectedError: "must be between",
			},
		}

		// 3. Run each test case
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Send request with the invalid project data
				response, err := sendHTTPRequest(t, ctx, projectsCreateEndpoint, tc.invalidProject, accessTokenHeader)
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
				assert.Equal(t, projectsCreateEndpoint.method, errorResp.Method, "Expected method to be set")
				assert.Equal(t, projectsCreateEndpoint.Path(), errorResp.Path, "Expected path to be set")
			})
		}

		// 4. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("require_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		project := map[string]any{
			"id":          "00000000-0000-0000-0000-000000000000",
			"name":        "Test Project",
			"description": "Test project description",
		}

		resp, err := sendHTTPRequest(t, ctx, projectsCreateEndpoint, project)
		assert.NoError(t, err, "Failed to send request without authentication")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

func TestProjectGet(t *testing.T) {
	t.Run("get_project", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a new project
		projectID, projectName, projectDesc := createTestProject(t, ctx, adminToken.AccessToken, "test_")
		assert.NotEmpty(t, projectID, "Project ID should not be empty")
		assert.NotEmpty(t, projectName, "Project name should not be empty")
		assert.NotEmpty(t, projectDesc, "Project description should not be empty")

		// 3. Get the project
		getEndpoint := projectsGetEndpoint.RewriteSlugs(projectID.String())
		getResponse, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Error sending get request: %v", err)
		defer getResponse.Body.Close()

		// 4. Check the response
		assert.Equal(t, http.StatusOK, getResponse.StatusCode, "Expected status code 200 OK for get. Got %d. Message: %s", getResponse.StatusCode, readResponseBody(t, getResponse))
		getAPIResp, err := parserResponseBody[domain.Project](t, getResponse)
		assert.NoError(t, err, "Failed to parse get response body", err)

		assert.Equal(t, projectID, getAPIResp.ID, "Expected project ID to match")
		assert.Equal(t, projectName, getAPIResp.Name, "Expected project name to match")
		assert.Equal(t, projectDesc, getAPIResp.Description, "Expected project description to match")
		assert.Equal(t, new(false), getAPIResp.Disabled, "Expected disabled to be false")
		assert.Equal(t, new(false), getAPIResp.System, "Expected system to be false")

		// 5. Cleanup
		t.Cleanup(func() {
			deleteProjectByIDFromDB(t, projectID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test retrieving a non-existent project
	t.Run("get_project_not_found", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Generate a random UUID that doesn't exist in the database
		nonExistentProjectID := uuid.NewV7()

		// 3. Try to get the non-existent project
		getEndpoint := projectsGetEndpoint.RewriteSlugs(nonExistentProjectID.String())
		getResponse, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to get non-existent project")
		defer getResponse.Body.Close()

		// 4. Check that we get a 404 Not Found response
		assert.Equal(t, http.StatusNotFound, getResponse.StatusCode, "Expected status code 404 for non-existent project. Got %d. Message: %s", getResponse.StatusCode, readResponseBody(t, getResponse))

		// 5. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, getResponse)
		assert.NoError(t, err, "Failed to parse error response")

		// Verify error message contains information about the project not being found
		assert.Contains(t, errorResp.Message, "not found", "Error message should indicate that the project was not found")
		assert.Equal(t, getEndpoint.method, errorResp.Method, "Expected method to be set")
		assert.Equal(t, getEndpoint.Path(), errorResp.Path, "Expected path to be set")

		// 6. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test retrieving a project with an invalid ID format
	t.Run("get_project_bad_request", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Try to get a project with an invalid ID format (not a UUID)
		invalidProjectID := "not-a-valid-uuid"
		getEndpoint := projectsGetEndpoint.RewriteSlugs(invalidProjectID)
		getResponse, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to get project with invalid ID")
		defer getResponse.Body.Close()

		// 3. Check that we get a 400 Bad Request response
		assert.Equal(t, http.StatusBadRequest, getResponse.StatusCode, "Expected status code 400 for invalid project ID format. Got %d. Message: %s", getResponse.StatusCode, readResponseBody(t, getResponse))

		// 4. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, getResponse)
		assert.NoError(t, err, "Failed to parse error response")

		// Verify error message contains information about the invalid UUID format
		assert.Contains(t, errorResp.Message, "invalid", "Error message should indicate that the project ID format is invalid")
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

		getEndpoint := projectsGetEndpoint.RewriteSlugs("00000000-0000-0000-0000-000000000000")
		resp, err := sendHTTPRequest(t, ctx, getEndpoint, nil)
		assert.NoError(t, err, "Failed to send request without authentication")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

func TestProjectDelete(t *testing.T) {
	t.Run("delete_project", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a new project
		projectID, _, _ := createTestProject(t, ctx, adminToken.AccessToken, "test_")
		assert.NotEmpty(t, projectID, "Project ID should not be empty")

		// 3. Delete the project
		deleteEndpoint := projectsDeleteEndpoint.RewriteSlugs(projectID.String())
		deleteResponse, err := sendHTTPRequest(t, ctx, deleteEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Error sending delete request: %v", err)
		defer deleteResponse.Body.Close()

		// 4. Check the delete response
		assert.Equal(t, http.StatusOK, deleteResponse.StatusCode, "Expected status code 200 OK for delete. Got %d. Message: %s", deleteResponse.StatusCode, readResponseBody(t, deleteResponse))
		deleteAPIResp, err := parserResponseBody[payload.HTTPMessage](t, deleteResponse)
		assert.NoError(t, err, "Failed to parse delete response body")

		assert.Equal(t, domain.ProjectsProjectDeletedSuccessfully, deleteAPIResp.Message, "Unexpected delete response message")
		assert.Equal(t, deleteEndpoint.method, deleteAPIResp.Method, "Expected method to be set for delete")
		assert.Equal(t, deleteEndpoint.Path(), deleteAPIResp.Path, "Expected path to be set for delete")

		// 5. Verify project is actually deleted (try to get it)
		getEndpoint := projectsGetEndpoint.RewriteSlugs(projectID.String())
		getResponse, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Error sending get request after delete: %v", err)
		defer getResponse.Body.Close()
		assert.Equal(t, http.StatusNotFound, getResponse.StatusCode, "Expected status code 404 Not Found after deletion. Got %d. Message: %s", getResponse.StatusCode, readResponseBody(t, getResponse))

		// 6. Cleanup admin user
		t.Cleanup(func() {
			// Project should already be deleted by the test, but try again just in case
			deleteProjectByIDFromDB(t, projectID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test deleting a project with an invalid ID format
	t.Run("delete_project_bad_request", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Try to delete a project with an invalid ID format (not a UUID)
		invalidProjectID := "not-a-valid-uuid"
		deleteEndpoint := projectsDeleteEndpoint.RewriteSlugs(invalidProjectID)

		deleteResponse, err := sendHTTPRequest(t, ctx, deleteEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to delete project with invalid ID")
		defer deleteResponse.Body.Close()

		// 3. Check that we get a 400 Bad Request response
		assert.Equal(t, http.StatusBadRequest, deleteResponse.StatusCode, "Expected status code 400 for invalid project ID format. Got %d. Message: %s", deleteResponse.StatusCode, readResponseBody(t, deleteResponse))

		// 4. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, deleteResponse)
		assert.NoError(t, err, "Failed to parse error response")

		// 5. Verify error message contains information about the invalid UUID format
		assert.Contains(t, errorResp.Message, "invalid", "Error message should indicate that the project ID format is invalid")
		assert.Equal(t, deleteEndpoint.method, errorResp.Method, "Expected method to be set")
		assert.Equal(t, deleteEndpoint.Path(), errorResp.Path, "Expected path to be set")

		// 6. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test deleting a non-existent project
	t.Run("delete_project_not_found", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Generate a UUID that doesn't exist in the database
		nonExistentProjectID := uuid.NewV7()
		deleteEndpoint := projectsDeleteEndpoint.RewriteSlugs(nonExistentProjectID.String())

		deleteResponse, err := sendHTTPRequest(t, ctx, deleteEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to delete non-existent project")
		defer deleteResponse.Body.Close()

		// 3. Check the response - this should still return StatusOK even though the project doesn't exist
		// This is because deleting a non-existent resource is considered idempotent in RESTful APIs
		assert.Equal(t, http.StatusOK, deleteResponse.StatusCode, "Expected status code 200 OK for deleting non-existent project. Got %d. Message: %s", deleteResponse.StatusCode, readResponseBody(t, deleteResponse))

		// 4. Parse and verify the success response
		deleteAPIResp, err := parserResponseBody[payload.HTTPMessage](t, deleteResponse)
		assert.NoError(t, err, "Failed to parse response body")

		// 5. Verify success message for deletion
		assert.Equal(t, domain.ProjectsProjectDeletedSuccessfully, deleteAPIResp.Message, "Expected success message")
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

		deleteEndpoint := projectsDeleteEndpoint.RewriteSlugs("00000000-0000-0000-0000-000000000000")
		resp, err := sendHTTPRequest(t, ctx, deleteEndpoint, nil)
		assert.NoError(t, err, "Failed to send request without authentication")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

func TestProjectUpdate(t *testing.T) {
	t.Run("update_project", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a new project
		projectID, originalName, _ := createTestProject(t, ctx, adminToken.AccessToken, "t_")

		// 3. Update the project
		updatedName := "updated_" + originalName
		updatedDesc := "Updated description " + projectID.String()
		disabled := true
		updatedProject := map[string]any{
			"name":        updatedName,
			"description": updatedDesc,
			"disabled":    disabled,
		}

		updateEndpoint := projectsUpdateEndpoint.RewriteSlugs(projectID.String())
		updateResponse, err := sendHTTPRequest(t, ctx, updateEndpoint, updatedProject, accessTokenHeader)
		assert.NoError(t, err, "Error sending update request: %v", err)
		defer updateResponse.Body.Close()

		// 4. Check the update response
		assert.Equal(t, http.StatusOK, updateResponse.StatusCode, "Expected status code 200 OK for update. Got %d. Message: %s", updateResponse.StatusCode, readResponseBody(t, updateResponse))
		updateAPIResp, err := parserResponseBody[payload.HTTPMessage](t, updateResponse)
		assert.NoError(t, err, "Failed to parse update response body")

		assert.Equal(t, domain.ProjectsProjectUpdatedSuccessfully, updateAPIResp.Message, "Unexpected update response message")
		assert.Equal(t, updateEndpoint.method, updateAPIResp.Method, "Expected method to be set for update")
		assert.Equal(t, updateEndpoint.Path(), updateAPIResp.Path, "Expected path to be set for update")

		// 5. Verify project is actually updated (get it again)
		getEndpoint := projectsGetEndpoint.RewriteSlugs(projectID.String())
		getResponse, err := sendHTTPRequest(t, ctx, getEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Error sending get request after update: %v", err)
		defer getResponse.Body.Close()
		assert.Equal(t, http.StatusOK, getResponse.StatusCode, "Expected status code 200 OK when getting updated project. Got %d. Message: %s", getResponse.StatusCode, readResponseBody(t, getResponse))

		getAPIResp, err := parserResponseBody[domain.Project](t, getResponse)
		assert.NoError(t, err, "Failed to parse get response body for updated project")

		assert.Equal(t, projectID, getAPIResp.ID, "Expected project ID to remain the same")
		assert.Equal(t, updatedName, getAPIResp.Name, "Expected project name to be updated")
		assert.Equal(t, updatedDesc, getAPIResp.Description, "Expected project description to be updated")
		assert.Equal(t, new(disabled), getAPIResp.Disabled, "Expected disabled to be updated")

		// 6. Cleanup
		t.Cleanup(func() {
			deleteProjectByIDFromDB(t, projectID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test updating a project with invalid data format
	t.Run("update_project_bad_request", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a valid project first
		projectID, _, _ := createTestProject(t, ctx, adminToken.AccessToken, "bad_req_test_")

		// 3. Set up test cases for different bad request scenarios
		testCases := []struct {
			name          string
			updateData    map[string]any
			expectedCode  int
			expectedError string
		}{
			{
				name:          "Empty project name",
				updateData:    map[string]any{"name": ""},
				expectedCode:  http.StatusBadRequest,
				expectedError: "cannot be empty",
			},
			{
				name:          "Name too long",
				updateData:    map[string]any{"name": string(make([]byte, 256))}, // Very long name
				expectedCode:  http.StatusBadRequest,
				expectedError: "must be between",
			},
			{
				name:          "Description too long",
				updateData:    map[string]any{"description": string(make([]byte, 5000))}, // Very long description
				expectedCode:  http.StatusBadRequest,
				expectedError: "must be between",
			},
		}

		// 4. Run each test case
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				updateEndpoint := projectsUpdateEndpoint.RewriteSlugs(projectID.String())
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

		// 5. Cleanup
		t.Cleanup(func() {
			deleteProjectByIDFromDB(t, projectID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test updating a non-existent project
	t.Run("update_project_not_found", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Generate a UUID that doesn't exist in the database
		nonExistentProjectID := uuid.NewV7()
		updateEndpoint := projectsUpdateEndpoint.RewriteSlugs(nonExistentProjectID.String())

		updatedProject := map[string]any{
			"name":        "UpdatedProjectName",
			"description": "Updated project description",
			"disabled":    true,
		}

		updateResponse, err := sendHTTPRequest(t, ctx, updateEndpoint, updatedProject, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to update non-existent project")
		defer updateResponse.Body.Close()

		// 3. Check that we get a 404 Not Found response
		assert.Equal(t, http.StatusNotFound, updateResponse.StatusCode, "Expected status code 404 for non-existent project. Got %d. Message: %s", updateResponse.StatusCode, readResponseBody(t, updateResponse))

		// 4. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, updateResponse)
		assert.NoError(t, err, "Failed to parse error response")

		// 5. Verify error message contains information about the project not being found
		assert.Contains(t, errorResp.Message, "not found", "Error message should indicate that the project was not found")
		assert.Equal(t, updateEndpoint.method, errorResp.Method, "Expected method to be set")
		assert.Equal(t, updateEndpoint.Path(), errorResp.Path, "Expected path to be set")

		// 6. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("require_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		updateEndpoint := projectsUpdateEndpoint.RewriteSlugs("00000000-0000-0000-0000-000000000000")
		updateData := map[string]any{
			"name":        "Updated Project Name",
			"description": "Updated project description",
		}

		resp, err := sendHTTPRequest(t, ctx, updateEndpoint, updateData)
		assert.NoError(t, err, "Failed to send request without authentication")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

func TestProjectList(t *testing.T) {
	t.Run("list_projects", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a couple of new projects
		var projectIDs []uuid.UUID
		for i := 0; i < 6; i++ {
			id := uuid.NewV7()
			projectIDs = append(projectIDs, id)
		}
		projectsToCreate := []map[string]any{}

		for i, projectID := range projectIDs {
			project := map[string]any{
				"id":          projectID.String(),
				"name":        "test_" + projectID.String(),
				"description": "This is a test project for list " + projectID.String(),
			}
			projectsToCreate = append(projectsToCreate, project)

			createResponse, err := sendHTTPRequest(t, ctx, projectsCreateEndpoint, project, accessTokenHeader)
			assert.NoError(t, err, "Failed to send request to create project %d", i+1)

			if createResponse != nil {
				defer createResponse.Body.Close()
				assert.Equal(t, http.StatusCreated, createResponse.StatusCode, "Expected status code 201 for project %d. Got %d. Message: %s", i+1, createResponse.StatusCode, readResponseBody(t, createResponse))
			}
		}

		// Wait briefly to ensure projects are created
		time.Sleep(500 * time.Millisecond)

		// 3. List the projects
		listResponse, err := sendHTTPRequest(t, ctx, projectsListEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to list projects")
		defer listResponse.Body.Close()

		// 4. Check the list response
		assert.Equal(t, http.StatusOK, listResponse.StatusCode, "Expected status code 200 for list. Got %d. Message: %s", listResponse.StatusCode, readResponseBody(t, listResponse))
		// Assuming domain.ListProjectsOutput exists and has an Items field []domain.Project
		listAPIResp, err := parserResponseBody[domain.ListProjectsOutput](t, listResponse)
		assert.NoError(t, err, "Failed to parse list response body")

		// 5. Verify the created projects are in the list
		foundCount := 0
		projectMap := make(map[string]bool) // Use name for checking presence
		for _, createdProject := range projectsToCreate {
			projectMap[createdProject["name"].(string)] = true
		}

		for _, listedProject := range listAPIResp.Items {
			if _, ok := projectMap[listedProject.Name]; ok {
				foundCount++
				// Optionally assert other fields match
				for _, createdProject := range projectsToCreate {
					if createdProject["name"] == listedProject.Name {
						assert.Equal(t, createdProject["description"], listedProject.Description)
						break
					}
				}
			}
		}
		assert.GreaterOrEqual(t, foundCount, len(projectsToCreate), "Expected to find at least the created projects in the list")

		// 6. Cleanup
		t.Cleanup(func() {
			for _, projectID := range projectIDs {
				deleteProjectByIDFromDB(t, projectID)
			}
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("require_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		resp, err := sendHTTPRequest(t, ctx, projectsListEndpoint, nil)
		assert.NoError(t, err, "Failed to send request without authentication")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

func TestProjectLinkUsers(t *testing.T) {
	t.Run("link_users_to_project", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test project
		projectID, _, _ := createTestProject(t, ctx, adminToken.AccessToken, "test-project-")

		// 3. Create test users
		firstName1, lastName1, email1 := generateUserData(t)
		userID1 := uuid.NewV7()

		user1 := map[string]any{
			"id":         userID1.String(),
			"email":      email1,
			"first_name": firstName1,
			"last_name":  lastName1,
			"password":   generatePassword(t),
		}

		user1CreateResponse, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, user1, accessTokenHeader)
		assert.NoError(t, err, "Failed to create user1")
		defer user1CreateResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, user1CreateResponse.StatusCode, "Expected status code 201")

		firstName2, lastName2, email2 := generateUserData(t)
		userID2 := uuid.NewV7()

		user2 := map[string]any{
			"id":         userID2.String(),
			"email":      email2,
			"first_name": firstName2,
			"last_name":  lastName2,
			"password":   generatePassword(t),
		}

		user2CreateResponse, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, user2, accessTokenHeader)
		assert.NoError(t, err, "Failed to create user2")
		defer user2CreateResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, user2CreateResponse.StatusCode, "Expected status code 201")

		// 4. Link users to project
		usersToLink := []uuid.UUID{userID1, userID2}
		linkRequest := map[string]any{
			"user_ids": usersToLink,
		}

		linkEndpoint := projectsLinkUsersEndpoint.RewriteSlugs(projectID.String())
		linkResponse, err := sendHTTPRequest(t, ctx, linkEndpoint, linkRequest, accessTokenHeader)
		assert.NoError(t, err, "Failed to link users to project")
		defer linkResponse.Body.Close()

		assert.Equal(t, http.StatusOK, linkResponse.StatusCode, "Expected status code 200")

		// Parse and verify the success response
		linkAPIResp, err := parserResponseBody[payload.HTTPMessage](t, linkResponse)
		assert.NoError(t, err, "Failed to parse link response")
		assert.Equal(t, domain.ProjectsUsersLinkedSuccessfully, linkAPIResp.Message, "Expected success message")

		// 5. Cleanup (LIFO order: last registered runs first)
		t.Cleanup(func() { deleteUserByIDFromDB(t, adminToken.UserID) })
		t.Cleanup(func() { deleteUserByIDFromDB(t, userID2) })
		t.Cleanup(func() { deleteUserByIDFromDB(t, userID1) })
		t.Cleanup(func() { deleteProjectByIDFromDB(t, projectID) })
		t.Cleanup(func() { unlinkAllUsersFromProjectViaDB(t, projectID) })
	})

	t.Run("link_users_to_project_bad_request", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test project
		projectID, _, _ := createTestProject(t, ctx, adminToken.AccessToken, "test-project-")

		// 3. Test cases for various invalid inputs
		testCases := []struct {
			name           string
			invalidRequest map[string]any
			expectedError  string
		}{
			{
				name: "Empty user_ids array",
				invalidRequest: map[string]any{
					"user_ids": []uuid.UUID{},
				},
				expectedError: "at least one user",
			},
			{
				name: "Invalid user ID format",
				invalidRequest: map[string]any{
					"user_ids": []string{"not-a-valid-uuid"},
				},
				expectedError: "invalid uuid",
			},
			{
				name:           "Missing user_ids field",
				invalidRequest: map[string]any{},
				expectedError:  "at least one user",
			},
		}

		// 4. Run each test case
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				linkEndpoint := projectsLinkUsersEndpoint.RewriteSlugs(projectID.String())
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
			deleteProjectByIDFromDB(t, projectID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("link_users_project_not_found", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Generate a random UUID that doesn't exist in the database
		nonExistentProjectID := uuid.NewV7()

		// 3. Create a test user
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

		// 4. Try to link users to non-existent project
		usersToLink := []uuid.UUID{userID}
		linkRequest := map[string]any{
			"user_ids": usersToLink,
		}

		linkEndpoint := projectsLinkUsersEndpoint.RewriteSlugs(nonExistentProjectID.String())
		linkResponse, err := sendHTTPRequest(t, ctx, linkEndpoint, linkRequest, accessTokenHeader)
		assert.NoError(t, err, "Failed to send link request")
		defer linkResponse.Body.Close()

		// 5. Check that we get a 404 Not Found response
		assert.Equal(t, http.StatusNotFound, linkResponse.StatusCode, "Expected status code 404")

		// 6. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, linkResponse)
		assert.NoError(t, err, "Failed to parse error response")

		// Verify error message contains information about the project not being found
		assert.Contains(t, strings.ToLower(errorResp.Message), "not found",
			"Error message should indicate project not found")

		// 7. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, userID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("require_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Generate a test project ID
		projectID := uuid.NewV7()

		// 2. Generate a test user ID
		userID := uuid.NewV7()

		// 3. Try to link users without authentication
		linkRequest := map[string]any{
			"user_ids": []uuid.UUID{userID},
		}

		linkEndpoint := projectsLinkUsersEndpoint.RewriteSlugs(projectID.String())
		response, err := sendHTTPRequest(t, ctx, linkEndpoint, linkRequest)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 4. Check that we get a 401 Unauthorized response
		assert.Equal(t, http.StatusUnauthorized, response.StatusCode, "Expected status code 401")
	})
}

func TestProjectUnlinkUsers(t *testing.T) {
	t.Run("unlink_users_from_project", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test project
		projectID, _, _ := createTestProject(t, ctx, adminToken.AccessToken, "test-project-")

		// 3. Create test users
		firstName1, lastName1, email1 := generateUserData(t)
		userID1 := uuid.NewV7()

		user1 := map[string]any{
			"id":         userID1.String(),
			"email":      email1,
			"first_name": firstName1,
			"last_name":  lastName1,
			"password":   generatePassword(t),
		}

		user1CreateResponse, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, user1, accessTokenHeader)
		assert.NoError(t, err, "Failed to create user1")
		defer user1CreateResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, user1CreateResponse.StatusCode, "Expected status code 201")

		firstName2, lastName2, email2 := generateUserData(t)
		userID2 := uuid.NewV7()

		user2 := map[string]any{
			"id":         userID2.String(),
			"email":      email2,
			"first_name": firstName2,
			"last_name":  lastName2,
			"password":   generatePassword(t),
		}

		user2CreateResponse, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, user2, accessTokenHeader)
		assert.NoError(t, err, "Failed to create user2")
		defer user2CreateResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, user2CreateResponse.StatusCode, "Expected status code 201")

		firstName3, lastName3, email3 := generateUserData(t)
		userID3 := uuid.NewV7()

		user3 := map[string]any{
			"id":         userID3.String(),
			"email":      email3,
			"first_name": firstName3,
			"last_name":  lastName3,
			"password":   generatePassword(t),
		}

		user3CreateResponse, err := sendHTTPRequest(t, ctx, usersCreateEndpoint, user3, accessTokenHeader)
		assert.NoError(t, err, "Failed to create user3")
		defer user3CreateResponse.Body.Close()
		assert.Equal(t, http.StatusCreated, user3CreateResponse.StatusCode, "Expected status code 201")

		// 4. First link users to project
		usersToLink := []uuid.UUID{userID1, userID2, userID3}
		linkRequest := map[string]any{
			"user_ids": usersToLink,
		}

		linkEndpoint := projectsLinkUsersEndpoint.RewriteSlugs(projectID.String())
		linkResponse, err := sendHTTPRequest(t, ctx, linkEndpoint, linkRequest, accessTokenHeader)
		assert.NoError(t, err, "Failed to link users to project")
		defer linkResponse.Body.Close()
		assert.Equal(t, http.StatusOK, linkResponse.StatusCode, "Expected status code 200")

		// 5. Now unlink some users (userID1 and userID2)
		usersToUnlink := []uuid.UUID{userID1, userID2}
		unlinkRequest := map[string]any{
			"user_ids": usersToUnlink,
		}

		unlinkEndpoint := projectsUnlinkUsersEndpoint.RewriteSlugs(projectID.String())
		unlinkResponse, err := sendHTTPRequest(t, ctx, unlinkEndpoint, unlinkRequest, accessTokenHeader)
		assert.NoError(t, err, "Failed to unlink users from project")
		defer unlinkResponse.Body.Close()

		assert.Equal(t, http.StatusOK, unlinkResponse.StatusCode, "Expected status code 200")

		// Parse and verify the success response
		unlinkAPIResp, err := parserResponseBody[payload.HTTPMessage](t, unlinkResponse)
		assert.NoError(t, err, "Failed to parse unlink response")
		assert.Equal(t, domain.ProjectsUsersUnlinkedSuccessfully, unlinkAPIResp.Message, "Expected success message")

		// 6. Cleanup (LIFO order: last registered runs first)
		// Note: userID1 and userID2 were already unlinked by the test, but userID3 is still linked!
		t.Cleanup(func() { deleteUserByIDFromDB(t, adminToken.UserID) })
		t.Cleanup(func() { deleteUserByIDFromDB(t, userID3) })
		t.Cleanup(func() { deleteUserByIDFromDB(t, userID2) })
		t.Cleanup(func() { deleteUserByIDFromDB(t, userID1) })
		t.Cleanup(func() { deleteProjectByIDFromDB(t, projectID) })
		t.Cleanup(func() { unlinkAllUsersFromProjectViaDB(t, projectID) })
	})

	t.Run("unlink_users_from_project_bad_request", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test project
		projectID, _, _ := createTestProject(t, ctx, adminToken.AccessToken, "test-project-")

		// 3. Test cases for various invalid inputs
		testCases := []struct {
			name           string
			invalidRequest map[string]any
			expectedError  string
		}{
			{
				name: "Empty user_ids array",
				invalidRequest: map[string]any{
					"user_ids": []uuid.UUID{},
				},
				expectedError: "at least one user",
			},
			{
				name: "Invalid user ID format",
				invalidRequest: map[string]any{
					"user_ids": []string{"not-a-valid-uuid"},
				},
				expectedError: "invalid uuid",
			},
			{
				name:           "Missing user_ids field",
				invalidRequest: map[string]any{},
				expectedError:  "at least one user",
			},
		}

		// 4. Run each test case
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				unlinkEndpoint := projectsUnlinkUsersEndpoint.RewriteSlugs(projectID.String())
				response, err := sendHTTPRequest(t, ctx, unlinkEndpoint, tc.invalidRequest, accessTokenHeader)
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
			deleteProjectByIDFromDB(t, projectID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("unlink_users_project_not_found", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Generate a random UUID that doesn't exist in the database
		nonExistentProjectID := uuid.NewV7()

		// 3. Create a test user
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

		// 4. Try to unlink users from non-existent project
		usersToUnlink := []uuid.UUID{userID}
		unlinkRequest := map[string]any{
			"user_ids": usersToUnlink,
		}

		unlinkEndpoint := projectsUnlinkUsersEndpoint.RewriteSlugs(nonExistentProjectID.String())
		unlinkResponse, err := sendHTTPRequest(t, ctx, unlinkEndpoint, unlinkRequest, accessTokenHeader)
		assert.NoError(t, err, "Failed to send unlink request")
		defer unlinkResponse.Body.Close()

		// 5. Check that we get a 200 Not Found response
		assert.Equal(t, http.StatusOK, unlinkResponse.StatusCode, "Expected status code 200")

		// 6. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, unlinkResponse)
		assert.NoError(t, err, "Failed to parse error response")

		// Verify  message contains information about the project not being found
		assert.Contains(t, strings.ToLower(errorResp.Message), "unlinked from project successfully",
			"Message should indicate project not found but still return success for unlinking")
		// 7. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, userID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("require_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Generate a test project ID
		projectID := uuid.NewV7()

		// 2. Generate a test user ID
		userID := uuid.NewV7()

		// 3. Try to unlink users without authentication
		unlinkRequest := map[string]any{
			"user_ids": []uuid.UUID{userID},
		}

		unlinkEndpoint := projectsUnlinkUsersEndpoint.RewriteSlugs(projectID.String())
		response, err := sendHTTPRequest(t, ctx, unlinkEndpoint, unlinkRequest)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		// 4. Check that we get a 401 Unauthorized response
		assert.Equal(t, http.StatusUnauthorized, response.StatusCode, "Expected status code 401")
	})
}

func TestProjectListByUserID(t *testing.T) {
	t.Run("list_projects_by_user_id", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test user who will own the projects
		firstName, lastName, email := generateUserData(t)
		password := generatePassword(t)
		testUserID := createUserInDB(t, firstName, lastName, email, password)

		// 3. Create a couple of test projects for this user
		var projectIDs []uuid.UUID
		var projectNames []string
		for i := 0; i < 3; i++ {
			projectID, projectName, _ := createTestProject(t, ctx, adminToken.AccessToken, "test_user_project_")
			projectIDs = append(projectIDs, projectID)
			projectNames = append(projectNames, projectName)
		}

		// Wait briefly to ensure projects are created
		time.Sleep(500 * time.Millisecond)

		// 4. Link the projects to the test user
		for _, projectID := range projectIDs {
			linkRequest := map[string]any{
				"user_ids": []uuid.UUID{testUserID},
			}

			linkEndpoint := projectsLinkUsersEndpoint.RewriteSlugs(projectID.String())
			linkResponse, err := sendHTTPRequest(t, ctx, linkEndpoint, linkRequest, accessTokenHeader)
			assert.NoError(t, err, "Failed to link user to project")
			defer linkResponse.Body.Close()
			assert.Equal(t, http.StatusOK, linkResponse.StatusCode, "Expected status code 200 for linking user to project")
		}

		// 5. List the projects for the test user
		listEndpoint := projectsListByUserIDEndpoint.RewriteSlugs(testUserID.String())
		listResponse, err := sendHTTPRequest(t, ctx, listEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request to list projects by user ID")
		defer listResponse.Body.Close()

		// 6. Check the list response
		assert.Equal(t, http.StatusOK, listResponse.StatusCode, "Expected status code 200 for list by user ID. Got %d. Message: %s", listResponse.StatusCode, readResponseBody(t, listResponse))

		// Parse the response
		listAPIResp, err := parserResponseBody[domain.ListProjectsOutput](t, listResponse)
		assert.NoError(t, err, "Failed to parse list projects by user ID response body")

		// 7. Verify the projects are in the list
		assert.GreaterOrEqual(t, len(listAPIResp.Items), 3, "Expected at least 3 projects in the list")

		// Check that our created projects are in the response
		foundProjects := 0
		for _, project := range listAPIResp.Items {
			for _, expectedName := range projectNames {
				if project.Name == expectedName {
					foundProjects++
					break
				}
			}
		}
		assert.Equal(t, 3, foundProjects, "Expected to find all 3 created projects in the user's project list")

		// 8. Cleanup (LIFO order: last registered runs first)
		t.Cleanup(func() { deleteUserByIDFromDB(t, adminToken.UserID) })
		t.Cleanup(func() { deleteUserByIDFromDB(t, testUserID) })
		t.Cleanup(func() {
			for _, projectID := range projectIDs {
				deleteProjectByIDFromDB(t, projectID)
			}
		})
		t.Cleanup(func() {
			for _, projectID := range projectIDs {
				unlinkAllUsersFromProjectViaDB(t, projectID)
			}
		})
	})

	t.Run("list_projects_by_user_id_invalid_uuid", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Try to list projects with an invalid user ID (not a UUID)
		invalidUserID := "invalid-uuid-format"
		listEndpoint := projectsListByUserIDEndpoint.RewriteSlugs(invalidUserID)
		listResponse, err := sendHTTPRequest(t, ctx, listEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request with invalid user ID")
		defer listResponse.Body.Close()

		// 3. Check that we get a 400 Bad Request response
		assert.Equal(t, http.StatusBadRequest, listResponse.StatusCode, "Expected status code 400 for invalid UUID format")

		// 4. Parse and verify the error response
		errorResp, err := parserResponseBody[payload.HTTPMessage](t, listResponse)
		assert.NoError(t, err, "Failed to parse error response")
		assert.Contains(t, strings.ToLower(errorResp.Message), "uuid", "Error message should mention UUID format issue")

		// 5. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("list_projects_by_user_id_not_found", func(t *testing.T) {
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

		// 3. Try to list projects for the non-existent user
		listEndpoint := projectsListByUserIDEndpoint.RewriteSlugs(nonExistentUserID.String())
		listResponse, err := sendHTTPRequest(t, ctx, listEndpoint, nil, accessTokenHeader)
		assert.NoError(t, err, "Failed to send request for non-existent user")
		defer listResponse.Body.Close()

		// 4. Check the response - this should return 200 with an empty list
		// since a non-existent user simply has no projects
		assert.Equal(t, http.StatusOK, listResponse.StatusCode, "Expected status code 200 for non-existent user (empty list)")

		// Parse the response
		listAPIResp, err := parserResponseBody[domain.ListProjectsOutput](t, listResponse)
		assert.NoError(t, err, "Failed to parse response for non-existent user")

		// Verify the list is empty or minimal
		assert.LessOrEqual(t, len(listAPIResp.Items), 0, "Expected empty or minimal project list for non-existent user")

		// 5. Cleanup
		t.Cleanup(func() {
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	t.Run("require_authentication", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Generate a test user ID
		testUserID := uuid.NewV7()

		// 2. Try to list projects without authentication
		listEndpoint := projectsListByUserIDEndpoint.RewriteSlugs(testUserID.String())
		response, err := sendHTTPRequest(t, ctx, listEndpoint, nil)
		assert.NoError(t, err, "Failed to send request without authentication")
		defer response.Body.Close()

		// 3. Check that we get a 401 Unauthorized response
		assert.Equal(t, http.StatusUnauthorized, response.StatusCode, "Expected status code 401 for unauthenticated request")
	})
}

// TestProjectJSONSerialization tests that all project endpoints return JSON responses with snake_case field names
func TestProjectJSONSerialization(t *testing.T) {
	// Test that ProjectResponse uses snake_case in JSON serialization
	t.Run("get_project_response_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test project
		projectID := uuid.NewV7()

		project := map[string]any{
			"id":          projectID.String(),
			"name":        generateRandomName(t, ""),
			"description": "Test project for JSON serialization",
		}

		createResponse, err := sendHTTPRequest(t, ctx, projectsCreateEndpoint, project, accessTokenHeader)
		require.NoError(t, err, "Failed to create project")
		defer createResponse.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse.StatusCode)

		// 3. Get the project
		getProjectEndpoint := projectsGetEndpoint.RewriteSlugs(projectID.String())
		response, err := sendHTTPRequest(t, ctx, getProjectEndpoint, nil, accessTokenHeader)
		require.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		// 4. Verify all ProjectResponse fields are in snake_case
		expectedFields := []string{
			"id",
			"name",
			"description",
			"disabled",
			"system",
			"created_at",
			"updated_at",
		}

		assertJSONFieldsAreSnakeCase(t, response, expectedFields)

		t.Cleanup(func() {
			deleteProjectByIDFromDB(t, projectID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test that ListProjectsOutput uses snake_case including nested objects
	t.Run("list_projects_response_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create test projects
		project1ID := uuid.NewV7()

		project1 := map[string]any{
			"id":          project1ID.String(),
			"name":        generateRandomName(t, ""),
			"description": "Test project 1",
		}

		createResponse1, err := sendHTTPRequest(t, ctx, projectsCreateEndpoint, project1, accessTokenHeader)
		require.NoError(t, err, "Failed to create project 1")
		defer createResponse1.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse1.StatusCode)

		project2ID := uuid.NewV7()

		project2 := map[string]any{
			"id":          project2ID.String(),
			"name":        generateRandomName(t, ""),
			"description": "Test project 2",
		}

		createResponse2, err := sendHTTPRequest(t, ctx, projectsCreateEndpoint, project2, accessTokenHeader)
		require.NoError(t, err, "Failed to create project 2")
		defer createResponse2.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse2.StatusCode)

		// 3. List projects
		response, err := sendHTTPRequest(t, ctx, projectsListEndpoint, nil, accessTokenHeader)
		require.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		// 4. Verify ListProjectsOutput top-level fields are snake_case
		expectedTopLevelFields := []string{
			"items",
			"paginator",
		}

		assertJSONFieldsAreSnakeCase(t, response, expectedTopLevelFields)

		t.Cleanup(func() {
			deleteProjectByIDFromDB(t, project1ID)
			deleteProjectByIDFromDB(t, project2ID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test that HTTPMessage response uses snake_case
	t.Run("create_project_http_message_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a new project
		projectID := uuid.NewV7()

		project := map[string]any{
			"id":          projectID.String(),
			"name":        generateRandomName(t, ""),
			"description": "Test project for HTTP message",
		}

		response, err := sendHTTPRequest(t, ctx, projectsCreateEndpoint, project, accessTokenHeader)
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
			deleteProjectByIDFromDB(t, projectID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test that update response uses snake_case
	t.Run("update_project_response_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test project
		projectID := uuid.NewV7()

		project := map[string]any{
			"id":          projectID.String(),
			"name":        generateRandomName(t, ""),
			"description": "Test project for update",
		}

		createResponse, err := sendHTTPRequest(t, ctx, projectsCreateEndpoint, project, accessTokenHeader)
		require.NoError(t, err, "Failed to create project")
		defer createResponse.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse.StatusCode)

		// 3. Update the project
		newName := generateRandomName(t, "")
		updatePayload := map[string]any{
			"name": newName,
		}

		updateEndpoint := projectsUpdateEndpoint.RewriteSlugs(projectID.String())
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
			deleteProjectByIDFromDB(t, projectID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test that delete response uses snake_case
	t.Run("delete_project_response_uses_snake_case", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Create an administrator user and get the token
		adminToken := getAdminUserTokens(t)
		assert.NotEmpty(t, adminToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 2. Create a test project
		projectID := uuid.NewV7()

		project := map[string]any{
			"id":          projectID.String(),
			"name":        generateRandomName(t, ""),
			"description": "Test project for delete",
		}

		createResponse, err := sendHTTPRequest(t, ctx, projectsCreateEndpoint, project, accessTokenHeader)
		require.NoError(t, err, "Failed to create project")
		defer createResponse.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse.StatusCode)

		// 3. Delete the project
		deleteEndpoint := projectsDeleteEndpoint.RewriteSlugs(projectID.String())
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

		// 2. Create a test project
		projectID := uuid.NewV7()

		project := map[string]any{
			"id":          projectID.String(),
			"name":        generateRandomName(t, ""),
			"description": "Test project for link users",
		}

		createResponse, err := sendHTTPRequest(t, ctx, projectsCreateEndpoint, project, accessTokenHeader)
		require.NoError(t, err, "Failed to create project")
		defer createResponse.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse.StatusCode)

		// 3. Create a test user
		firstName, lastName, email := generateUserData(t)
		password := generatePassword(t)

		userID := createUserInDB(t, firstName, lastName, email, password)

		// 4. Link user to project
		linkPayload := map[string]any{
			"user_ids": []uuid.UUID{userID},
		}

		linkEndpoint := projectsLinkUsersEndpoint.RewriteSlugs(projectID.String())
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
			unlinkAllUsersFromProjectViaDB(t, projectID)
			deleteUserByIDFromDB(t, userID)
			deleteProjectByIDFromDB(t, projectID)
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

		// 2. Create a test project
		projectID := uuid.NewV7()

		project := map[string]any{
			"id":          projectID.String(),
			"name":        generateRandomName(t, ""),
			"description": "Test project for unlink users",
		}

		createResponse, err := sendHTTPRequest(t, ctx, projectsCreateEndpoint, project, accessTokenHeader)
		require.NoError(t, err, "Failed to create project")
		defer createResponse.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse.StatusCode)

		// 3. Create a test user and link it
		firstName, lastName, email := generateUserData(t)
		password := generatePassword(t)

		userID := createUserInDB(t, firstName, lastName, email, password)

		// Link the user
		linkQuery := `INSERT INTO projects_users (projects_id, users_id) VALUES ($1, $2)`
		_, err = testDBPool.Exec(ctx, linkQuery, projectID, userID)
		require.NoError(t, err, "Failed to link user to project")

		// 4. Unlink user from project
		unlinkPayload := map[string]any{
			"user_ids": []uuid.UUID{userID},
		}

		unlinkEndpoint := projectsUnlinkUsersEndpoint.RewriteSlugs(projectID.String())
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
			unlinkAllUsersFromProjectViaDB(t, projectID)
			deleteUserByIDFromDB(t, userID)
			deleteProjectByIDFromDB(t, projectID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})

	// Test that list projects by user ID response uses snake_case
	t.Run("list_projects_by_user_id_uses_snake_case", func(t *testing.T) {
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

		// 3. Create test projects and link them
		project1ID := uuid.NewV7()

		project1 := map[string]any{
			"id":          project1ID.String(),
			"name":        generateRandomName(t, ""),
			"description": "Test project 1",
		}

		createResponse1, err := sendHTTPRequest(t, ctx, projectsCreateEndpoint, project1, accessTokenHeader)
		require.NoError(t, err, "Failed to create project 1")
		defer createResponse1.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse1.StatusCode)

		project2ID := uuid.NewV7()

		project2 := map[string]any{
			"id":          project2ID.String(),
			"name":        generateRandomName(t, ""),
			"description": "Test project 2",
		}

		createResponse2, err := sendHTTPRequest(t, ctx, projectsCreateEndpoint, project2, accessTokenHeader)
		require.NoError(t, err, "Failed to create project 2")
		defer createResponse2.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse2.StatusCode)

		// Link projects to user
		linkQuery := `INSERT INTO projects_users (projects_id, users_id) VALUES ($1, $2)`
		_, err = testDBPool.Exec(ctx, linkQuery, project1ID, userID)
		require.NoError(t, err, "Failed to link project 1 to user")
		_, err = testDBPool.Exec(ctx, linkQuery, project2ID, userID)
		require.NoError(t, err, "Failed to link project 2 to user")

		// 4. List projects by user ID
		listEndpoint := projectsListByUserIDEndpoint.RewriteSlugs(userID.String())
		response, err := sendHTTPRequest(t, ctx, listEndpoint, nil, accessTokenHeader)
		require.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode, "Expected status code 200")

		// 5. Verify ListProjectsOutput top-level fields are snake_case
		expectedTopLevelFields := []string{
			"items",
			"paginator",
		}

		assertJSONFieldsAreSnakeCase(t, response, expectedTopLevelFields)

		t.Cleanup(func() {
			unlinkAllUsersFromProjectViaDB(t, project1ID)
			unlinkAllUsersFromProjectViaDB(t, project2ID)
			deleteProjectByIDFromDB(t, project1ID)
			deleteProjectByIDFromDB(t, project2ID)
			deleteUserByIDFromDB(t, userID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})
	})
}
