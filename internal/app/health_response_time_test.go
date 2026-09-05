//go:build unit

package app

import (
	"testing"
	"time"
)

// TestFormatHealthDurationKeepsSubMillisecondChecks is the regression this file
// exists for.
//
// The field carried `ResponseTimeMs int64`, set from `duration.Milliseconds()`,
// and the converter blanked it whenever it was zero. A healthy local database
// pings in a few hundred microseconds, so every check truncated to 0 and was
// then reported as never measured -- /health/detailed carried no response_time
// at all, for any component, ever. The published example for the field is
// "2.5ms", which milliseconds alone cannot even express.
func TestFormatHealthDurationKeepsSubMillisecondChecks(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		in   time.Duration
		want string
	}{
		{"a fast local ping", 412 * time.Microsecond, "412µs"},
		{"just under a millisecond", 999 * time.Microsecond, "999µs"},
		{"the documented example", 2500 * time.Microsecond, "2.5ms"},
		{"a slow dependency", 1500 * time.Millisecond, "1.50s"},
		{"faster than a microsecond", 800 * time.Nanosecond, "800ns"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := formatHealthDuration(tc.in); got != tc.want {
				t.Errorf("formatHealthDuration(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestConvertToModelHealthDistinguishesUnmeasuredFromInstant pins the other half
// of the bug: zero was overloaded to mean "not measured".
func TestConvertToModelHealthDistinguishesUnmeasuredFromInstant(t *testing.T) {
	t.Parallel()

	instant := time.Duration(0)
	fast := 300 * time.Microsecond

	app := &App{}
	got := app.convertToModelHealth(&AppHealth{
		Components: map[string]ComponentHealth{
			"never-measured": {Status: ComponentStatusHealthy},
			"measured-zero":  {Status: ComponentStatusHealthy, ResponseTime: &instant},
			"measured-fast":  {Status: ComponentStatusHealthy, ResponseTime: &fast},
		},
	})

	if rt := got.Components["never-measured"].ResponseTime; rt != "" {
		t.Errorf("a component that was never timed should report no response_time, got %q", rt)
	}
	// A measurement that rounds to nothing is still a measurement.
	if rt := got.Components["measured-zero"].ResponseTime; rt == "" {
		t.Error("a measured zero duration must not be reported as unmeasured")
	}
	if rt := got.Components["measured-fast"].ResponseTime; rt != "300µs" {
		t.Errorf("measured-fast response_time = %q, want %q", rt, "300µs")
	}
}
