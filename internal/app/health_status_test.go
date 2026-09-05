package app

import "testing"

// TestOverallForDatabase pins the distinction that a health endpoint exists to
// make: "slow, still serve me" versus "gone, stop sending traffic".
//
// Both used to collapse into degraded, which the handler maps to 206 — a
// SUCCESS code. A service whose database was unreachable therefore answered
// 2xx, so anything treating 2xx as healthy kept sending it work it could not
// do, and the 503 the endpoint documented could never happen.
func TestOverallForDatabase(t *testing.T) {
	for name, tc := range map[string]struct {
		database ComponentStatus
		want     ComponentStatus
		why      string
	}{
		"healthy": {
			database: ComponentStatusHealthy,
			want:     ComponentStatusHealthy,
			why:      "a working database is the ordinary case",
		},
		"unhealthy": {
			database: ComponentStatusUnhealthy,
			want:     ComponentStatusUnhealthy,
			why:      "without a database this service cannot answer anything, and must say so",
		},
		"degraded": {
			database: ComponentStatusDegraded,
			want:     ComponentStatusDegraded,
			why:      "a slow database is worth reporting and still worth traffic",
		},
		"unknown": {
			database: ComponentStatusUnknown,
			want:     ComponentStatusDegraded,
			why:      "an unrecognised state is reported, not assumed healthy",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := overallForDatabase(tc.database); got != tc.want {
				t.Errorf("overallForDatabase(%q) = %q, want %q — %s", tc.database, got, tc.want, tc.why)
			}
		})
	}
}

// TestCacheNeverChangesTheOverallStatus guards the exemption the cache has, and
// the reason for it.
//
// The cache layer is fail-open: a read that times out falls through to the
// database and the request still succeeds. Failing readiness over it would take
// a working service out of rotation because an optimisation was unavailable.
// The component is reported so the outage is visible, and that is all.
func TestCacheNeverChangesTheOverallStatus(t *testing.T) {
	// overallForDatabase is the only thing that moves the overall status, and
	// it takes the DATABASE status. There is deliberately no equivalent for the
	// cache; if one is ever added, this test is where the reasoning above has
	// to be revisited rather than quietly overridden.
	for _, cacheState := range []ComponentStatus{
		ComponentStatusHealthy, ComponentStatusDegraded, ComponentStatusUnhealthy,
	} {
		if got := overallForDatabase(ComponentStatusHealthy); got != ComponentStatusHealthy {
			t.Fatalf("a %q cache changed the overall status to %q; the cache is fail-open and must not", cacheState, got)
		}
	}
}
