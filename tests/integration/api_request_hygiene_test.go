//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestUnknownJSONFieldIsRefused pins the decoder's posture: a field the API
// does not know is a 400 that names it, in this service's own words. Every
// body decoder used to accept and drop it, so a client kept sending what it
// believed was being honoured.
func TestUnknownJSONFieldIsRefused(t *testing.T) {
	ctx := t.Context()
	admin := getAdminUserTokens(t)
	auth := map[string]string{"Authorization": "Bearer " + admin.AccessToken}

	resp, err := sendHTTPRequest(t, ctx, rolesCreateEndpoint, map[string]any{
		"name":        "hygiene-proof",
		"description": "a role that must not be created",
		"bogus":       1,
	}, auth)
	require.NoError(t, err)
	defer resp.Body.Close()

	body := readResponseBody(t, resp)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, body)
	require.Contains(t, body, "unknown field")
	require.Contains(t, body, "bogus")
	require.NotContains(t, body, "json:", "the decoder's own text must not reach the client")
}
