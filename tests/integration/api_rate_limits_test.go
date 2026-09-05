//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
)

var (
	rateLimitCreateEndpoint    = newAPIEndpoint(http.MethodPost, "/rate_limits")
	rateLimitListEndpoint      = newAPIEndpoint(http.MethodGet, "/rate_limits")
	rateLimitGetEndpoint       = newAPIEndpoint(http.MethodGet, "/rate_limits/{rate_limit_id}")
	rateLimitUpdateEndpoint    = newAPIEndpoint(http.MethodPut, "/rate_limits/{rate_limit_id}")
	rateLimitDeleteEndpoint    = newAPIEndpoint(http.MethodDelete, "/rate_limits/{rate_limit_id}")
	rateLimitEffectiveEndpoint = newAPIEndpoint(http.MethodGet, "/rate_limits/effective")
)

// validRateLimitBody returns a rule that targets a route this service really
// serves. Every rejection case below mutates ONE field of it, so a validator
// that refused everything could not pass them all.
func validRateLimitBody(id uuid.UUID) map[string]any {
	return map[string]any{
		"id":          id.String(),
		"name":        "test " + id.String(),
		"description": "created by the integration suite",
		"target_kind": "endpoint",
		"target":      "/products",
		"methods":     []string{"GET"},
		"scope":       "ip",
		"audience":    "any",
		"strategy":    "token_bucket",
		"windows":     []map[string]any{{"requests": 100, "period": "1m0s", "burst": 100}},
	}
}

func createTestRateLimit(t *testing.T, accessToken string, body map[string]any) uuid.UUID {
	t.Helper()

	hdr := map[string]string{"Authorization": "Bearer " + accessToken}

	resp, err := sendHTTPRequest(t, t.Context(), rateLimitCreateEndpoint, body, hdr)
	require.NoError(t, err, "failed to send create request")

	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode,
		"expected 201, got %d: %s", resp.StatusCode, readResponseBody(t, resp))

	id, err := uuid.Parse(body["id"].(string))
	require.NoError(t, err)

	return id
}

func deleteTestRateLimit(t *testing.T, accessToken string, id uuid.UUID) {
	t.Helper()

	hdr := map[string]string{"Authorization": "Bearer " + accessToken}

	// context.Background(), NOT t.Context(): the test context is cancelled
	// BEFORE cleanups run, so a cleanup using it never reaches the server and
	// every rule this suite creates would be left behind.
	resp, err := sendHTTPRequest(t, context.Background(), rateLimitDeleteEndpoint.RewriteSlugs(id.String()), nil, hdr)
	if err != nil {
		return
	}

	resp.Body.Close()
}

// clearRateLimitsForTarget removes every rule pointing at target.
//
// Necessary because two endpoint rules on one path tie at the same rung and the
// tie-break is by name, so a rule left behind by a previous run silently decides
// the answer.
func clearRateLimitsForTarget(t *testing.T, accessToken, target string) {
	t.Helper()

	hdr := map[string]string{"Authorization": "Bearer " + accessToken}

	resp, err := sendHTTPRequest(t, t.Context(), rateLimitListEndpoint, nil, hdr)
	require.NoError(t, err)

	defer resp.Body.Close()

	var listed payload.ListRateLimitsResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listed))

	for _, item := range listed.Items {
		if item.Target == target && item.TargetKind == "endpoint" {
			deleteTestRateLimit(t, accessToken, item.ID)
		}
	}
}

func TestRateLimitsCRUD(t *testing.T) {
	tokens := getAdminUserTokens(t)
	hdr := map[string]string{"Authorization": "Bearer " + tokens.AccessToken}

	id := uuid.NewV7()
	body := validRateLimitBody(id)

	createTestRateLimit(t, tokens.AccessToken, body)
	t.Cleanup(func() { deleteTestRateLimit(t, tokens.AccessToken, id) })

	t.Run("get_returns_the_rule_with_its_windows", func(t *testing.T) {
		resp, err := sendHTTPRequest(t, t.Context(), rateLimitGetEndpoint.RewriteSlugs(id.String()), nil, hdr)
		require.NoError(t, err)

		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", readResponseBody(t, resp))

		var got payload.RateLimitResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))

		assert.Equal(t, "endpoint", got.TargetKind)
		assert.Equal(t, "/products", got.Target)
		assert.Equal(t, "token_bucket", got.Strategy)
		require.Len(t, got.Windows, 1, "the window must come back with the rule; a rule with no window has no budget")
		// A duration string, not seconds: seconds are what the column holds,
		// a duration is what every other duration in this API uses.
		assert.Equal(t, "1m0s", got.Windows[0].Period)
		assert.Equal(t, 100, got.Windows[0].Requests)
	})

	t.Run("list_includes_it", func(t *testing.T) {
		resp, err := sendHTTPRequest(t, t.Context(), rateLimitListEndpoint, nil, hdr)
		require.NoError(t, err)

		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", readResponseBody(t, resp))

		var got payload.ListRateLimitsResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))

		found := false

		for _, item := range got.Items {
			if item.ID == id {
				found = true
				assert.NotEmpty(t, item.Windows, "the listing must carry each rule's windows, or a reader cannot see the budget")
			}
		}

		assert.True(t, found, "the created rule should appear in the listing")
	})

	t.Run("update_replaces_the_window_set_wholesale", func(t *testing.T) {
		upd := validRateLimitBody(id)
		delete(upd, "id")
		upd["strategy"] = "leaky_bucket"
		upd["windows"] = []map[string]any{
			{"requests": 10, "period": "1s", "burst": 1},
			{"requests": 300, "period": "1m0s"},
		}

		resp, err := sendHTTPRequest(t, t.Context(), rateLimitUpdateEndpoint.RewriteSlugs(id.String()), upd, hdr)
		require.NoError(t, err)

		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", readResponseBody(t, resp))

		get, err := sendHTTPRequest(t, t.Context(), rateLimitGetEndpoint.RewriteSlugs(id.String()), nil, hdr)
		require.NoError(t, err)

		defer get.Body.Close()

		var got payload.RateLimitResponse
		require.NoError(t, json.NewDecoder(get.Body).Decode(&got))

		assert.Equal(t, "leaky_bucket", got.Strategy, "the strategy must survive the round trip; it is easy to lose between the form and the limiter")
		require.Len(t, got.Windows, 2, "the window set is replaced in full, not merged")
		// Shortest period first, which is the order they are evaluated in.
		assert.Equal(t, "1s", got.Windows[0].Period)
		assert.Equal(t, "1m0s", got.Windows[1].Period)
	})
}

// A rule for a route this service does not serve is not inert: it looks correct
// in a listing, reports no error, and protects nothing. Catching it at write
// time is the whole reason the check exists.
func TestRateLimitsRejects(t *testing.T) {
	tokens := getAdminUserTokens(t)
	hdr := map[string]string{"Authorization": "Bearer " + tokens.AccessToken}

	tests := []struct {
		mutate func(map[string]any)
		name   string
		want   int
		says   string
	}{
		{
			name: "a_path_no_route_matches", want: http.StatusBadRequest, says: "no route",
			mutate: func(b map[string]any) { b["target"] = "/not_a_real_endpoint" },
		},
		{
			name: "a_verb_the_path_does_not_register", want: http.StatusBadRequest, says: "not",
			mutate: func(b map[string]any) { b["methods"] = []string{"DELETE"} },
		},
		{
			name: "an_unknown_strategy", want: http.StatusBadRequest, says: "strategy",
			mutate: func(b map[string]any) { b["strategy"] = "sliding_window" },
		},
		{
			name: "an_unknown_scope", want: http.StatusBadRequest, says: "scope",
			mutate: func(b map[string]any) { b["scope"] = "organisation" },
		},
		{
			name: "star_mixed_with_a_named_verb", want: http.StatusBadRequest, says: "methods",
			mutate: func(b map[string]any) { b["methods"] = []string{"*", "GET"} },
		},
		{
			name: "no_windows", want: http.StatusBadRequest, says: "windows",
			mutate: func(b map[string]any) { b["windows"] = []map[string]any{} },
		},
		{
			name: "two_windows_on_one_period", want: http.StatusBadRequest, says: "windows",
			mutate: func(b map[string]any) {
				b["windows"] = []map[string]any{
					{"requests": 1, "period": "1s"},
					{"requests": 2, "period": "1s"},
				}
			},
		},
		{
			name: "a_period_that_is_not_a_duration", want: http.StatusBadRequest, says: "duration",
			mutate: func(b map[string]any) {
				b["windows"] = []map[string]any{{"requests": 1, "period": "60"}}
			},
		},
		{
			name: "a_prefix_without_a_trailing_slash", want: http.StatusBadRequest, says: "target",
			mutate: func(b map[string]any) {
				b["target_kind"] = "prefix"
				b["target"] = "/projects"
			},
		},
		{
			name: "a_guest_rule_scoped_to_user", want: http.StatusBadRequest, says: "scope",
			mutate: func(b map[string]any) {
				b["audience"] = "guest"
				b["scope"] = "user"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := validRateLimitBody(uuid.NewV7())
			tt.mutate(body)

			resp, err := sendHTTPRequest(t, t.Context(), rateLimitCreateEndpoint, body, hdr)
			require.NoError(t, err)

			defer resp.Body.Close()

			got := readResponseBody(t, resp)
			assert.Equal(t, tt.want, resp.StatusCode, "body: %s", got)
			assert.Contains(t, got, tt.says, "the refusal should say what is wrong, not only that something is")
		})
	}
}

// The valid body itself must be ACCEPTED, or every rejection above passes for
// the wrong reason.
func TestRateLimitsAcceptsTheValidBodyTheRejectionsMutate(t *testing.T) {
	tokens := getAdminUserTokens(t)

	id := uuid.NewV7()
	createTestRateLimit(t, tokens.AccessToken, validRateLimitBody(id))
	deleteTestRateLimit(t, tokens.AccessToken, id)
}

func TestRateLimitsDuplicateNameIs409(t *testing.T) {
	tokens := getAdminUserTokens(t)
	hdr := map[string]string{"Authorization": "Bearer " + tokens.AccessToken}

	first := uuid.NewV7()
	body := validRateLimitBody(first)
	createTestRateLimit(t, tokens.AccessToken, body)

	t.Cleanup(func() { deleteTestRateLimit(t, tokens.AccessToken, first) })

	// Same name, different id.
	second := validRateLimitBody(uuid.NewV7())
	second["name"] = body["name"]

	resp, err := sendHTTPRequest(t, t.Context(), rateLimitCreateEndpoint, second, hdr)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusConflict, resp.StatusCode,
		"a duplicate name is unique_rate_limit_name, which must map to 409 and not 500: %s", readResponseBody(t, resp))
}

func TestRateLimitsGetUnknownIs404(t *testing.T) {
	tokens := getAdminUserTokens(t)
	hdr := map[string]string{"Authorization": "Bearer " + tokens.AccessToken}

	resp, err := sendHTTPRequest(t, t.Context(), rateLimitGetEndpoint.RewriteSlugs(uuid.NewV7().String()), nil, hdr)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestRateLimitsGetMalformedIDIs400(t *testing.T) {
	tokens := getAdminUserTokens(t)
	hdr := map[string]string{"Authorization": "Bearer " + tokens.AccessToken}

	resp, err := sendHTTPRequest(t, t.Context(), rateLimitGetEndpoint.RewriteSlugs("not-a-uuid"), nil, hdr)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// The endpoint that makes the precedence ladder usable. It resolves with the
// same function the middleware uses, so it cannot disagree with what is
// enforced.
func TestRateLimitsEffective(t *testing.T) {
	tokens := getAdminUserTokens(t)
	hdr := map[string]string{"Authorization": "Bearer " + tokens.AccessToken}

	// Its own target, and every pre-existing rule for it removed first.
	//
	// Two endpoint rules on one path tie at the same rung, and the tie-break is
	// by NAME -- so a rule left behind by an earlier run wins, and the
	// assertion below fails for a reason that has nothing to do with the code.
	// That is exactly what happened the first time this ran.
	const target = "/resources"

	clearRateLimitsForTarget(t, tokens.AccessToken, target)

	body := validRateLimitBody(uuid.NewV7())
	body["target"] = target

	id, err := uuid.Parse(body["id"].(string))
	require.NoError(t, err)

	createTestRateLimit(t, tokens.AccessToken, body)

	t.Cleanup(func() { deleteTestRateLimit(t, tokens.AccessToken, id) })

	t.Run("resolves_the_endpoint_rule", func(t *testing.T) {
		ep := rateLimitEffectiveEndpoint.Clone()
		ep.SetQueryParams(map[string]string{"method": "GET", "endpoint": target})

		resp, err := sendHTTPRequest(t, t.Context(), ep, nil, hdr)
		require.NoError(t, err)

		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", readResponseBody(t, resp))

		var got payload.EffectiveRateLimitsResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))

		// Branch on Enforcing, because the answer legitimately differs.
		//
		// This required Effective to be non-empty unconditionally, and could
		// therefore never pass in its own harness: tests/provisioning/
		// integration-test.yaml runs the service with `-ratelimit.enabled=false`,
		// which is also what CLAUDE.md prescribes -- the suite fires many
		// requests from one address, and a limit would make failures depend on
		// test order.
		//
		// With no mirror the endpoint returns an EMPTY set deliberately.
		// From usecase.RateLimitsService.Effective:
		//
		//   No mirror at all means ratelimit.enabled=false, and then the honest
		//   answer is that NOTHING applies -- not "here are the rules that
		//   would apply". Returning them would repeat the mistake this endpoint
		//   was already fixed for once: describing rules that are not being
		//   enforced as the effective ones.
		//
		// So an empty set under `enforcing:false` is the contract working, and
		// asserting the opposite was asserting the bug. Both branches are
		// checked so the test verifies something in either configuration
		// rather than being skipped into silence.
		if !got.Enforcing {
			assert.Empty(t, got.Effective,
				"with ratelimit.enabled=false nothing is enforced, so nothing may be reported as effective")

			return
		}

		require.NotEmpty(t, got.Effective, "at least the seeded global rule should apply")

		found := false

		for _, e := range got.Effective {
			if e.RuleID == id {
				found = true
				assert.NotEmpty(t, e.Why, "the explanation is the point of this endpoint; an empty one answers nothing")
				assert.NotEmpty(t, e.Windows)
			}
		}

		assert.True(t, found, "the endpoint rule should win its scope over the global rule")
	})

	t.Run("method_is_required", func(t *testing.T) {
		ep := rateLimitEffectiveEndpoint.Clone()
		ep.SetQueryParam("endpoint", target)

		resp, err := sendHTTPRequest(t, t.Context(), ep, nil, hdr)
		require.NoError(t, err)

		defer resp.Body.Close()

		// Without it the answer is ambiguous: naming a verb beats * at every
		// tier, and an endpoint usually has rules for both.
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "body: %s", readResponseBody(t, resp))
	})

	t.Run("endpoint_is_required", func(t *testing.T) {
		ep := rateLimitEffectiveEndpoint.Clone()
		ep.SetQueryParam("method", "GET")

		resp, err := sendHTTPRequest(t, t.Context(), ep, nil, hdr)
		require.NoError(t, err)

		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "body: %s", readResponseBody(t, resp))
	})
}

func TestRateLimitsRequiresAuthentication(t *testing.T) {
	resp, err := sendHTTPRequest(t, t.Context(), rateLimitListEndpoint, nil, nil)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}
