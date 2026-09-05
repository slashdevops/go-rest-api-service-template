package throttlememory_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driven/throttlememory"
)

func newThrottle(t *testing.T, maxAttempts int, window time.Duration) *throttlememory.Throttle {
	t.Helper()

	th := throttlememory.New(throttlememory.Conf{
		MaxAttempts: maxAttempts,
		Window:      window,
		IdleAfter:   time.Minute,
	})
	t.Cleanup(func() { _ = th.Close() })

	return th
}

// TestThrottle_successNeverAccumulates is the property that matters for a
// legitimate user: signing in correctly, however often, must never approach the
// ceiling. Attempt spends and Succeed refunds, so the net is zero.
func TestThrottle_successNeverAccumulates(t *testing.T) {
	t.Parallel()

	th := newThrottle(t, 3, time.Hour)

	for i := range 50 {
		_, allowed := th.Attempt("account")
		require.True(t, allowed, "successful login %d must not be refused", i+1)
		th.Succeed("account")
	}
}

func TestThrottle_refusesAfterMaxFailures(t *testing.T) {
	t.Parallel()

	const maxAttempts = 3
	th := newThrottle(t, maxAttempts, time.Hour)

	for i := range maxAttempts {
		_, allowed := th.Attempt("account")
		require.True(t, allowed, "attempt %d should be allowed", i+1)
	}

	retryAfter, allowed := th.Attempt("account")

	assert.False(t, allowed, "the budget is spent after %d failures", maxAttempts)
	assert.Positive(t, retryAfter, "a refused caller must be told when to come back")
}

func TestThrottle_successResetsTheBudget(t *testing.T) {
	t.Parallel()

	th := newThrottle(t, 3, time.Hour)

	for range 3 {
		th.Attempt("account")
	}

	_, allowed := th.Attempt("account")
	require.False(t, allowed, "precondition: the budget is spent")

	// What a correct password does.
	th.Succeed("account")

	_, allowed = th.Attempt("account")
	assert.True(t, allowed, "a successful login must restore the full budget")
}

func TestThrottle_budgetsAreIndependentPerKey(t *testing.T) {
	t.Parallel()

	th := newThrottle(t, 2, time.Hour)

	th.Attempt("victim")
	th.Attempt("victim")

	_, allowed := th.Attempt("victim")
	require.False(t, allowed, "precondition: the first account is refused")

	_, allowed = th.Attempt("bystander")
	assert.True(t, allowed, "one account being guessed at must not refuse everybody else")
}

func TestThrottle_refillsOverTheWindow(t *testing.T) {
	t.Parallel()

	// 4 attempts over 400ms, so one token returns every 100ms.
	th := newThrottle(t, 4, 400*time.Millisecond)

	for range 4 {
		th.Attempt("account")
	}

	_, allowed := th.Attempt("account")
	require.False(t, allowed, "precondition: the budget is spent")

	// Real time rather than synctest: the bucket reads the wall clock through
	// golang.org/x/time/rate, which a synctest bubble does not virtualise.
	time.Sleep(150 * time.Millisecond)

	_, allowed = th.Attempt("account")
	assert.True(t, allowed, "the budget must refill on its own; a throttle that never recovers is a lockout")
}

func TestThrottle_retryAfterShrinksAsTheBudgetRefills(t *testing.T) {
	t.Parallel()

	th := newThrottle(t, 2, 2*time.Second)

	th.Attempt("account")
	th.Attempt("account")

	first, allowed := th.Attempt("account")
	require.False(t, allowed)

	time.Sleep(300 * time.Millisecond)

	second, allowed := th.Attempt("account")
	require.False(t, allowed, "still inside the window")

	assert.Less(t, second, first, "the wait must count down rather than restart on every check")
}
