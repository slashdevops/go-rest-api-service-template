//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
	"github.com/stretchr/testify/require"
)

// grantProjectRead gives a fresh role one policy on every project's embedding
// configs, the grant that used to open every project: OPA expands the "*" to
// any uuid and cannot know membership.
func grantProjectRead(t *testing.T) (roleName string) {
	t.Helper()

	var resourceID uuid.UUID
	err := testDBPool.QueryRow(context.Background(),
		`SELECT id FROM resources WHERE action = 'GET' AND resource = '/projects/{project_id}/products' LIMIT 1`,
	).Scan(&resourceID)
	require.NoError(t, err, "the resource row for GET /projects/{project_id}/products must exist")

	roleID, policyID := uuid.NewV7(), uuid.NewV7()
	roleName = "membership-proof-" + roleID.String()[:8]

	_, err = testDBPool.Exec(context.Background(),
		`INSERT INTO roles (id, name, description) VALUES ($1, $2, 'membership proof')`, roleID, roleName)
	require.NoError(t, err)

	_, err = testDBPool.Exec(context.Background(),
		`INSERT INTO policies (id, resources_id, name, description, allowed_action, allowed_resource)
		 VALUES ($1, $2, $3, 'membership proof', 'GET', '/projects/*/products')`,
		policyID, resourceID, "membership-proof-"+policyID.String()[:8])
	require.NoError(t, err)

	_, err = testDBPool.Exec(context.Background(),
		`INSERT INTO roles_policies (roles_id, policies_id) VALUES ($1, $2)`, roleID, policyID)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = testDBPool.Exec(context.Background(), `DELETE FROM roles WHERE id = $1`, roleID)
		_, _ = testDBPool.Exec(context.Background(), `DELETE FROM policies WHERE id = $1`, policyID)
	})

	return roleName
}

func loginForMembership(t *testing.T, email, password string) payload.LoginUserResponse {
	t.Helper()

	resp, err := sendHTTPRequest(t, t.Context(), authLoginEndpoint, map[string]any{"email": email, "password": password})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "login: %s", readResponseBody(t, resp))

	tokens, err := parserResponseBody[payload.LoginUserResponse](t, resp)
	require.NoError(t, err)

	return tokens
}

// TestProjectMembershipGatesProjectScopedRoutes is the Phase-0 proof of the
// security review, kept as a test: a grant on /projects/*/products
// admitted its holder to EVERY project. Now it admits them to the projects
// they are a member of, an administrator to all, and answers a non-member
// with the same 404 a missing project gets.
func TestProjectMembershipGatesProjectScopedRoutes(t *testing.T) {
	ctx := t.Context()
	roleName := grantProjectRead(t)

	project := createProjectInDB(t, "membership-proof-"+uuid.NewV7().String()[:8], "a project two users are authorised for and one belongs to")
	t.Cleanup(func() { deleteProjectByIDFromDB(t, project.ID) })

	const password = "ThisIsApassw0rd.,"
	newUser := func(tag string) (uuid.UUID, string) {
		email := fmt.Sprintf("membership-%s-%s@example.com", tag, uuid.NewV7().String()[:8])
		id := createUserInDB(t, "Member", tag, email, password)
		enableUserByEmailFromDB(t, email)
		assignRoleToUserInDB(t, id, roleName)
		t.Cleanup(func() { _, _ = testDBPool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, id) })
		return id, email
	}

	memberID, memberEmail := newUser("in")
	_, outsiderEmail := newUser("out")

	_, err := testDBPool.Exec(ctx, `INSERT INTO projects_users (projects_id, users_id) VALUES ($1, $2)`, project.ID, memberID)
	require.NoError(t, err)

	member := loginForMembership(t, memberEmail, password)
	outsider := loginForMembership(t, outsiderEmail, password)
	admin := getAdminUserTokens(t)

	list := productsListEndpoint.RewriteSlugs(project.ID.String())
	call := func(token string, ep *apiEndpoint) (int, string) {
		resp, err := sendHTTPRequest(t, ctx, ep, nil, map[string]string{"Authorization": "Bearer " + token})
		require.NoError(t, err)
		defer resp.Body.Close()
		return resp.StatusCode, readResponseBody(t, resp)
	}

	t.Run("a_member_with_the_grant_is_admitted", func(t *testing.T) {
		status, body := call(member.AccessToken, list)
		require.Equal(t, http.StatusOK, status, body)
	})

	t.Run("an_administrator_is_admitted_to_every_project", func(t *testing.T) {
		status, body := call(admin.AccessToken, list)
		require.Equal(t, http.StatusOK, status, body)
	})

	t.Run("a_non_member_with_the_same_grant_is_refused_as_not_found", func(t *testing.T) {
		status, body := call(outsider.AccessToken, list)
		require.Equal(t, http.StatusNotFound, status, body)
		require.NotContains(t, body, "member", "the refusal must not say why")
	})

	t.Run("a_missing_project_answers_the_same_way", func(t *testing.T) {
		missing := productsListEndpoint.RewriteSlugs(uuid.NewV7().String())
		status, body := call(outsider.AccessToken, missing)
		require.Equal(t, http.StatusNotFound, status, body)
	})

	t.Run("a_caller_without_the_grant_is_403_before_membership_is_asked", func(t *testing.T) {
		email := fmt.Sprintf("membership-nogrant-%s@example.com", uuid.NewV7().String()[:8])
		id := createUserInDB(t, "No", "Grant", email, password)
		enableUserByEmailFromDB(t, email)
		assignRoleToUserInDB(t, id, "AuthenticatedUser")
		t.Cleanup(func() { _, _ = testDBPool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, id) })
		_, err := testDBPool.Exec(ctx, `INSERT INTO projects_users (projects_id, users_id) VALUES ($1, $2)`, project.ID, id)
		require.NoError(t, err)

		status, body := call(loginForMembership(t, email, password).AccessToken, list)
		require.Equal(t, http.StatusForbidden, status, body)
	})
}
