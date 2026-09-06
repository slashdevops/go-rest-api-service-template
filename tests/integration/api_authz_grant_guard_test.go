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

const (
	administratorRoleID = "019822af-b448-750c-ae0d-edaf3aaafc41"
	fullAccessPolicyID  = "019822c9-9775-7678-b6ea-5c4701531a00"
)

// TestGrantGuardStopsTheEscalationChain keeps the Phase-0 proof of the
// authorization review: with the four grants POST /policies, POST /roles,
// POST /roles/*/policies and POST /users/*/roles, a regular account minted an
// allow-all policy, a role, attached both to itself and became an
// administrator. Every step of that chain is refused now with 403, and the
// same steps still work for an administrator.
func TestGrantGuardStopsTheEscalationChain(t *testing.T) {
	ctx := t.Context()
	admin := getAdminUserTokens(t)
	adminAuth := map[string]string{"Authorization": "Bearer " + admin.AccessToken}

	post := func(token map[string]string, ep *apiEndpoint, body map[string]any) (int, string) {
		resp, err := sendHTTPRequest(t, ctx, ep, body, token)
		require.NoError(t, err)
		defer resp.Body.Close()
		return resp.StatusCode, readResponseBody(t, resp)
	}

	// A role holding exactly the five grants, assigned to a fresh account.
	roleName := "grant-guard-" + uuid.NewV7().String()
	status, body := post(adminAuth, rolesCreateEndpoint, map[string]any{"name": roleName, "description": "the five grants"})
	require.Equal(t, http.StatusCreated, status, body)
	var roleID uuid.UUID
	require.NoError(t, testDBPool.QueryRow(context.Background(), `SELECT id FROM roles WHERE name = $1`, roleName).Scan(&roleID))
	t.Cleanup(func() { _, _ = testDBPool.Exec(context.Background(), `DELETE FROM roles WHERE id = $1`, roleID) })

	var policyIDs []uuid.UUID
	for i, g := range [][2]string{{"POST", "/policies"}, {"PUT", "/policies/*"}, {"POST", "/roles"}, {"POST", "/roles/*/policies"}, {"POST", "/users/*/roles"}} {
		name := fmt.Sprintf("%s-%d", roleName, i)
		status, body := post(adminAuth, policyCreateEndpoint, map[string]any{"name": name, "description": "one of the five", "allowed_action": g[0], "allowed_resource": g[1]})
		require.Equal(t, http.StatusCreated, status, body)
		var id uuid.UUID
		require.NoError(t, testDBPool.QueryRow(context.Background(), `SELECT id FROM policies WHERE name = $1`, name).Scan(&id))
		policyIDs = append(policyIDs, id)
		t.Cleanup(func() { _, _ = testDBPool.Exec(context.Background(), `DELETE FROM policies WHERE id = $1`, id) })
	}

	status, body = post(adminAuth, rolesLinkPoliciesEndpoint.RewriteSlugs(roleID.String()), map[string]any{"policy_ids": policyIDs})
	require.Equal(t, http.StatusOK, status, body)

	const password = "ThisIsApassw0rd.,"
	email := fmt.Sprintf("gg-%s@example.com", uuid.NewV7().String()[24:])
	userID := createUserInDB(t, "Grant", "Guard", email, password)
	enableUserByEmailFromDB(t, email)
	assignRoleToUserInDB(t, userID, "AuthenticatedUser")
	t.Cleanup(func() { deleteUserByIDFromDB(t, userID) })
	status, body = post(adminAuth, usersLinkRolesEndpoint.RewriteSlugs(userID.String()), map[string]any{"role_ids": []string{roleID.String()}})
	require.Equal(t, http.StatusOK, status, body)

	loginResp, err := sendHTTPRequest(t, ctx, authLoginEndpoint, map[string]any{"email": email, "password": password})
	require.NoError(t, err)
	defer loginResp.Body.Close()
	tokens, err := parserResponseBody[payload.LoginUserResponse](t, loginResp)
	require.NoError(t, err)
	userAuth := map[string]string{"Authorization": "Bearer " + tokens.AccessToken}

	t.Run("cannot_mint_a_policy_they_do_not_hold", func(t *testing.T) {
		status, body := post(userAuth, policyCreateEndpoint, map[string]any{"name": roleName + "-allow-all", "description": "escalation", "allowed_action": "*", "allowed_resource": "*"})
		require.Equal(t, http.StatusForbidden, status, body)
		require.Contains(t, body, "does not hold")
	})

	t.Run("can_mint_a_policy_they_do_hold", func(t *testing.T) {
		status, body := post(userAuth, policyCreateEndpoint, map[string]any{"name": roleName + "-held", "description": "a grant the caller holds", "allowed_action": "POST", "allowed_resource": "/roles"})
		require.Equal(t, http.StatusCreated, status, body)
		t.Cleanup(func() {
			_, _ = testDBPool.Exec(context.Background(), `DELETE FROM policies WHERE name = $1`, roleName+"-held")
		})
	})

	t.Run("cannot_attach_full_access_to_a_role", func(t *testing.T) {
		status, body := post(userAuth, rolesCreateEndpoint, map[string]any{"name": roleName + "-owned", "description": "escalation"})
		require.Equal(t, http.StatusCreated, status, body)
		var owned uuid.UUID
		require.NoError(t, testDBPool.QueryRow(context.Background(), `SELECT id FROM roles WHERE name = $1`, roleName+"-owned").Scan(&owned))
		t.Cleanup(func() { _, _ = testDBPool.Exec(context.Background(), `DELETE FROM roles WHERE id = $1`, owned) })

		status, body = post(userAuth, rolesLinkPoliciesEndpoint.RewriteSlugs(owned.String()), map[string]any{"policy_ids": []string{fullAccessPolicyID}})
		require.Equal(t, http.StatusForbidden, status, body)

		// Nor assign the Administrator role to anyone, themselves included.
		status, body = post(userAuth, usersLinkRolesEndpoint.RewriteSlugs(userID.String()), map[string]any{"role_ids": []string{administratorRoleID}})
		require.Equal(t, http.StatusForbidden, status, body)
	})

	// Editing a policy the caller does not hold: the description may change,
	// because that widens nothing; the grant may not, because the result is
	// a grant the caller does not hold.
	t.Run("can_rename_but_not_regrant_a_policy_they_do_not_hold", func(t *testing.T) {
		name := roleName + "-not-held"
		status, body := post(adminAuth, policyCreateEndpoint, map[string]any{"name": name, "description": "held by nobody but an administrator", "allowed_action": "*", "allowed_resource": "*"})
		require.Equal(t, http.StatusCreated, status, body)
		var id uuid.UUID
		require.NoError(t, testDBPool.QueryRow(context.Background(), `SELECT id FROM policies WHERE name = $1`, name).Scan(&id))
		t.Cleanup(func() { _, _ = testDBPool.Exec(context.Background(), `DELETE FROM policies WHERE id = $1`, id) })

		status, body = post(userAuth, policyUpdateEndpoint.RewriteSlugs(id.String()), map[string]any{"description": "a new description"})
		require.Equal(t, http.StatusOK, status, body)

		status, body = post(userAuth, policyUpdateEndpoint.RewriteSlugs(id.String()), map[string]any{"allowed_action": "GET"})
		require.Equal(t, http.StatusForbidden, status, body)
		require.Contains(t, body, "does not hold")
	})

	t.Run("an_administrator_still_can", func(t *testing.T) {
		status, body := post(adminAuth, policyCreateEndpoint, map[string]any{"name": roleName + "-admin-all", "description": "an administrator holds everything", "allowed_action": "*", "allowed_resource": "*"})
		require.Equal(t, http.StatusCreated, status, body)
		t.Cleanup(func() {
			_, _ = testDBPool.Exec(context.Background(), `DELETE FROM policies WHERE name = $1`, roleName+"-admin-all")
		})
	})
}

// TestBootstrapLinksCannotBeSevered: the Administrator ↔ Full Access link and
// the seeded administrator's role link are refused deletion at the database,
// whoever asks. One API call used to lock every administrator out for good.
func TestBootstrapLinksCannotBeSevered(t *testing.T) {
	ctx := t.Context()
	admin := getAdminUserTokens(t)
	adminAuth := map[string]string{"Authorization": "Bearer " + admin.AccessToken}

	resp, err := sendHTTPRequest(t, ctx, rolesUnlinkPoliciesEndpoint.RewriteSlugs(administratorRoleID), map[string]any{"policy_ids": []string{fullAccessPolicyID}}, adminAuth)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.NotEqual(t, http.StatusOK, resp.StatusCode, readResponseBody(t, resp))

	var n int
	require.NoError(t, testDBPool.QueryRow(ctx, `SELECT count(*) FROM roles_policies WHERE roles_id = $1 AND policies_id = $2`, administratorRoleID, fullAccessPolicyID).Scan(&n))
	require.Equal(t, 1, n, "the Administrator must still hold Full Access")

	_, err = testDBPool.Exec(ctx, `DELETE FROM users_roles WHERE users_id = '019822af-b448-73fb-89a1-447e8f8d1cde' AND roles_id = $1`, administratorRoleID)
	require.Error(t, err, "the bootstrap administrator's role link must be refused at the database")
}
