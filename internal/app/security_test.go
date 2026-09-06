package app

import (
	"os"
	"strings"
	"testing"
)

// TestEveryRequestIsRecoveredBoundedAndHeadered reads server.go, the way the
// rate-limit exemption test does, and pins the part of the middleware order
// that a refactor is most likely to lose without any test noticing.
//
// Recovery was defined, documented as outermost, and wired nowhere for years;
// a panic dropped the connection. The security headers and the body bound are
// new and would be just as invisible if dropped: every request still succeeds.
func TestEveryRequestIsRecoveredBoundedAndHeadered(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}

	body := string(src)

	chain := strings.Index(body, "apiCommonMdws := []middleware.Middleware{")
	if chain < 0 {
		t.Fatal("the common middleware chain is no longer built as apiCommonMdws in server.go")
	}

	// Outermost first; each must appear, in this order, before the chain is closed.
	order := []string{
		"middleware.RequestID,",
		"middleware.Recovery,",
		"middleware.SecurityHeaders(",
		"middleware.MaxBody(",
		"middleware.RequireJSONBody,",
	}

	pos := chain
	for _, want := range order {
		i := strings.Index(body[pos:], want)
		if i < 0 {
			t.Fatalf("%s is not in the common chain after %q; every request must pass through it", want, body[pos:pos+40])
		}
		pos += i
	}

	closing := strings.Index(body[chain:], "\n\t}\n")
	if closing >= 0 && chain+closing < pos {
		t.Fatalf("the common chain closes before %s; it must be inside apiCommonMdws", order[len(order)-1])
	}

	// CORS answers preflights itself, so it must sit inside (after) the
	// pre-auth limiter or every OPTIONS is an unmetered request.
	limiter := strings.Index(body, "apiCommonMdws = append(apiCommonMdws, exemptions.Wrap(mdw))")
	cors := strings.Index(body, "middleware.Cors(corsOpts)")

	if limiter < 0 || cors < 0 {
		t.Fatal("the pre-auth limiter or the CORS middleware is no longer appended in server.go")
	}

	if cors < limiter {
		t.Fatal("CORS is appended before the pre-auth rate limiter; a preflight would bypass it")
	}
}

// TestProjectMembershipIsCheckedAfterAuthorization pins the one order that
// matters for the membership middleware: after CheckAuthz in the chain every
// authenticated route uses, so a caller with no grant is refused with 403
// before membership is even asked, and a caller with a grant on
// /projects/*/... is still refused for a project they are not in.
func TestProjectMembershipIsCheckedAfterAuthorization(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}

	body := string(src)
	chain := strings.Index(body, "accessTokenMiddlewares := middleware.Chain(")
	if chain < 0 {
		t.Fatal("accessTokenMiddlewares is no longer built in server.go")
	}

	end := strings.Index(body[chain:], "postAuthRateLimit)...,")
	if end < 0 {
		t.Fatal("the access-token chain no longer ends with the post-auth limiter")
	}

	segment := body[chain : chain+end]
	authz := strings.Index(segment, "middleware.CheckAuthz(")
	membership := strings.Index(segment, "middleware.RequireProjectMembership(")

	if authz < 0 || membership < 0 {
		t.Fatalf("the access-token chain must contain CheckAuthz and RequireProjectMembership; found authz=%d membership=%d", authz, membership)
	}

	if membership < authz {
		t.Fatal("RequireProjectMembership runs before CheckAuthz; a caller with no grant would get 404 instead of 403")
	}
}
