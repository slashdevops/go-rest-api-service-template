//go:build unit

package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/respond"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driving"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

// A call's outcome on the metric is decided from its error by the status
// table (respond.IsRefusal). A handler that answered 500 whatever the error
// counted that 500 as `refused` whenever the error was one the table calls a
// 4xx. The tests below hold the sites that an error of the table can reach,
// found by reading every WriteInternalError call of the handlers against what
// its use-case returns, to the table: the status written is the table's, so
// the status and the outcome say the same thing.

// agrees fails when the status a handler wrote and the outcome its call was
// counted with disagree: a refusal is answered below 500, a failure at 500
// or above.
func agrees(t *testing.T, err error, status int) {
	t.Helper()

	if refused := respond.IsRefusal(err); refused != (status < http.StatusInternalServerError) {
		t.Errorf("%T is counted refused=%v and was answered %d", err, refused, status)
	}
}

// testTelemetry is what a handler needs to open its span and record its call.
func testTelemetry(t *testing.T) (*o11y.OpenTelemetry, *o11y.LayerMetrics) {
	t.Helper()

	ot := &o11y.OpenTelemetry{
		Traces:  o11y.NewOpenTelemetryTracer(t.Context(), &o11y.OpenTelemetryTracerConfig{Name: "test"}),
		Metrics: o11y.NewOpenTelemetryMeter(t.Context(), &o11y.OpenTelemetryMeterConfig{Name: "test"}),
	}

	metrics, err := o11y.NewLayerMetrics(ot.Metrics.Meter, "")
	if err != nil {
		t.Fatalf("NewLayerMetrics: %v", err)
	}

	return ot, metrics
}

// TestTheIDPCallbackOfACallerWhoWentAwayIsNotA500: the tab was closed while
// the code was being exchanged. The adapter does not blame the provider for
// it and returns the context's own error; the callback answered it as any
// error no arm names, a 500 counted failed, with an ERROR access line. It is
// the caller's side: 499, nothing written for nobody to read, counted
// refused.
func TestTheIDPCallbackOfACallerWhoWentAwayIsNotA500(t *testing.T) {
	t.Parallel()

	ot, metrics := testTelemetry(t)
	_, span := ot.Traces.Tracer.Start(t.Context(), "test")
	defer span.End()

	gone := fmt.Errorf("exchanging the code: %w", context.Canceled)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/idp/x/callback", nil)

	(&AuthnIDPsHandler{metrics: metrics}).writeCallbackError(rec, req, span, time.Now(), nil, gone)

	if rec.Code != respond.StatusClientClosedRequest {
		t.Fatalf("status = %d, want %d. Body: %s", rec.Code, respond.StatusClientClosedRequest, rec.Body.String())
	}

	agrees(t, gone, rec.Code)

	if rec.Body.Len() != 0 {
		t.Errorf("an answer nobody receives has a body: %s", rec.Body.String())
	}
}

// TestTheIDPCallbackDoesNotPrintTheAccountID: the account a link was started
// for was deleted before the callback. The user service's error names the
// account by its id, and the callback is public: it answers 404 in a fixed
// sentence. It was a 500 counted as refused.
func TestTheIDPCallbackDoesNotPrintTheAccountID(t *testing.T) {
	t.Parallel()

	ot, metrics := testTelemetry(t)
	_, span := ot.Traces.Tracer.Start(t.Context(), "test")
	defer span.End()

	id := uuid.NewV7()
	gone := &domain.UserNotFoundError{ID: id, Message: "User not found"}

	rec := httptest.NewRecorder()
	(&AuthnIDPsHandler{metrics: metrics}).writeCallbackError(rec, httptest.NewRequest(http.MethodGet, "/auth/idp/x/callback", nil), span, time.Now(), nil, gone)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404. Body: %s", rec.Code, rec.Body.String())
	}

	agrees(t, gone, rec.Code)

	body := rec.Body.String()
	if !strings.Contains(body, accountGoneMessage) {
		t.Errorf("the body is not the fixed sentence: %s", body)
	}

	if strings.Contains(body, id.String()) {
		t.Errorf("the body prints the account's id to an anonymous caller: %s", body)
	}
}

// theLimitIsFull is the error the resources-limits repository returns when
// the system's user limit is full, with the numbers it puts in its message.
func theLimitIsFull() *domain.ResourcesLimitsHardLimitReachedError {
	return &domain.ResourcesLimitsHardLimitReachedError{
		Message:  "hard limit reached: scope type system, scope ID 0199, resource type users. soft limit 90, hard limit 100, current usage 100",
		Resource: "users", Limit: 100, Current: 100,
	}
}

// noLimitIn fails when a body tells an anonymous caller about the limit.
func noLimitIn(t *testing.T, body string) {
	t.Helper()

	if !strings.Contains(body, registrationClosedMessage) {
		t.Errorf("the body does not say registration is closed: %s", body)
	}

	for _, leak := range []string{"hard limit", "current usage", "scope", "0199"} {
		if strings.Contains(body, leak) {
			t.Errorf("the body tells an anonymous caller about the limit (%q): %s", leak, body)
		}
	}
}

// TestTheIDPCallbackDoesNotDiscloseTheUserLimit: a first sign-in creates the
// account, and the system's user limit can be reached. The route is public,
// and the limit error's own message names the scope, both limits and the
// current usage. It hears what the public registration route says.
func TestTheIDPCallbackDoesNotDiscloseTheUserLimit(t *testing.T) {
	t.Parallel()

	ot, metrics := testTelemetry(t)
	_, span := ot.Traces.Tracer.Start(t.Context(), "test")
	defer span.End()

	full := theLimitIsFull()

	rec := httptest.NewRecorder()
	(&AuthnIDPsHandler{metrics: metrics}).writeCallbackError(rec, httptest.NewRequest(http.MethodGet, "/auth/idp/x/callback", nil), span, time.Now(), nil, full)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409. Body: %s", rec.Code, rec.Body.String())
	}

	agrees(t, full, rec.Code)
	noLimitIn(t, rec.Body.String())
}

// registering is the one method of the port the registration route calls.
type registering struct {
	driving.Authn

	err error
}

func (r registering) RegisterUser(context.Context, *domain.RegisterUserInput) error { return r.err }

// TestRegistrationAnswersThroughTheTable: the system's user limit is full. It
// was a 500 counted as refused; it is the 409 the callback answers, in the
// same fixed sentence.
func TestRegistrationAnswersThroughTheTable(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		err  error
		want int
	}{
		{name: "the_user_limit_is_full", err: theLimitIsFull(), want: http.StatusConflict},
		// The optional id is a user's: its conflict, never the address's.
		{name: "the_id_is_taken", err: &domain.UserAlreadyExistsError{ID: uuid.NewV7()}, want: http.StatusConflict},
		{name: "a_fault", err: errors.New("the database is away"), want: http.StatusInternalServerError},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ot, metrics := testTelemetry(t)
			h := &AuthnHandler{service: registering{err: tt.err}, ot: ot, metrics: metrics}

			body := `{"first_name":"Ada","last_name":"Lovelace","email":"ada@example.com","password":"Analytical-Engine-1843"}`
			req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			rec := httptest.NewRecorder()
			h.registerUser(rec, req)

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d. Body: %s", rec.Code, tt.want, rec.Body.String())
			}

			agrees(t, tt.err, rec.Code)

			if tt.name == "the_user_limit_is_full" {
				noLimitIn(t, rec.Body.String())
			}

			if strings.Contains(rec.Body.String(), "ada@example.com") {
				t.Errorf("the address is in the body: %s", rec.Body.String())
			}

			if strings.Contains(rec.Body.String(), "database is away") {
				t.Errorf("the cause is in the body: %s", rec.Body.String())
			}
		})
	}
}

// unlinking is the one method of the port the unlink route calls.
type unlinking struct {
	driving.AuthnIDPs

	err error
}

func (u unlinking) UnlinkIdentity(context.Context, uuid.UUID, uuid.UUID) error { return u.err }

func TestUnlinkingAnIdentityAnswersThroughTheTable(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		err  error
		want int
	}{
		// The account was deleted between the middleware's check of its
		// standing and the unlink.
		{name: "a_user_that_is_gone", err: &domain.UserNotFoundError{Message: "User not found"}, want: http.StatusNotFound},
		{name: "a_fault", err: errors.New("the database is away"), want: http.StatusInternalServerError},
		// The arm that was there stays as it was.
		{name: "no_identity_at_that_provider", err: &domain.UserIdentityNotFoundError{Message: "none"}, want: http.StatusNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ot, metrics := testTelemetry(t)
			h := &AuthnIDPsHandler{service: unlinking{err: tt.err}, ot: ot, metrics: metrics}

			req := httptest.NewRequest(http.MethodDelete, "/me/identities/x", nil)
			req.SetPathValue("idp_id", uuid.NewV7().String())

			rec := httptest.NewRecorder()
			h.unlinkIdentity(rec, withCaller(req, uuid.NewV7()))

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d. Body: %s", rec.Code, tt.want, rec.Body.String())
			}

			agrees(t, tt.err, rec.Code)
		})
	}
}

// updatingAPolicy is the one method of the port the policy update calls.
type updatingAPolicy struct {
	driving.Policies

	err error
}

func (u updatingAPolicy) UpdateByID(context.Context, *domain.UpdatePolicyInput) error { return u.err }

// TestAPolicyUpdateAnswersThroughTheTable: an update that changes the grant
// to one no endpoint matches is the ResourceNotFoundError the create answers
// 404. The update answered it 500, counted as refused.
func TestAPolicyUpdateAnswersThroughTheTable(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		err  error
		want int
	}{
		{name: "a_grant_no_endpoint_matches", err: &domain.ResourceNotFoundError{Message: "no endpoint matches GET /nowhere"}, want: http.StatusNotFound},
		{name: "a_fault", err: errors.New("the database is away"), want: http.StatusInternalServerError},
		// The arm that was there stays as it was.
		{name: "a_policy_that_is_not_there", err: &domain.PolicyNotFoundError{Message: "Policy not found"}, want: http.StatusNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ot, metrics := testTelemetry(t)
			h := &PoliciesHandler{service: updatingAPolicy{err: tt.err}, ot: ot, metrics: metrics}

			req := httptest.NewRequest(http.MethodPut, "/policies/x", strings.NewReader(`{"allowed_resource":"/nowhere"}`))
			req.Header.Set("Content-Type", "application/json")
			req.SetPathValue("policy_id", uuid.NewV7().String())

			rec := httptest.NewRecorder()
			h.updateByID(rec, withCaller(req, uuid.NewV7()))

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d. Body: %s", rec.Code, tt.want, rec.Body.String())
			}

			agrees(t, tt.err, rec.Code)
		})
	}
}

// linkingPolicies is the one method of the port the role's policy link calls.
type linkingPolicies struct {
	driving.Roles

	err error
}

func (l linkingPolicies) LinkPolicies(context.Context, *domain.LinkPoliciesToRoleInput) error {
	return l.err
}

// TestLinkingPoliciesToARoleAnswersThroughTheTable: the grant guard reads each
// policy to learn what it grants, so a policy that is not there is its
// PolicyNotFoundError. The route answered it 500, counted as refused.
func TestLinkingPoliciesToARoleAnswersThroughTheTable(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		err  error
		want int
	}{
		{name: "a_policy_that_is_not_there", err: &domain.PolicyNotFoundError{Message: "Policy not found"}, want: http.StatusNotFound},
		{name: "a_fault", err: errors.New("the database is away"), want: http.StatusInternalServerError},
		// The arm that was there stays as it was.
		{name: "a_role_that_is_not_there", err: &domain.RoleNotFoundError{Message: "Role not found"}, want: http.StatusNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ot, metrics := testTelemetry(t)
			h := &RolesHandler{service: linkingPolicies{err: tt.err}, ot: ot, metrics: metrics}

			body := `{"policy_ids":["` + uuid.NewV7().String() + `"]}`
			req := httptest.NewRequest(http.MethodPost, "/roles/x/policies", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.SetPathValue("role_id", uuid.NewV7().String())

			rec := httptest.NewRecorder()
			h.linkPolicies(rec, withCaller(req, uuid.NewV7()))

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d. Body: %s", rec.Code, tt.want, rec.Body.String())
			}

			agrees(t, tt.err, rec.Code)
		})
	}
}

// abandoning records what the callback asks of the port when the provider
// reports an error.
type abandoning struct {
	driving.AuthnIDPs

	states []string
	err    error
}

func (a *abandoning) AbandonCallback(_ context.Context, _ uuid.UUID, state string) error {
	a.states = append(a.states, state)

	return a.err
}

// TestACallbackTheProviderDidNotCompleteIsRefusedAndItsStateSpent: the
// provider sends the person back with `error=` and no code. That is a
// refusal on the caller's side: 401, and RECORDED as a refusal, which is what
// the call's outcome and the access line are decided from. And it was the
// state's one callback, so the state is spent: it used to stay good for a
// callback with a code for the rest of its fifteen minutes.
func TestACallbackTheProviderDidNotCompleteIsRefusedAndItsStateSpent(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name      string
		query     string
		spendErr  error
		wantSpent []string
		want      int
		outcome   string
	}{
		{name: "cancelled", query: "error=access_denied&state=the-state", wantSpent: []string{"the-state"}, want: http.StatusUnauthorized, outcome: "refused"},
		{name: "with_a_description", query: "error=access_denied&error_description=SECRET+DETAIL&state=the-state", wantSpent: []string{"the-state"}, want: http.StatusUnauthorized, outcome: "refused"},
		{name: "a_state_that_is_refused", query: "error=access_denied&state=forged", spendErr: &domain.InvalidJWTError{Message: "the state is not valid"}, wantSpent: []string{"forged"}, want: http.StatusUnauthorized, outcome: "refused"},
		{name: "no_state_at_all", query: "error=access_denied", want: http.StatusUnauthorized, outcome: "refused"},
		// The store that spends states is away: not a refused sign-in.
		{name: "a_state_that_cannot_be_spent_because_the_store_is_away", query: "error=access_denied&state=the-state", spendErr: errors.New("the database is away"), wantSpent: []string{"the-state"}, want: http.StatusInternalServerError, outcome: "failed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ot, metrics := testTelemetry(t)
			port := &abandoning{err: tt.spendErr}
			h := &AuthnIDPsHandler{service: port, ot: ot, metrics: metrics}

			req := httptest.NewRequest(http.MethodGet, "/auth/idp/x/callback?"+tt.query, nil)
			req.SetPathValue("idp_id", uuid.NewV7().String())
			req = req.WithContext(o11y.WithFailure(req.Context()))

			rec := httptest.NewRecorder()
			h.callback(rec, req)

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d. Body: %s", rec.Code, tt.want, rec.Body.String())
			}

			// What the handler recorded, not what this test thinks it is.
			recorded, ok := o11y.TakeFailure(req.Context())
			if !ok {
				t.Fatal("nothing was recorded for a request that was not answered 2xx")
			}

			got := "failed"
			if respond.IsRefusal(recorded.Err) {
				got = "refused"
			}

			if got != tt.outcome {
				t.Errorf("recorded %T, counted %s; want %s", recorded.Err, got, tt.outcome)
			}

			agrees(t, recorded.Err, rec.Code)

			if !slices.Equal(port.states, tt.wantSpent) {
				t.Errorf("spent %v, want %v", port.states, tt.wantSpent)
			}

			if body := rec.Body.String(); strings.Contains(body, "SECRET") || strings.Contains(body, "access_denied") || strings.Contains(body, "database") {
				t.Errorf("the provider's words or the cause are in the body: %s", body)
			}

			// What the provider said is the refusal's detail, on the
			// request's one line: it was an INFO line of its own.
			if strings.Contains(tt.query, "SECRET") && !strings.Contains(recorded.Detail, "access_denied: SECRET DETAIL") {
				t.Errorf("the provider's words are not the refusal's detail: %q", recorded.Detail)
			}
		})
	}
}

// TestTheCallbacks503SaysOneThing: a provider that does not answer and one
// whose answer cannot be used are told apart in the log and on the metric.
// The public body says one sentence for both, and not that it is this
// service's own registration the provider rejects.
func TestTheCallbacks503SaysOneThing(t *testing.T) {
	t.Parallel()

	ot, metrics := testTelemetry(t)
	_, span := ot.Traces.Tracer.Start(t.Context(), "test")
	defer span.End()

	for _, err := range []error{
		&domain.IDPUnreachableError{Message: "the identity provider is not reachable"},
		&domain.InvalidAuthnServiceError{Message: "the identity provider did not accept this service's request"},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/auth/idp/x/callback", nil)

		(&AuthnIDPsHandler{metrics: metrics}).writeCallbackError(rec, req, span, time.Now(), nil, err)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%T: status = %d, want 503", err, rec.Code)
		}

		agrees(t, err, rec.Code)

		body := rec.Body.String()
		if !strings.Contains(body, signInUnavailableMessage) || strings.Contains(body, "this service's request") || strings.Contains(body, "not reachable") {
			t.Errorf("%T: the body is not the one sentence: %s", err, body)
		}
	}
}

// The two that were one error answered 401: a sign-in refused at the
// provider is the caller's side.
func TestASignInRefusedAtTheProviderIsA401(t *testing.T) {
	t.Parallel()

	ot, metrics := testTelemetry(t)
	_, span := ot.Traces.Tracer.Start(t.Context(), "test")
	defer span.End()

	refused := &domain.IDPSignInRefusedError{Message: "the identity provider did not accept this sign-in"}

	rec := httptest.NewRecorder()
	(&AuthnIDPsHandler{metrics: metrics}).writeCallbackError(rec, httptest.NewRequest(http.MethodGet, "/auth/idp/x/callback", nil), span, time.Now(), nil, refused)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401. Body: %s", rec.Code, rec.Body.String())
	}

	agrees(t, refused, rec.Code)
}

// TestAProviderThatCannotBeUsedIsA503AtTheStart: a provider whose
// discovery cannot be fetched, and a provider row the service cannot sign in
// with, are the service's side, answered on the public start routes in the
// callback's one sentence. Which it was, and the provider's own words, is the
// failure's detail, for the access line and never the body.
func TestAProviderThatCannotBeUsedIsA503AtTheStart(t *testing.T) {
	t.Parallel()

	for name, err := range map[string]error{
		"unreachable":    &domain.IDPUnreachableError{Message: "the identity provider is not reachable", Detail: "discovery: dial tcp: refused"},
		"not_configured": &domain.InvalidAuthnServiceError{Message: domain.IDPNotConfiguredMessage, Detail: "no issuer URL"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ot, metrics := testTelemetry(t)
			_, span := ot.Traces.Tracer.Start(t.Context(), "test")
			defer span.End()

			rec := httptest.NewRecorder()
			(&AuthnIDPsHandler{metrics: metrics}).writeStartError(rec, httptest.NewRequest(http.MethodGet, "/auth/idp/x/login", nil), span, time.Now(), nil, err)

			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503. Body: %s", rec.Code, rec.Body.String())
			}

			agrees(t, err, rec.Code)

			body := rec.Body.String()
			if !strings.Contains(body, signInUnavailableMessage) {
				t.Errorf("the body is not the callback's one sentence: %s", body)
			}

			for _, leak := range []string{"refused", "issuer", "configured", "reachable"} {
				if strings.Contains(body, leak) {
					t.Errorf("the body carries the failure's detail (%q): %s", leak, body)
				}
			}
		})
	}
}
