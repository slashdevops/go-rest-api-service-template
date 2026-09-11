package middleware

import (
	"context"
	"net/http"
)

// subjectKey carries the per-request subject holder.
type subjectKey struct{}

// requestSubject is what a request turned out to be about, filled in as the
// chain learns it.
//
// # Why a mutable holder and not a context value
//
// The obvious thing is context.WithValue in the middleware that learns the
// subject. It does not work, for the same reason the server span could not
// start in the handler: a context flows DOWN and never back up. The
// authentication middleware puts the claims on a NEW request and passes it
// down; the access log runs above it and holds the request it passed down,
// which never gains that value. So by the time the line is written, the
// identity the middleware verified is out of reach.
//
// A pointer placed in the context ONCE, above everything that fills it, is
// reachable from both sides: the middleware below writes through the pointer,
// the access log above reads the same pointer. It is the pattern otelhttp uses
// for its labeler, and for the same reason.
//
// # On concurrency
//
// Both the writes and the read happen on the request's own goroutine -- the
// writes in the authentication and membership middlewares as the chain
// descends, the read in Logging after next.ServeHTTP has returned. That is a
// happens-before, so no lock is needed. A handler that hands this pointer to
// another goroutine would be introducing a race, which is why nothing outside
// this package can reach it.
type requestSubject struct {
	userID    string
	tokenType string
	projectID string
	admin     bool
}

// WithSubject installs the holder. It is called once, high in the chain, by
// [Tracing]; everything that fills it runs below.
func WithSubject(ctx context.Context) context.Context {
	return context.WithValue(ctx, subjectKey{}, &requestSubject{})
}

// subjectFrom returns the holder, or nil when the chain did not install one --
// a handler driven directly by a test, for instance.
func subjectFrom(ctx context.Context) *requestSubject {
	holder, _ := ctx.Value(subjectKey{}).(*requestSubject)

	return holder
}

// SetSubject records who a verified request is from.
//
// The user id is the `sub` claim, which is a uuid. The address is deliberately
// NOT recorded: an id identifies the account for an operator without putting a
// person's e-mail in a retained, searchable store. See
// TestNoSensitiveKeysAboveTrace.
func SetSubject(ctx context.Context, userID, tokenType string) {
	if holder := subjectFrom(ctx); holder != nil {
		holder.userID = userID
		holder.tokenType = tokenType
	}
}

// SetSubjectProject records which project a request is scoped to.
//
// The tenant boundary is the one dimension every RAG metric and log line was
// aggregated across: a slow or failing project was invisible, because nothing
// on the line said which one it was.
func SetSubjectProject(ctx context.Context, projectID string, admin bool) {
	if holder := subjectFrom(ctx); holder != nil {
		holder.projectID = projectID
		holder.admin = admin
	}
}

// logAttrs returns the subject as log attributes, omitting what was never
// filled in. An anonymous request produces none, rather than a line padded
// with empty strings that read as "the empty user".
func (s *requestSubject) logAttrs() []any {
	if s == nil {
		return nil
	}

	var out []any

	if s.userID != "" {
		out = append(out, "user_id", s.userID)
	}

	if s.tokenType != "" {
		out = append(out, "token_type", s.tokenType)
	}

	if s.projectID != "" {
		out = append(out, "project_id", s.projectID)
	}

	if s.admin {
		out = append(out, "project_admin", true)
	}

	return out
}

// subjectOf is the holder for a request, for callers that have the request
// rather than the context.
func subjectOf(r *http.Request) *requestSubject {
	return subjectFrom(r.Context())
}
