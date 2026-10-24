//go:build unit

package middleware

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"uuid"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/respond"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

// logged is one record: its level, its message and its attributes as text.
type logged struct {
	attrs   map[string]string
	message string
	level   slog.Level
}

// recordingHandler keeps every record at any level, DEBUG included.
type recordingHandler struct {
	records *[]logged
	mu      *sync.Mutex
}

func (recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h recordingHandler) Handle(_ context.Context, r slog.Record) error {
	entry := logged{level: r.Level, message: r.Message, attrs: map[string]string{}}
	r.Attrs(func(a slog.Attr) bool {
		entry.attrs[a.Key] = a.Value.String()

		return true
	})

	h.mu.Lock()
	defer h.mu.Unlock()

	*h.records = append(*h.records, entry)

	return nil
}

func (h recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h recordingHandler) WithGroup(string) slog.Handler      { return h }

// recordLogs replaces the default logger for the test and returns what was
// written.
func recordLogs(t *testing.T) *[]logged {
	t.Helper()

	records := &[]logged{}

	previous := slog.Default()
	slog.SetDefault(slog.New(recordingHandler{records: records, mu: &sync.Mutex{}}))
	t.Cleanup(func() { slog.SetDefault(previous) })

	return records
}

// through is a request that passes through three layers, as a real one does:
// the repository fails, the use-case returns its error, the handler answers.
// Each records the failure with o11y.RecordError, as all 1 748 call sites do.
func through(tracer trace.Tracer, failure error, answer func(http.ResponseWriter, *http.Request, error)) http.Handler {
	layer := func(ctx context.Context, name string, inner func(context.Context) error) error {
		start := time.Now()
		ctx, span, attrs := o11y.SetupTrace(ctx, tracer, o11y.Metadata{Layer: name, Domain: "Things"}, "List")
		defer span.End()

		if err := inner(ctx); err != nil {
			return o11y.RecordError(ctx, span, start, err, nil, attrs)
		}

		return nil
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := layer(r.Context(), o11y.LayerHandler, func(ctx context.Context) error {
			return layer(ctx, o11y.LayerUsecase, func(ctx context.Context) error {
				return layer(ctx, o11y.LayerRepository, func(context.Context) error { return failure })
			})
		})

		answer(w, r, err)
	})
}

func serve(t *testing.T, tracer trace.Tracer, handler http.Handler) *httptest.ResponseRecorder {
	t.Helper()

	mux := http.NewServeMux()
	mux.Handle("GET /things", handler)

	rec := httptest.NewRecorder()
	chain(t, tracer, mux).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/things", nil))

	return rec
}

func atOrAbove(records []logged, level slog.Level) []logged {
	var out []logged
	for _, r := range records {
		if r.level >= level {
			out = append(out, r)
		}
	}

	return out
}

// TestARefusedRequestIsNotAnError: a request the service refuses on purpose
// -- a bad field, here -- writes nothing at ERROR and nothing at WARN. Its
// access line is INFO, as it always was, and now says what was refused; the
// detail is one DEBUG line.
//
// Each layer used to write its own ERROR line for the refusal it returned:
// three lines, at the level that should mean "the service is broken", for a
// request answered 400. 98% of the ERROR lines of an integration run were
// these, measured in svc-qu3ry-core, where this was built.
func TestARefusedRequestIsNotAnError(t *testing.T) {
	tracer, _ := recorder(t)
	records := recordLogs(t)

	refusal := &domain.InvalidInputError{Field: "name", Message: "name is required"}

	rec := serve(t, tracer, through(tracer, refusal, func(w http.ResponseWriter, r *http.Request, err error) {
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, err.Error())
	}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}

	if loud := atOrAbove(*records, slog.LevelWarn); len(loud) != 0 {
		t.Fatalf("a refused request wrote %d line(s) at WARN or above: %+v", len(loud), loud)
	}

	info := atOrAbove(*records, slog.LevelInfo)
	if len(info) != 1 || info[0].message != "request" {
		t.Fatalf("want exactly one INFO line, the access line; got %+v", info)
	}

	if got := info[0].attrs["error_type"]; got != "*domain.InvalidInputError" {
		t.Errorf("the access line says error_type=%q, want what was refused", got)
	}

	if _, has := info[0].attrs["error"]; has {
		t.Error("the access line of a refusal carries the error text; that is the DEBUG line's")
	}

	var detail []logged
	for _, r := range *records {
		if r.level == slog.LevelDebug && r.message == "request_refused" {
			detail = append(detail, r)
		}
	}

	if len(detail) != 1 {
		t.Fatalf("want one DEBUG request_refused line, got %d", len(detail))
	}

	// Where it arose: the deepest layer, which recorded first.
	if got := detail[0].attrs[o11y.AttrLayer]; got != o11y.LayerRepository {
		t.Errorf("the detail names layer %q, want the repository, where the error arose", got)
	}

	if !strings.Contains(detail[0].attrs["error"], "name is required") {
		t.Errorf("the detail's error is %q", detail[0].attrs["error"])
	}

	if detail[0].attrs["status"] != "400" || detail[0].attrs["func"] == "" {
		t.Errorf("the detail lacks the status or the function: %+v", detail[0].attrs)
	}
}

// TestAFailedRequestIsOneErrorLine: a request the service could not answer
// writes one ERROR line, the access line, with the cause and where it arose.
// The access line used to be INFO for every status, so the failed requests
// were at INFO and the ERROR level held refusals.
func TestAFailedRequestIsOneErrorLine(t *testing.T) {
	tracer, _ := recorder(t)
	records := recordLogs(t)

	fault := fmt.Errorf("selecting things: %w", errors.New("connection refused"))

	rec := serve(t, tracer, through(tracer, fault, func(w http.ResponseWriter, r *http.Request, err error) {
		respond.WriteInternalError(w, r, err)
	}))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}

	if strings.Contains(rec.Body.String(), "connection refused") {
		t.Fatal("the cause reached the response body")
	}

	loud := atOrAbove(*records, slog.LevelInfo)
	if len(loud) != 1 {
		t.Fatalf("want exactly one line at INFO or above for a failed request, got %d: %+v", len(loud), loud)
	}

	line := loud[0]
	if line.level != slog.LevelError || line.message != "request" {
		t.Fatalf("the line is %s %q, want ERROR \"request\"", line.level, line.message)
	}

	if line.attrs["status"] != "500" {
		t.Errorf("status = %q", line.attrs["status"])
	}

	if !strings.Contains(line.attrs["error"], "connection refused") {
		t.Errorf("the ERROR line does not carry the cause: %q", line.attrs["error"])
	}

	if line.attrs["error_type"] != "*errors.errorString" {
		t.Errorf("error_type = %q, want the type at the bottom of the chain", line.attrs["error_type"])
	}

	if line.attrs[o11y.AttrLayer] != o11y.LayerRepository || line.attrs["func"] == "" || line.attrs["line"] == "" {
		t.Errorf("the ERROR line does not say where the error arose: %+v", line.attrs)
	}

	for _, r := range *records {
		if r.message == "operation_failed" || r.message == "internal server error" {
			t.Errorf("a second line was written for the same failure: %s %q", r.level, r.message)
		}
	}
}

// TestASuccessfulRequestSaysNothingAboutFailure: the access line of a request
// that worked is what it always was.
func TestASuccessfulRequestSaysNothingAboutFailure(t *testing.T) {
	tracer, _ := recorder(t)
	records := recordLogs(t)

	serve(t, tracer, through(tracer, nil, func(w http.ResponseWriter, _ *http.Request, _ error) {
		w.WriteHeader(http.StatusOK)
	}))

	if len(*records) != 1 || (*records)[0].level != slog.LevelInfo || (*records)[0].message != "request" {
		t.Fatalf("want only the INFO access line, got %+v", *records)
	}

	if _, has := (*records)[0].attrs["error_type"]; has {
		t.Error("a request that worked carries an error_type")
	}
}

// TestAFailureTheHandlerDidNotAnswerWithIsDebug: a handler may answer 200 on
// purpose after something failed -- a re-verification that must not say
// whether the account exists. The request succeeded as far as the caller is
// told, so it is INFO; what failed is a DEBUG line, not lost.
func TestAFailureTheHandlerDidNotAnswerWithIsDebug(t *testing.T) {
	tracer, _ := recorder(t)
	records := recordLogs(t)

	serve(t, tracer, through(tracer, &domain.UserNotFoundError{}, func(w http.ResponseWriter, _ *http.Request, _ error) {
		w.WriteHeader(http.StatusOK)
	}))

	if loud := atOrAbove(*records, slog.LevelWarn); len(loud) != 0 {
		t.Fatalf("wrote at WARN or above: %+v", loud)
	}

	var found bool
	for _, r := range *records {
		if r.level == slog.LevelDebug && r.message == "request_failure_not_answered" && r.attrs["status"] == "200" {
			found = true
		}
	}

	if !found {
		t.Fatalf("the failure was dropped: %+v", *records)
	}

	// The access line of a request answered 200 names no error: error_type
	// is the type of the error a request was answered with.
	for _, r := range *records {
		if r.message == "request" {
			if _, has := r.attrs["error_type"]; has {
				t.Errorf("a request answered 200 carries an error_type: %+v", r.attrs)
			}
		}
	}
}

// TestTheLineNamesTheFailureTheRequestWasAnsweredFor: the first sign-in
// through an identity provider. The identity lookup says "not found", which
// the use-case recovers from; the insert that follows fails and the request
// is answered 500. The one ERROR line names the insert. It used to name the
// lookup, the first failure recorded, and the insert was logged nowhere.
func TestTheLineNamesTheFailureTheRequestWasAnsweredFor(t *testing.T) {
	tracer, _ := recorder(t)
	records := recordLogs(t)

	record := func(ctx context.Context, layer, action string, err error) error {
		start := time.Now()
		ctx, span, attrs := o11y.SetupTrace(ctx, tracer, o11y.Metadata{Layer: layer, Domain: "Identities"}, action)
		defer span.End()

		return o11y.RecordError(ctx, span, start, err, nil, attrs)
	}

	serve(t, tracer, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Recorded, and recovered from.
		_ = record(ctx, o11y.LayerRepository, "SelectBySubject", &domain.UserNotFoundError{Message: "identity"})

		// The fault.
		err := record(ctx, o11y.LayerRepository, "Link", errors.New("insert: connection refused"))
		err = record(ctx, o11y.LayerUsecase, "SignIn", err)

		respond.WriteInternalError(w, r, err)
	}))

	loud := atOrAbove(*records, slog.LevelInfo)
	if len(loud) != 1 || loud[0].level != slog.LevelError || loud[0].message != "request" {
		t.Fatalf("want one ERROR access line, got %+v", loud)
	}

	line := loud[0]
	if !strings.Contains(line.attrs["error"], "insert: connection refused") {
		t.Errorf("the line names %q, not the failure the request was answered for", line.attrs["error"])
	}

	if line.attrs["error_type"] != "*errors.errorString" || line.attrs[o11y.AttrAction] != "Link" {
		t.Errorf("error_type %q at action %q, want the insert's", line.attrs["error_type"], line.attrs[o11y.AttrAction])
	}
}

// TestAPanickingRequestStillHasItsLine: a panic leaves Logging before the
// access line is written. The request had no line at all, and whatever the
// layers had recorded went with it. It gets its line, as a 500 with the
// panic as the cause, and the panic goes on up to Recovery.
func TestAPanickingRequestStillHasItsLine(t *testing.T) {
	tracer, _ := recorder(t)
	records := recordLogs(t)

	mux := http.NewServeMux()
	mux.Handle("GET /things", through(tracer, &domain.UserNotFoundError{Message: "thing"}, func(http.ResponseWriter, *http.Request, error) {
		panic("index out of range")
	}))

	rec := httptest.NewRecorder()
	Recovery(chain(t, tracer, mux)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/things", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d: Recovery did not get the panic", rec.Code)
	}

	var lines []logged
	for _, r := range *records {
		if r.message == "request" {
			lines = append(lines, r)
		}
	}

	if len(lines) != 1 || lines[0].level != slog.LevelError {
		t.Fatalf("want one ERROR access line for the panicking request, got %+v", lines)
	}

	if lines[0].attrs["status"] != "500" || !strings.Contains(lines[0].attrs["error"], "panic: index out of range") {
		t.Errorf("the line does not say what happened: %+v", lines[0].attrs)
	}

	if lines[0].attrs["route"] != "GET /things" {
		t.Errorf("route = %q", lines[0].attrs["route"])
	}

	// Two ERROR lines for a panic, and no third: the access line and
	// Recovery's own. Recovery used to answer through WriteInternalError,
	// which has no holder up there and logged the panic once more.
	var loud []string
	for _, r := range atOrAbove(*records, slog.LevelError) {
		loud = append(loud, r.message)
	}

	slices.Sort(loud)

	if !slices.Equal(loud, []string{"panic recovered", "request"}) {
		t.Errorf("ERROR lines for one panic: %q, want the access line and `panic recovered`", loud)
	}

	if !strings.Contains(rec.Body.String(), respond.InternalServerErrorMessage) {
		t.Errorf("the 500 body is %q", rec.Body.String())
	}
}

// TestWorkThatOutlivesTheRequestLogsItsOwnFailure: the cache refreshes a
// stale entry after the response, on a copy of the request's context that
// still carries the holder. Its failure is not held for a line that was
// already written: the layer logs it.
func TestWorkThatOutlivesTheRequestLogsItsOwnFailure(t *testing.T) {
	tracer, _ := recorder(t)
	records := recordLogs(t)

	var detached context.Context

	serve(t, tracer, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		detached = context.WithoutCancel(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	before := len(*records)

	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(detached, tracer, o11y.Metadata{Layer: o11y.LayerRepository, Domain: "Things"}, "SelectByID")
	_ = o11y.RecordError(ctx, span, start, errors.New("the refresh could not reach the database"), nil, attrs)
	span.End()

	after := (*records)[before:]
	if len(after) != 1 || after[0].level != slog.LevelError || after[0].message != "operation_failed" {
		t.Fatalf("want the layer's own ERROR line, got %+v", after)
	}
}

// failingStandings is a users use-case whose read of an account fails, and
// records the failure as the use-case does.
type failingStandings struct {
	tracer trace.Tracer
	err    error
}

func (f failingStandings) Standing(ctx context.Context, _ uuid.UUID) (domain.AccountStanding, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, f.tracer, o11y.Metadata{Layer: o11y.LayerUsecase, Domain: "Users"}, "Standing")
	defer span.End()

	return domain.AccountUnknown, o11y.RecordError(ctx, span, start, f.err, nil, attrs)
}

// fixedStanding answers one standing.
type fixedStanding domain.AccountStanding

func (f fixedStanding) Standing(context.Context, uuid.UUID) (domain.AccountStanding, error) {
	return domain.AccountStanding(f), nil
}

// TestTheAccountCheckWritesOneLineAndNoneForARefusal: RequireActiveAccount is
// on every authenticated request, so what it writes it writes for all of
// them. A read that fails is the one ERROR access line, with the cause, and
// no second line from the middleware. A disabled or deleted account is a
// refused request: no line at WARN or above at all.
func TestTheAccountCheckWritesOneLineAndNoneForARefusal(t *testing.T) {
	claims := map[string]any{"sub": "01a1028f-352c-7c1c-ba58-382d24a144a0"}

	serve := func(t *testing.T, tracer trace.Tracer, accounts AccountStandings) *httptest.ResponseRecorder {
		t.Helper()

		mux := http.NewServeMux()
		mux.Handle("GET /things", RequireActiveAccount(accounts)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Error("the request went through")
		})))

		// The claims are put in the context below Tracing, where the
		// authentication middleware puts them.
		withClaims := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mux.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), JwtClaims, claims)))
		})

		rec := httptest.NewRecorder()
		chain(t, tracer, withClaims).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/things", nil))

		return rec
	}

	t.Run("a_failed_read_is_one_error_line", func(t *testing.T) {
		tracer, _ := recorder(t)
		records := recordLogs(t)

		rec := serve(t, tracer, failingStandings{tracer: tracer, err: errors.New("the database is unreachable")})
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
		}

		loud := atOrAbove(*records, slog.LevelWarn)
		if len(loud) != 1 || loud[0].message != "request" || loud[0].level != slog.LevelError {
			t.Fatalf("want the one ERROR access line, got %+v", loud)
		}

		if !strings.Contains(loud[0].attrs["error"], "the database is unreachable") {
			t.Errorf("the cause is not on the line: %+v", loud[0].attrs)
		}
	})

	for name, standing := range map[string]domain.AccountStanding{
		"a_disabled_account_is_no_line_above_info": domain.AccountDisabled,
		"a_missing_account_is_no_line_above_info":  domain.AccountMissing,
	} {
		t.Run(name, func(t *testing.T) {
			tracer, _ := recorder(t)
			records := recordLogs(t)

			rec := serve(t, tracer, fixedStanding(standing))
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
			}

			if loud := atOrAbove(*records, slog.LevelWarn); len(loud) != 0 {
				t.Errorf("a refused request is not an error of the service, and it wrote %+v", loud)
			}
		})
	}
}

// A failure's detail -- the identity provider's own words, which the
// caller never reads -- is on the one line about it: the ERROR access line
// of a 503, and the DEBUG line of a refusal. It was a second line of the
// adapter's, at WARN and then at DEBUG, and the 503's ERROR line said only
// "the identity provider did not accept this service's request".
func TestTheLineAboutAFailureCarriesItsDetail(t *testing.T) {
	for name, tt := range map[string]struct {
		err     error
		level   slog.Level
		message string
	}{
		"a_failure": {
			err:     &domain.InvalidAuthnServiceError{Message: "the identity provider did not accept this service's request", Detail: "oauth2: \"invalid_client\" \"the client secret has expired\""},
			level:   slog.LevelError,
			message: "request",
		},
		"a_refusal": {
			err:     &domain.IDPIdentityNotLinkedError{Detail: "auto-provisioning is off"},
			level:   slog.LevelDebug,
			message: "request_refused",
		},
	} {
		t.Run(name, func(t *testing.T) {
			tracer, _ := recorder(t)
			records := recordLogs(t)

			rec := serve(t, tracer, through(tracer, tt.err, func(w http.ResponseWriter, r *http.Request, err error) {
				respond.WriteError(w, r, err)
			}))

			if strings.Contains(rec.Body.String(), tt.err.(interface{ FailureDetail() string }).FailureDetail()) {
				t.Errorf("the detail reached the caller: %s", rec.Body.String())
			}

			var lines []logged
			for _, r := range *records {
				if r.level == tt.level && r.message == tt.message {
					lines = append(lines, r)
				}
			}

			if len(lines) != 1 {
				t.Fatalf("want one %s line at %s, got %d: %+v", tt.message, tt.level, len(lines), *records)
			}

			if got, want := lines[0].attrs["detail"], tt.err.(interface{ FailureDetail() string }).FailureDetail(); got != want {
				t.Errorf("the line's detail is %q, want %q", got, want)
			}

			// A refusal's detail can be the caller's own text (the callback's
			// error_description): it stays on the DEBUG line, never the INFO
			// access line an operator reads by default.
			if tt.level == slog.LevelDebug {
				for _, r := range *records {
					if r.level == slog.LevelInfo && r.message == "request" {
						if d, ok := r.attrs["detail"]; ok {
							t.Errorf("the refusal's INFO access line carries detail %q", d)
						}
					}
				}
			}
		})
	}
}

// TestACallerWhoWentAwayIsNotAnError: the client closed the connection
// before the request was answered -- a tab closed during a sign-in's code
// exchange. Nobody did anything wrong and nobody reads the answer. The
// access line said 500 at ERROR; it says 499 at INFO, with what ended the
// request named, and the server span is not red.
func TestACallerWhoWentAwayIsNotAnError(t *testing.T) {
	tracer, spans := recorder(t)
	records := recordLogs(t)

	gone := fmt.Errorf("exchanging the code: %w", context.Canceled)

	mux := http.NewServeMux()
	mux.Handle("GET /things", through(tracer, gone, func(w http.ResponseWriter, r *http.Request, err error) {
		respond.WriteError(w, r, err)
	}))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	rec := httptest.NewRecorder()
	chain(t, tracer, mux).ServeHTTP(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, "/things", nil))

	if rec.Code != respond.StatusClientClosedRequest {
		t.Fatalf("status = %d, want %d", rec.Code, respond.StatusClientClosedRequest)
	}

	if loud := atOrAbove(*records, slog.LevelWarn); len(loud) != 0 {
		t.Fatalf("a request its caller abandoned wrote %d line(s) at WARN or above: %+v", len(loud), loud)
	}

	info := atOrAbove(*records, slog.LevelInfo)
	if len(info) != 1 || info[0].message != "request" {
		t.Fatalf("want exactly one INFO line, the access line; got %+v", info)
	}

	if got := info[0].attrs["status"]; got != "499" {
		t.Errorf("the access line says status=%s, want 499", got)
	}

	if got := info[0].attrs["error_type"]; got != "context.Canceled" {
		t.Errorf("the access line says error_type=%q, want context.Canceled", got)
	}

	for _, s := range spans.Ended() {
		if s.SpanKind() == trace.SpanKindServer && s.Status().Code == codes.Error {
			t.Errorf("the server span %q is marked Error for a request its caller abandoned", s.Name())
		}
	}
}
