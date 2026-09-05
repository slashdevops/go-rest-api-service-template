//go:build unit

package app

import (
	"strings"
	"testing"

	"github.com/slashdevops/go-rest-api-service-template/internal/config"
)

// TestFailModeConsequenceNamesWhatTheOperatorWillSee checks that the health
// message says what a store outage actually does, which the mode name alone
// does not.
//
// The two modes fail in opposite directions and the difference is the whole
// reason ratelimit.store.fail.mode exists: closed refuses everything, local
// keeps serving but multiplies the effective limit by the replica count. An
// operator reading "the shared rate-limit counter is unreachable" needs to know
// which of those they are living with, and it is not derivable from the
// component's status.
func TestFailModeConsequenceNamesWhatTheOperatorWillSee(t *testing.T) {
	closed := failModeConsequence(config.RateLimitFailModeClosed)
	if !strings.Contains(closed, "429") {
		t.Errorf("fail-closed consequence should name the status callers get, got %q", closed)
	}

	local := failModeConsequence(config.RateLimitFailModeLocal)
	if !strings.Contains(local, "replica") {
		t.Errorf("fail-local consequence should say the limit is per replica, got %q", local)
	}

	if closed == local {
		t.Error("the two fail modes must not describe the same consequence: " +
			"one refuses every request, the other serves them all")
	}
}

// TestUnknownFailModeDescribesTheRefusingBehaviour pins the default branch.
//
// Config validation restricts the mode, so this is about what happens if that
// ever slips: describing an unknown mode as the permissive one would tell an
// operator the service is still serving when it may not be. The refusing
// description is the safe default to be wrong in.
func TestUnknownFailModeDescribesTheRefusingBehaviour(t *testing.T) {
	if got := failModeConsequence("something-else"); !strings.Contains(got, "refusing") {
		t.Errorf("an unrecognised fail mode should describe refusal, got %q", got)
	}
}
