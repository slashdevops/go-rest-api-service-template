package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

type fakeMembership struct {
	answer domain.ProjectMembership
	err    error
	calls  int
}

func (f *fakeMembership) Membership(_ context.Context, _, _ uuid.UUID) (domain.ProjectMembership, error) {
	f.calls++
	return f.answer, f.err
}

func TestRequireProjectMembership(t *testing.T) {
	t.Parallel()

	project := uuid.NewV7()
	user := uuid.NewV7()

	// A mux, so r.PathValue is populated the way it is in the real chain.
	serve := func(checker *fakeMembership, pattern, target string, withClaims bool) *httptest.ResponseRecorder {
		mux := http.NewServeMux()
		mux.Handle(pattern, RequireProjectMembership(checker)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})))

		req := httptest.NewRequest(http.MethodGet, target, nil)
		if withClaims {
			req = req.WithContext(context.WithValue(req.Context(), JwtClaims, map[string]any{"sub": user.String()}))
		}

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	t.Run("a_member_passes", func(t *testing.T) {
		t.Parallel()
		f := &fakeMembership{answer: domain.ProjectMembershipMember}
		rec := serve(f, "GET /projects/{project_id}/embedding_configs", "/projects/"+project.String()+"/embedding_configs", true)
		if rec.Code != http.StatusOK || f.calls != 1 {
			t.Fatalf("status %d, calls %d", rec.Code, f.calls)
		}
	})

	t.Run("an_administrator_passes", func(t *testing.T) {
		t.Parallel()
		f := &fakeMembership{answer: domain.ProjectMembershipAdmin}
		rec := serve(f, "GET /projects/{project_id}/embedding_configs", "/projects/"+project.String()+"/embedding_configs", true)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
	})

	t.Run("a_non_member_is_refused_as_not_found", func(t *testing.T) {
		t.Parallel()
		// 404, not 403: the same answer a missing project gets, so the refusal
		// does not confirm the project exists.
		f := &fakeMembership{answer: domain.ProjectMembershipNone}
		rec := serve(f, "GET /projects/{project_id}/embedding_configs", "/projects/"+project.String()+"/embedding_configs", true)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status %d, want 404: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("a_store_fault_fails_closed", func(t *testing.T) {
		t.Parallel()
		f := &fakeMembership{err: errors.New("db away")}
		rec := serve(f, "GET /projects/{project_id}/embedding_configs", "/projects/"+project.String()+"/embedding_configs", true)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status %d, want 500", rec.Code)
		}
	})

	t.Run("a_route_without_a_project_is_not_asked", func(t *testing.T) {
		t.Parallel()
		f := &fakeMembership{answer: domain.ProjectMembershipNone}
		rec := serve(f, "GET /roles", "/roles", true)
		if rec.Code != http.StatusOK || f.calls != 0 {
			t.Fatalf("status %d, calls %d; a route with no project_id must pass untouched", rec.Code, f.calls)
		}
	})

	t.Run("no_claims_is_401", func(t *testing.T) {
		t.Parallel()
		f := &fakeMembership{answer: domain.ProjectMembershipMember}
		rec := serve(f, "GET /projects/{project_id}/embedding_configs", "/projects/"+project.String()+"/embedding_configs", false)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status %d, want 401", rec.Code)
		}
	})
}
