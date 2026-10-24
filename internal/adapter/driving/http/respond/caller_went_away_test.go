package respond

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

// A request whose caller went away -- a tab closed while a sign-in's code was
// being exchanged -- is answered 499 with no body, and counted a refusal: the
// caller's side. It was a 500, counted failed, with an ERROR
// access line, for something nobody did wrong and nobody receives.
//
// Only the request's own cancellation is: a context.Canceled that reaches a
// handler while its caller is still there is answered 500, and a deadline is
// something slow, a failure.
func TestACallerWhoWentAwayIsAnsweredNothing(t *testing.T) {
	t.Parallel()

	gone := fmt.Errorf("exchanging the code: %w", context.Canceled)

	request := func(cancelled bool) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/auth/idp/callback", nil)
		if cancelled {
			ctx, cancel := context.WithCancel(req.Context())
			cancel()

			req = req.WithContext(ctx)
		}

		return req
	}

	for _, tt := range []struct {
		err       error
		name      string
		status    int
		cancelled bool
		refused   bool
	}{
		{name: "the_caller_went_away", err: gone, cancelled: true, status: StatusClientClosedRequest, refused: true},
		{name: "through_write_error", err: gone, cancelled: true, status: StatusClientClosedRequest, refused: true},
		{name: "cancelled_while_the_caller_is_there", err: gone, cancelled: false, status: http.StatusInternalServerError, refused: true},
		{name: "a_deadline", err: fmt.Errorf("reading: %w", context.DeadlineExceeded), cancelled: true, status: http.StatusInternalServerError, refused: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			if tt.name == "through_write_error" {
				WriteError(rec, request(tt.cancelled), tt.err)
			} else {
				WriteInternalError(rec, request(tt.cancelled), tt.err)
			}

			if rec.Code != tt.status {
				t.Errorf("status = %d, want %d", rec.Code, tt.status)
			}

			if tt.status == StatusClientClosedRequest && rec.Body.Len() != 0 {
				t.Errorf("a request nobody receives was answered a body: %q", rec.Body.String())
			}

			if got := IsRefusal(tt.err); got != tt.refused {
				t.Errorf("IsRefusal = %v, want %v", got, tt.refused)
			}
		})
	}
}

// The failure reaches the access line: a request whose handler did not
// record it still says what ended it (error_type context.Canceled), because
// WriteCallerGone hands it to the holder the Tracing middleware installs.
func TestACallerWhoWentAwayIsOnTheAccessLine(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(o11y.WithFailure(t.Context()))
	cancel()

	gone := fmt.Errorf("reading the grants: %w", context.Canceled)

	if !WriteCallerGone(httptest.NewRecorder(), httptest.NewRequestWithContext(ctx, http.MethodGet, "/projects", nil), gone) {
		t.Fatal("a request its caller abandoned was not answered as one")
	}

	failure, held := o11y.TakeFailure(ctx)
	if !held || failure.Type != "context.Canceled" {
		t.Errorf("the access line would say %+v (held %v), want the cancellation", failure, held)
	}
}
