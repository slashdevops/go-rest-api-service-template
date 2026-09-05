package app

import (
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/middleware"
)

// There is ONE limiter.
//
// This replaces TestTheRuleLimiterAndTheFlagLimiterAreMutuallyExclusive, which
// pinned an `else` keeping two limiters apart so neither charged a request the
// other had already charged. The flag limiter is gone -- budgets come from the
// rate_limits table and nowhere else -- so the property to protect is no longer
// "they alternate" but "there is only one of them to alternate with".
func TestThereIsOnlyOneLimiter(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}

	body := string(src)

	if strings.Contains(body, "IPRateLimiter") {
		t.Error("the per-IP flag limiter is registered again. It was removed because its budget was " +
			"also the rule limiter's fallback, so the two duplicated one number and disagreed about " +
			"exemptions. If it is genuinely wanted back, the mutual-exclusion guard has to come back with it.")
	}

	if n := strings.Count(body, "a.rateLimitMiddleware(middleware.RateLimitStage"); n != 2 {
		t.Errorf("the limiter is built %d times, want exactly 2 (one per stage)", n)
	}
}

// A nil Middleware appended to a chain is invoked and panics, so appendRateLimit
// has to drop it. This is the mechanism itself, not a reading of the source.
func TestAppendRateLimitDropsANilMiddleware(t *testing.T) {
	t.Parallel()

	chain := []middleware.Middleware{middleware.RewriteStandardErrorsAsJSON}

	got := appendRateLimit(chain, nil)
	if len(got) != len(chain) {
		t.Fatalf("appendRateLimit added a nil middleware: len %d -> %d; "+
			"a nil in a chain is invoked and panics on the first request", len(chain), len(got))
	}

	real := middleware.Middleware(func(next http.Handler) http.Handler { return next })

	got = appendRateLimit(chain, real)
	if len(got) != len(chain)+1 {
		t.Fatalf("appendRateLimit dropped a real middleware: len %d -> %d; "+
			"the post-auth stage would never run", len(chain), len(got))
	}
}

// Every use of the post-auth middleware must go through appendRateLimit.
//
// Counted rather than located: an earlier version of this scanned backwards a
// fixed number of characters for the enclosing call, which failed on correct
// code as soon as the argument list grew past the window.
func TestEveryPostAuthUseGoesThroughAppendRateLimit(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}

	body := string(src)

	// Count only the places the variable is READ. Its declaration and the
	// assignment that fills it are not uses, and counting "every occurrence
	// minus one" broke the moment the two were split apart -- which is what
	// wrapping it in the exemption gate required.
	total := len(regexp.MustCompile(`postAuthRateLimit`).FindAllString(body, -1))
	declared := len(regexp.MustCompile(`var postAuthRateLimit\b`).FindAllString(body, -1))
	assigned := len(regexp.MustCompile(`postAuthRateLimit\s*=[^=]`).FindAllString(body, -1))

	uses := total - declared - assigned
	if uses < 1 {
		t.Fatal("postAuthRateLimit is declared but never used; the post-auth stage would never run")
	}

	// Excluding the function's own declaration, which is in this file too.
	calls := len(regexp.MustCompile(`appendRateLimit\(`).FindAllString(body, -1)) -
		len(regexp.MustCompile(`func appendRateLimit\(`).FindAllString(body, -1))
	if calls != uses {
		t.Fatalf("postAuthRateLimit is read %d times but appendRateLimit is called %d times; "+
			"a use outside it appends a possibly-nil middleware, which is invoked and panics", uses, calls)
	}
}

// Every limiter registration must go through the exemption gate.
//
// This is the wiring half of the bug the gate was written for, and it is the
// half no unit test could catch: the exemptions were correct code, sitting
// inside the limiter that does NOT run by default. So /health answered 429 and
// ratelimit.excluded.ips did nothing, while every test of the exemptions passed
// -- because they all exercised the limiter that does have them.
//
// Structural, and read from the source for the same reason the mutual-exclusion
// test above is: standing up the whole App to assert where a wrapper sits is a
// worse trade than parsing three lines.
func TestEveryLimiterRegistrationGoesThroughTheExemptionGate(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}

	body := string(src)

	// Each way a limiter reaches a chain, and what it must be wrapped in.
	for _, reg := range []struct{ what, appended string }{
		{"the pre-auth limiter", "apiCommonMdws = append(apiCommonMdws, exemptions.Wrap(mdw))"},
		{"the post-auth limiter", "postAuthRateLimit = exemptions.Wrap(mdw)"},
	} {
		if !strings.Contains(body, reg.appended) {
			t.Errorf("%s is not wrapped in the exemption gate.\n"+
				"Expected to find:\n  %s\n"+
				"An unwrapped limiter rate limits /health and ignores ratelimit.excluded.ips, "+
				"which is exactly what shipped when the exemptions lived inside one limiter.",
				reg.what, reg.appended)
		}
	}

	// The gate must be built ABOVE the limiter, not inside it: an excluded
	// address has to be exempt even when rate limiting is switched off, and an
	// exempt request has to SKIP the limiter rather than be allowed by it.
	gate := strings.Index(body, "a.rateLimitExemptions(clientIP)")
	first := strings.Index(body, "a.rateLimitMiddleware(middleware.RateLimitStage")

	if gate < 0 {
		t.Fatal("the exemption gate is no longer built in server.go")
	}

	if gate > first {
		t.Fatal("the exemption gate is built after the limiter; it must be built above it")
	}
}
