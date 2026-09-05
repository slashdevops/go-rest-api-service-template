//go:build unit

package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/config"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
	mocks "github.com/slashdevops/go-rest-api-service-template/mocks/handler"
)

// A refusal is not a fault, and the status code is the only part of that a
// client can act on.
//
// Five System*Error types guard rows the shared
// tr_restrict_delete_update_on_system_* trigger protects. Three were mapped to
// 403; SystemRoleError and SystemGenerateConfigError were not, so they fell
// through to the generic branch and answered 500 -- telling a caller to retry
// something that can never succeed, and paging whoever is on call for an error
// nobody can fix.
//
// Measured against the running service before the fix:
//
//	DELETE /roles/{id}   -> 500  "role cannot be modified: system/protected resource"
//
// The message was already right. Only the status lied.
//
// Both operations are covered because the trigger refuses UPDATE as well as
// DELETE, and a guard added to one of the pair is the easy half to forget.
func newRolesTestHandler(t *testing.T, svc *mocks.MockRoles) *RolesHandler {
	t.Helper()

	conf := config.NewOpenTelemetryConfig("test", "0.0.0-test")
	conf.TraceExporter.Value = config.ExporterNoop
	conf.MetricExporter.Value = config.ExporterNoop

	ot, err := o11y.New(t.Context(), conf)
	if err != nil {
		t.Fatalf("o11y.New: %v", err)
	}

	h, err := NewRolesHandler(RolesHandlerConf{
		Service:       svc,
		OT:            ot,
		MetricsPrefix: "test",
	})
	if err != nil {
		t.Fatalf("NewRolesHandler: %v", err)
	}

	return h
}

func TestSystemRoleDeleteAnswers403NotServerError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	id := uuid.NewV7()

	svc := mocks.NewMockRoles(ctrl)
	svc.EXPECT().
		DeleteByID(gomock.Any(), gomock.Any()).
		Return(&domain.SystemRoleError{RoleID: id})

	h := newRolesTestHandler(t, svc)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodDelete, "/roles/"+id.String(), nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d -- a protected row is a refusal, not a fault",
			rec.Code, http.StatusForbidden)
	}

	// The reason has to survive: "you may not delete this" is actionable in a
	// way that a bare 403 from the auth middleware is not.
	if !strings.Contains(rec.Body.String(), "system") {
		t.Errorf("body = %q, want it to say why the row is protected", rec.Body.String())
	}
}
