package respond

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
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

// WriteInternalError answers 500 with a fixed message and logs the cause
// under the request id, so an operator can join the two and a caller cannot
// read the first.
func WriteInternalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("internal server error",
		"request_id", RequestIDFrom(r.Context()),
		"method", r.Method,
		"path", r.URL.Path,
		"error", err,
	)

	writeJSONMessage(w, r, http.StatusInternalServerError, "", InternalServerErrorMessage)
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
