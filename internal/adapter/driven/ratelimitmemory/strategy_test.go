package ratelimitmemory

import (
	"slices"
	"testing"
	"testing/synctest"
	"time"
)

// admissionPattern charges one bucket on a fixed schedule and records which
// requests were admitted.
//
// The schedule is the point: a burst, then a pause shorter than the window, then
// a pause longer than it. A single back-to-back burst cannot tell a budget from
// a pace, because both admit exactly their capacity at t=0.
func admissionPattern(t *testing.T, strategy string) []bool {
	t.Helper()

	a := New()
	b := budget(10, time.Second, 3, strategy)

	var got []bool

	charge := func(n int) {
		for range n {
			d, err := a.Allow(t.Context(), "k", b, 1)
			if err != nil {
				t.Fatalf("%s: %v", strategy, err)
			}

			got = append(got, d.Allowed)
		}
	}

	charge(6)
	synctest.Sleep(250 * time.Millisecond)
	charge(3)
	synctest.Sleep(2 * time.Second)
	charge(5)

	return got
}

// The two strategies admit IDENTICALLY at equal parameters.
//
// CLAUDE.md and the architecture doc both state this as measured, and the UI is
// required to present the column as "budget vs pace" rather than "bursty vs
// smooth" because of it -- but nothing verified it, so the claim rested on a
// measurement nobody could repeat. If it ever stops being true, the wording in
// three documents becomes wrong at once and the operator-facing help text is
// the thing that misleads.
//
// It also fixes what the tracker asked for and could not get. The open item
// wanted a test proving leaky_bucket PACES where token_bucket BURSTS. No such
// test can exist: at equal parameters there is no observable difference, so any
// test that appears to show one is really showing something else. The first
// attempt here gave the two different bursts and passed with the strategy
// hardcoded to token_bucket -- it was measuring burst, not strategy.
func TestBothStrategiesAdmitIdenticallyAtEqualParameters(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		token := admissionPattern(t, "token_bucket")
		leaky := admissionPattern(t, "leaky_bucket")

		if !slices.Equal(token, leaky) {
			t.Fatalf("the two strategies diverged at equal parameters:\n  token_bucket: %v\n  leaky_bucket: %v\n"+
				"they are duals, and the UI sells the column on that being true", token, leaky)
		}

		// A pattern that refused nothing would make the comparison vacuous --
		// two limiters that admit everything also "agree".
		if !slices.Contains(token, false) {
			t.Fatalf("nothing was refused, so agreement proves nothing: %v", token)
		}

		if !slices.Contains(token, true) {
			t.Fatalf("nothing was admitted, so agreement proves nothing: %v", token)
		}
	})
}
