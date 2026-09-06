package usecase

import (
	"context"
	"errors"
	"testing"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

type fakeGrants struct {
	out *domain.SelectAuthzOutput
	err error
}

func (f fakeGrants) SelectAuthz(_ context.Context, _ uuid.UUID) (*domain.SelectAuthzOutput, error) {
	return f.out, f.err
}

type fakePolicies struct {
	byID   map[uuid.UUID]*domain.Policy
	byRole map[uuid.UUID][]domain.Policy
}

func (f fakePolicies) SelectByID(_ context.Context, id uuid.UUID) (*domain.Policy, error) {
	if p, ok := f.byID[id]; ok {
		return p, nil
	}
	return nil, &domain.PolicyNotFoundError{ID: id}
}

func (f fakePolicies) SelectByRoleID(_ context.Context, roleID uuid.UUID, _ *domain.SelectPoliciesInput) (*domain.SelectPoliciesOutput, error) {
	return &domain.SelectPoliciesOutput{Items: f.byRole[roleID]}, nil
}

func held(user uuid.UUID, grants map[string]any) *domain.SelectAuthzOutput {
	return &domain.SelectAuthzOutput{Permissions: map[string]any{"permissions": map[string]any{"users": map[string]any{user.String(): grants}}}}
}

func TestGrantGuard(t *testing.T) {
	t.Parallel()

	caller := uuid.NewV7()
	allowAll := uuid.NewV7()
	readRoles := uuid.NewV7()
	role := uuid.NewV7()

	policies := fakePolicies{
		byID: map[uuid.UUID]*domain.Policy{
			allowAll:  {ID: allowAll, AllowedAction: "*", AllowedResource: "*"},
			readRoles: {ID: readRoles, AllowedAction: "GET", AllowedResource: "/roles"},
		},
		byRole: map[uuid.UUID][]domain.Policy{role: {{AllowedAction: "GET", AllowedResource: "/roles"}, {AllowedAction: "POST", AllowedResource: "/roles"}}},
	}

	guard := func(t *testing.T, grants fakeGrants) *GrantGuard {
		t.Helper()
		g, err := NewGrantGuard(GrantGuardConf{Grants: grants, Policies: policies, OT: testOT(t)})
		if err != nil {
			t.Fatal(err)
		}
		return g
	}

	refused := func(err error) bool {
		_, ok := errors.AsType[*domain.GrantNotHeldError](err)
		return ok
	}

	t.Run("an_administrator_may_grant_anything", func(t *testing.T) {
		t.Parallel()
		g := guard(t, fakeGrants{out: held(caller, map[string]any{"*": []any{"*"}})})
		if err := g.CheckPolicies(context.Background(), caller, []uuid.UUID{allowAll}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("a_caller_may_grant_exactly_what_they_hold", func(t *testing.T) {
		t.Parallel()
		g := guard(t, fakeGrants{out: held(caller, map[string]any{"/roles": []any{"GET"}})})
		if err := g.CheckPolicies(context.Background(), caller, []uuid.UUID{readRoles}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("a_caller_may_not_grant_what_they_do_not_hold", func(t *testing.T) {
		t.Parallel()
		// The measured escalation: holder of POST /policies mints allow-all.
		g := guard(t, fakeGrants{out: held(caller, map[string]any{"/policies": []any{"POST"}})})
		if err := g.CheckGrants(context.Background(), caller, []domain.Grant{{Action: "*", Resource: "*"}}); !refused(err) {
			t.Fatalf("got %v, want a GrantNotHeldError", err)
		}
	})

	t.Run("a_role_is_the_sum_of_its_policies", func(t *testing.T) {
		t.Parallel()
		// Holds GET on /roles but not POST: the role carries both.
		g := guard(t, fakeGrants{out: held(caller, map[string]any{"/roles": []any{"GET"}})})
		if err := g.CheckRoles(context.Background(), caller, []uuid.UUID{role}); !refused(err) {
			t.Fatalf("got %v, want a GrantNotHeldError", err)
		}
		g2 := guard(t, fakeGrants{out: held(caller, map[string]any{"/roles": []any{"*"}})})
		if err := g2.CheckRoles(context.Background(), caller, []uuid.UUID{role}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("a_global_method_grant_covers_that_method_anywhere", func(t *testing.T) {
		t.Parallel()
		g := guard(t, fakeGrants{out: held(caller, map[string]any{"*": []any{"GET"}})})
		if err := g.CheckGrants(context.Background(), caller, []domain.Grant{{Action: "GET", Resource: "/anything"}}); err != nil {
			t.Fatal(err)
		}
		if err := g.CheckGrants(context.Background(), caller, []domain.Grant{{Action: "POST", Resource: "/anything"}}); !refused(err) {
			t.Fatalf("got %v, want a GrantNotHeldError", err)
		}
	})

	t.Run("a_narrower_pattern_is_not_derived", func(t *testing.T) {
		t.Parallel()
		// Sound in theory, refused on purpose: narrowing is the engine's job.
		g := guard(t, fakeGrants{out: held(caller, map[string]any{"/projects/*": []any{"GET"}})})
		if err := g.CheckGrants(context.Background(), caller, []domain.Grant{{Action: "GET", Resource: "/projects/01a02b03-0000-7000-8000-000000000009"}}); !refused(err) {
			t.Fatalf("got %v, want a GrantNotHeldError", err)
		}
	})

	t.Run("a_store_fault_or_a_nil_caller_refuses", func(t *testing.T) {
		t.Parallel()
		g := guard(t, fakeGrants{err: errors.New("db away")})
		if err := g.CheckGrants(context.Background(), caller, nil); err == nil {
			t.Fatal("a store fault must refuse")
		}
		g2 := guard(t, fakeGrants{out: held(caller, map[string]any{"*": []any{"*"}})})
		if err := g2.CheckGrants(context.Background(), uuid.Nil(), nil); err == nil {
			t.Fatal("a nil caller must refuse")
		}
	})
}
