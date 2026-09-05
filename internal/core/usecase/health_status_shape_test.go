//go:build unit

package usecase_test

import (
	"encoding/json"
	"testing"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// TestStatusCheckCarriesNothingButItsVerdict guards the shape of the public
// health endpoint.
//
// /health/status is unauthenticated and exempt from the rate limiter, which is
// what makes anything carried on it both a disclosure and a cheap amplifier.
// go-rest-api-service-template#391 moved the runtime detail off it for exactly that reason --
// but left the `data` field behind, unpopulated and without `omitempty`, so the
// endpoint went on answering `"data": null` and publishing a free-form object
// in its schema that no caller could ever receive.
//
// A field is easy to add back to a struct and hard to notice in a response that
// a probe never reads. This fails if one appears.
func TestStatusCheckCarriesNothingButItsVerdict(t *testing.T) {
	encoded, err := json.Marshal(domain.Check{
		Name:   "database",
		Kind:   "pgx",
		Status: domain.StatusUp,
	})
	if err != nil {
		t.Fatalf("encoding a check: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("decoding a check: %v", err)
	}

	want := map[string]bool{"name": true, "kind": true, "status": true}

	for field := range fields {
		if !want[field] {
			t.Errorf("the public status endpoint gained a %q field. It is "+
				"unauthenticated and rate-limit exempt, so anything here is "+
				"served to anyone, for free, at any rate. Per-component detail "+
				"belongs on ComponentHealth, behind /health/detailed", field)
		}
	}

	for field := range want {
		if _, ok := fields[field]; !ok {
			t.Errorf("a check must still carry %q; a probe reads the verdict", field)
		}
	}
}
