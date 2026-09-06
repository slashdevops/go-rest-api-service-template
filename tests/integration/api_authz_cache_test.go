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

// TestARevokedGrantBitesOnTheNextRequest: a user's effective permissions are
// cached for hours; every path that removes a grant must invalidate that
// cache, or the grant outlives the change. Three unlink routes, each proven
// to take effect on the very next request.
func TestARevokedGrantBitesOnTheNextRequest(t *testing.T) {
	ctx := t.Context()
	admin := getAdminUserTokens(t)
	adminAuth := map[string]string{"Authorization": "Bearer " + admin.AccessToken}

	call := func(token map[string]string, ep *apiEndpoint, body map[string]any) int {
		resp, err := sendHTTPRequest(t, ctx, ep, body, token)
		require.NoError(t, err)
		defer resp.Body.Close()
		return resp.StatusCode
	}

	suffix := uuid.NewV7().String()[24:]
	const password = "ThisIsApassw0rd.,"
	email := fmt.Sprintf("revoke-%s@example.com", suffix)
	userID := createUserInDB(t, "Revoke", "Me", email, password)
	enableUserByEmailFromDB(t, email)
	assignRoleToUserInDB(t, userID, "AuthenticatedUser")
	t.Cleanup(func() { deleteUserByIDFromDB(t, userID) })

	// A role holding GET /roles, assigned to the user through the API so the
	// cache is invalidated the way a real assignment does it.
	roleName := "revoke-" + suffix
	require.Equal(t, http.StatusCreated, call(adminAuth, rolesCreateEndpoint, map[string]any{"name": roleName, "description": "revoke proof"}))
	var roleID uuid.UUID
	require.NoError(t, testDBPool.QueryRow(context.Background(), `SELECT id FROM roles WHERE name = $1`, roleName).Scan(&roleID))
	t.Cleanup(func() { _, _ = testDBPool.Exec(context.Background(), `DELETE FROM roles WHERE id = $1`, roleID) })
	policyName := roleName + "-read-roles"
	require.Equal(t, http.StatusCreated, call(adminAuth, policyCreateEndpoint, map[string]any{"name": policyName, "description": "revoke proof", "allowed_action": "GET", "allowed_resource": "/roles"}))
	var policyID uuid.UUID
	require.NoError(t, testDBPool.QueryRow(context.Background(), `SELECT id FROM policies WHERE name = $1`, policyName).Scan(&policyID))
	t.Cleanup(func() { _, _ = testDBPool.Exec(context.Background(), `DELETE FROM policies WHERE id = $1`, policyID) })

	loginResp, err := sendHTTPRequest(t, ctx, authLoginEndpoint, map[string]any{"email": email, "password": password})
	require.NoError(t, err)
	defer loginResp.Body.Close()
	tokens, err := parserResponseBody[payload.LoginUserResponse](t, loginResp)
	require.NoError(t, err)
	userAuth := map[string]string{"Authorization": "Bearer " + tokens.AccessToken}
	listRoles := rolesListEndpoint

	grant := func(t *testing.T) {
		require.Equal(t, http.StatusOK, call(adminAuth, rolesLinkPoliciesEndpoint.RewriteSlugs(roleID.String()), map[string]any{"policy_ids": []string{policyID.String()}}))
		require.Equal(t, http.StatusOK, call(adminAuth, usersLinkRolesEndpoint.RewriteSlugs(userID.String()), map[string]any{"role_ids": []string{roleID.String()}}))
		require.Equal(t, http.StatusOK, call(userAuth, listRoles, nil), "the grant must be live before it is revoked")
	}

	t.Run("policy_unlinked_from_the_role_via_policies", func(t *testing.T) {
		grant(t)
		require.Equal(t, http.StatusOK, call(adminAuth, policyUnlinkRolesEndpoint.RewriteSlugs(policyID.String()), map[string]any{"role_ids": []string{roleID.String()}}))
		require.Equal(t, http.StatusForbidden, call(userAuth, listRoles, nil))
	})

	t.Run("policy_unlinked_from_the_role_via_roles", func(t *testing.T) {
		grant(t)
		require.Equal(t, http.StatusOK, call(adminAuth, rolesUnlinkPoliciesEndpoint.RewriteSlugs(roleID.String()), map[string]any{"policy_ids": []string{policyID.String()}}))
		require.Equal(t, http.StatusForbidden, call(userAuth, listRoles, nil))
	})

	t.Run("role_unlinked_from_the_user", func(t *testing.T) {
		grant(t)
		require.Equal(t, http.StatusOK, call(adminAuth, usersUnlinkRolesEndpoint.RewriteSlugs(userID.String()), map[string]any{"role_ids": []string{roleID.String()}}))
		require.Equal(t, http.StatusForbidden, call(userAuth, listRoles, nil))
	})
}
