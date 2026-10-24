//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driving"
)

// idpAccountsStub answers a provider's accounts and records which provider
// was asked. Any other method panics: the embedded port is nil.
type idpAccountsStub struct {
	driving.Users
	err   error
	out   *domain.SelectIDPAccountsOutput
	asked uuid.UUID
}

func (s *idpAccountsStub) ListByIDPID(_ context.Context, idpID uuid.UUID, _ *domain.ListUsersInput) (*domain.SelectIDPAccountsOutput, error) {
	s.asked = idpID

	return s.out, s.err
}

// A provider's accounts are answered with when each was linked and whether
// the provider is its only way in, for the provider in the path; a provider
// that is not there is the use-case's 404, which the lists' writeListError
// would have answered 500.
func TestAProvidersAccountsAreAnsweredForTheProviderInThePath(t *testing.T) {
	t.Parallel()

	idp, account := uuid.NewV7(), uuid.NewV7()
	linked := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)

	serve := func(t *testing.T, stub *idpAccountsStub) *httptest.ResponseRecorder {
		t.Helper()

		ot, _ := testTelemetry(t)

		h, err := NewUsersHandler(UsersHandlerConf{Service: stub, Recoverer: recovererNeverAsked{}, OT: ot})
		if err != nil {
			t.Fatal(err)
		}

		req := httptest.NewRequest(http.MethodGet, "/auth/idps/"+idp.String()+"/identities", nil)
		req.SetPathValue("idp_id", idp.String())

		rec := httptest.NewRecorder()
		h.listByIDPID(rec, req)

		return rec
	}

	t.Run("the_accounts", func(t *testing.T) {
		t.Parallel()

		stub := &idpAccountsStub{out: &domain.SelectIDPAccountsOutput{Items: []domain.IDPAccount{
			{User: domain.User{ID: account, Email: "someone@example.com"}, LinkedAt: linked, OnlyWayIn: true},
		}}}

		rec := serve(t, stub)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
		}

		if stub.asked != idp {
			t.Errorf("asked about %s, want the path's provider %s", stub.asked, idp)
		}

		var page payload.ListIDPAccountsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}

		if len(page.Items) != 1 || page.Items[0].ID != account || !page.Items[0].OnlyWayIn || !page.Items[0].LinkedAt.Equal(linked) {
			t.Errorf("answered %+v, want the account, linked at %s, its only way in this provider", page.Items, linked)
		}
	})

	t.Run("a_provider_that_is_not_there", func(t *testing.T) {
		t.Parallel()

		rec := serve(t, &idpAccountsStub{err: &domain.IDPNotFoundError{ID: idp}})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404. Body: %s", rec.Code, rec.Body.String())
		}
	})
}
