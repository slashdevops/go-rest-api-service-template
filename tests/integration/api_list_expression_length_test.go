//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// TestAnOverLongListExpressionIsRefusedAndTheServiceStillAnswers: a list
// expression over its length is refused before it is parsed.
//
// A query string is bounded only by the header limit, and the filter parser
// is recursive: before qfv v1.0.3 a deeply nested filter overflowed the
// stack, which is a fatal error of the runtime and not a panic. The length
// check was there on six lists and did not stop the parse; four had none.
//
// If this test fails by losing its connection, the service under test is
// gone, and every test after it fails with it.
func TestAnOverLongListExpressionIsRefusedAndTheServiceStillAnswers(t *testing.T) {
	headers := map[string]string{"Authorization": "Bearer " + getAdminUserTokens(t).AccessToken}

	// One refusal is answered as that refusal; two or more as "validation
	// failed with N errors: …". An expression that was found too long and
	// parsed anyway was two.
	const lengthSays = "exceeds maximum length"

	for _, tt := range []struct {
		name   string
		path   string
		filter string
	}{
		// /users checked the length and parsed anyway: two refusals.
		{name: "nested_and_over_the_limit", path: "/users", filter: strings.Repeat("(", 300_000)},
		// /policies had no length check. A long filter that parses was
		// accepted and sent on to the database, and a nested one was the
		// parser's to refuse.
		{name: "one_character_over_the_limit", path: "/policies", filter: "name = '" + strings.Repeat("a", domain.MaxFilterExpressionLength-len("name = ''")+1) + "'"},
		{name: "a_long_filter_that_parses", path: "/policies", filter: strings.Repeat("name = 'a' OR ", 200) + "name = 'a'"},
		{name: "nested_on_a_list_that_had_no_length_check", path: "/policies", filter: strings.Repeat("(", 300_000)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			endpoint := newAPIEndpoint(http.MethodGet, tt.path)
			endpoint.SetQueryParam("filter", tt.filter)

			status, body := getRetryingTooManyRequests(t, endpoint, headers)
			require.Equal(t, http.StatusBadRequest, status, "body: %.300s", body)
			assert.Contains(t, body, lengthSays, "the refusal is the length's: %.300s", body)
			assert.NotContains(t, body, "validation failed with", "the filter is refused once, by its length, and by nothing that parsed it: %.300s", body)

			// The process that answered is still there.
			status, body = getRetryingTooManyRequests(t, newAPIEndpoint(http.MethodGet, tt.path), headers)
			assert.Equal(t, http.StatusOK, status, "the service does not answer after the over-long filter. Body: %.300s", body)
		})
	}

	for param, limit := range map[string]int{"sort": domain.MaxSortExpressionLength, "fields": domain.MaxFieldsExpressionLength} {
		t.Run(param+"_over_its_limit", func(t *testing.T) {
			endpoint := newAPIEndpoint(http.MethodGet, "/policies")
			endpoint.SetQueryParam(param, strings.Repeat("name,", limit))

			status, body := getRetryingTooManyRequests(t, endpoint, headers)
			require.Equal(t, http.StatusBadRequest, status, "body: %.300s", body)
			assert.Contains(t, body, lengthSays, "body: %.300s", body)
		})
	}
}
