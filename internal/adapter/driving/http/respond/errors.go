package respond

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

const (
	// InternalServerErrorMessage is the whole body of every 500. The cause
	// goes to the log with the request id, never to the client: a 500 is
	// by definition an error the caller cannot act on, and its text was the
	// database driver's, the JSON decoder's or a dependency's. Measured on
	// 2026-09-06, 218 handler sites wrote err.Error() into a 500.
	InternalServerErrorMessage = "internal server error"
	// BodyTooLargeMessage is the 413 body. The limit is configuration and
	// the message is contract, so the number is not in it.
	BodyTooLargeMessage = "request body too large"
	// DecodeErrorMessage is the 400 body for a request body that is not the
	// JSON the route expects. One wording, whatever the decoder said.
	DecodeErrorMessage = "failed to decode request body"
)

// StatusClientClosedRequest is the status of a request whose caller went
// away before it was answered: nginx's 499. Nobody receives it -- the
// connection is gone -- so no route declares it, and it is written with no
// body. It is what the access line says: INFO, with the error's type, as
// every refusal is.
const StatusClientClosedRequest = 499

// WriteCallerGone answers a request whose caller went away, and reports
// whether it did. It went away when err is the request's own cancellation:
// the client closed the connection -- a tab closed while a sign-in's code
// was being exchanged, a client that gave up on its side -- before the
// request was answered. Then it is answered [StatusClientClosedRequest] with
// no body, the failure is handed to the access line, and it reports true.
// Otherwise it writes nothing and reports false.
//
// Both halves are asked: a context.Canceled that reaches the handler while
// the request is still alive was cancelled by something else, and that
// caller is still there to be answered.
//
// A deadline is not the caller leaving. context.DeadlineExceeded -- a cache
// read past its timeout, a generation past llm.generate.timeout -- is
// something that was slow, and stays a failure.
//
// Every site that answers an error with a 500 asks it first:
// [WriteInternalError], and through it [WriteError], and the few that write
// a 500 sentence of their own (TestEvery500AsksWhetherTheCallerWentAway).
// Without it the calls below were counted refused and the request answered
// 500 at ERROR: the status and the outcome disagreed.
func WriteCallerGone(w http.ResponseWriter, r *http.Request, err error) bool {
	if !errors.Is(err, context.Canceled) || !errors.Is(r.Context().Err(), context.Canceled) {
		return false
	}

	o11y.NoteFailure(r.Context(), err)
	w.WriteHeader(StatusClientClosedRequest)

	return true
}

// WriteInternalError answers 500 with a fixed message and has the cause
// logged under the request id, so an operator can join the two and a caller
// cannot read the first.
//
// The cause goes on the request's access line, which is written at ERROR for
// a 5xx with what failed and where (middleware.Logging): the cause is handed
// to the request's failure holder, where a handler's RecordError has usually
// put it already. It used to be logged here as well, a second ERROR line for
// one failure. With no holder -- a handler driven directly, outside the
// middleware chain -- it is logged here, as before.
//
// A request whose caller went away is not a 500: nobody did anything wrong
// and nobody receives the answer. [WriteCallerGone] answers it, and the
// access line says so at INFO.
func WriteInternalError(w http.ResponseWriter, r *http.Request, err error) {
	if WriteCallerGone(w, r, err) {
		return
	}

	if !o11y.NoteFailure(r.Context(), err) {
		slog.ErrorContext(r.Context(), "internal server error",
			"request_id", RequestIDFrom(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"error", err,
		)
	}

	writeJSONMessage(w, r, http.StatusInternalServerError, "", InternalServerErrorMessage)
}

// DecodeError marks an error that came from decoding a request's body. It is
// transparent: its message is the wrapped error's, and so is everything
// errors.Is and errors.AsType find in it. The decoder's errors have no type
// of ours -- they are encoding/json's, or io's -- so nothing could tell that
// such an error is the caller's to fix, which it always is:
// [WriteDecodeError] answers every one with a 400 or a 413. [IsRefusal]
// needs to know that: without the mark, a client posting malformed JSON
// would count as a failure of the service.
type DecodeError struct {
	Err error
}

func (e *DecodeError) Error() string { return e.Err.Error() }

func (e *DecodeError) Unwrap() error { return e.Err }

// IsRefusal reports whether err is one the service answers by telling the
// caller what they got wrong: a status below 500 in the one table of errors
// and statuses ([StatusFor]), or a body that could not be decoded, which is
// answered by [WriteDecodeError] and is in no table. An error the table does
// not know is a 500, and so a failure.
//
// Or a request its caller abandoned (context.Canceled), answered
// [StatusClientClosedRequest] by [WriteCallerGone] and in no table either:
// the caller's side. It was a 500, counted `failed`, and an ERROR access
// line, for a tab closed during a sign-in. Every layer's call returns the
// same context.Canceled on the way up, and each is counted refused with it
// (the cache's is c3e's own, and stays failed). context.DeadlineExceeded is
// not matched: something slow is a failure. The classifier sees the error
// and not the request, so a context.Canceled raised while the caller is
// still there would be answered 500 and counted refused; nothing in the
// request path cancels a context of its own.
func IsRefusal(err error) bool {
	if _, undecodable := errors.AsType[*DecodeError](err); undecodable {
		return true
	}

	if errors.Is(err, context.Canceled) {
		return true
	}

	return StatusFor(err) < http.StatusInternalServerError
}

// WriteDecodeError answers a failed request-body decode.
//
// A body cut by the size bound is 413, so a client learns to send less
// rather than to fix its JSON. Everything else is 400 with one of three
// wordings, all ours: a field of the wrong type names the JSON field (the
// name is the caller's, not the library's), an id that is not a uuid says
// so, and any other failure -- a syntax error, an empty body, trailing bytes
// -- is the bare message. The decoder's own text used to be forwarded
// ("unexpected EOF", "invalid character 'x' looking for beginning of
// value"), which shipped encoding/json's error strings as part of this API's
// contract.
func WriteDecodeError(w http.ResponseWriter, r *http.Request, err error) {
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		writeJSONMessage(w, r, http.StatusRequestEntityTooLarge, "", BodyTooLargeMessage)
		return
	}

	message := DecodeErrorMessage

	if reqErr, ok := errors.AsType[*domain.InvalidRequestError](err); ok {
		// Raised by the handler's decoder for an unknown field or trailing
		// data: the message is this service's own.
		message += ": " + reqErr.Message
	} else if typeErr, ok := errors.AsType[*json.UnmarshalTypeError](err); ok && typeErr.Field != "" {
		message += ": field '" + typeErr.Field + "' has the wrong type"
	} else if err != nil && err.Error() == uuidParseFailure {
		// The standard library's uuid.Parse has exactly one failure text and
		// no error type, and the decoder returns it bare. Matching it is the
		// same accommodation handler.uuidParseReason makes for path ids; the
		// wording that goes out is ours.
		message += ": invalid uuid"
	}

	writeJSONMessage(w, r, http.StatusBadRequest, "", message)
}

// uuidParseFailure is the one string the standard library's uuid.Parse fails
// with. See [WriteDecodeError].
const uuidParseFailure = "invalid uuid"
