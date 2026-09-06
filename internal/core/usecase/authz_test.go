package usecase

import (
	"context"
	"errors"
	"testing"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/policy"
)

type fakeAuthzUsers struct {
	perms map[string]any
	err   error
	calls int
}

func (f *fakeAuthzUsers) SelectAuthz(_ context.Context, _ uuid.UUID) (map[string]any, error) {
	f.calls++
	return f.perms, f.err
}

// tableEngine answers from the decision's own permission map with the one
// rule the use case relies on, method listed on the path or on "*". The
// real policy is tested against the real engine in policyopa; this test is
// about what the use case does around it, and core tests may not import
// an adapter.
type tableEngine struct{ err error }

func (e tableEngine) IsAllowed(_ context.Context, d policy.Decision) (bool, error) {
	if e.err != nil {
		return true, e.err // "true" beside an error must still be a deny
	}
	users, _ := d.Permissions["permissions"].(map[string]any)
	byUser, _ := users["users"].(map[string]any)
	grants, _ := byUser[d.UserID].(map[string]any)
	for _, key := range []string{d.Resource, "*"} {
		actions, _ := grants[key].([]any)
		for _, a := range actions {
			if a == d.Action || a == "*" {
				return true, nil
			}
		}
	}
	return false, nil
}

func wire(user uuid.UUID, grants map[string]any) map[string]any {
	return map[string]any{"permissions": map[string]any{"users": map[string]any{user.String(): grants}}}
}

// TestIsAuthorized covers what the use case does around the engine: the
// shape it hands over, and that every failure is a refusal.
func TestIsAuthorized(t *testing.T) {
	t.Parallel()

	engine := tableEngine{}
	user := uuid.NewV7()

	newService := func(t *testing.T, users UsersServiceConsumer, eng policy.Engine) *AuthzService {
		t.Helper()
		svc, err := NewAuthzService(AuthzServiceConf{UserService: users, PolicyEngine: eng, OT: testOT(t)})
		if err != nil {
			t.Fatal(err)
		}
		return svc
	}

	t.Run("a_grant_admits_and_a_missing_one_refuses", func(t *testing.T) {
		t.Parallel()
		svc := newService(t, &fakeAuthzUsers{perms: wire(user, map[string]any{"/roles": []any{"GET"}})}, engine)

		if ok, err := svc.IsAuthorized(context.Background(), user, "GET", "/roles"); err != nil || !ok {
			t.Fatalf("GET /roles: %v, %v", ok, err)
		}
		if ok, err := svc.IsAuthorized(context.Background(), user, "DELETE", "/roles"); err != nil || ok {
			t.Fatalf("DELETE /roles: %v, %v; want refused", ok, err)
		}
	})

	t.Run("a_nil_user_id_is_refused_before_anything_is_read", func(t *testing.T) {
		t.Parallel()
		users := &fakeAuthzUsers{perms: wire(user, map[string]any{"*": []any{"*"}})}
		svc := newService(t, users, engine)

		if ok, err := svc.IsAuthorized(context.Background(), uuid.Nil(), "GET", "/roles"); err == nil || ok {
			t.Fatalf("got %v, %v; want an error and a refusal", ok, err)
		}
		if users.calls != 0 {
			t.Fatal("the permission store was read for a nil user")
		}
	})

	t.Run("a_store_fault_is_a_refusal", func(t *testing.T) {
		t.Parallel()
		svc := newService(t, &fakeAuthzUsers{err: errors.New("db away")}, engine)

		if ok, err := svc.IsAuthorized(context.Background(), user, "GET", "/roles"); err == nil || ok {
			t.Fatalf("got %v, %v; want an error and a refusal", ok, err)
		}
	})

	t.Run("an_engine_error_is_a_refusal_even_with_true_beside_it", func(t *testing.T) {
		t.Parallel()
		svc := newService(t, &fakeAuthzUsers{perms: wire(user, map[string]any{"*": []any{"*"}})}, tableEngine{err: errors.New("policy broke")})

		if ok, err := svc.IsAuthorized(context.Background(), user, "GET", "/roles"); err == nil || ok {
			t.Fatalf("got %v, %v; want an error and a refusal", ok, err)
		}
	})

	t.Run("no_grants_at_all_is_a_refusal_not_an_error", func(t *testing.T) {
		t.Parallel()
		svc := newService(t, &fakeAuthzUsers{perms: nil}, engine)

		if ok, err := svc.IsAuthorized(context.Background(), user, "GET", "/roles"); err != nil || ok {
			t.Fatalf("got %v, %v; want false, nil", ok, err)
		}
	})
}
