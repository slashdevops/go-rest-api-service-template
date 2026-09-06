package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"uuid"

	"github.com/golang-jwt/jwt/v5"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/middleware"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// parseUUIDQueryParams parses a string into a UUID.
// If the input is empty, it returns an error.
// If the input is not a valid UUID, it returns an error.
// If the input is a nil UUID, it returns an error.
func parseUUIDQueryParams(input string) (uuid.UUID, error) {
	if input == "" {
		return uuid.Nil(), &domain.InvalidUUIDError{Message: "input is empty"}
	}

	id, err := uuid.Parse(input)
	if err != nil {
		return uuid.Nil(), &domain.InvalidUUIDError{UUID: input, Message: uuidParseReason(input)}
	}

	if !domain.IsUUIDV7(id) {
		return uuid.Nil(), &domain.InvalidUUIDError{UUID: input, Message: "input is nil"}
	}

	return id, nil
}

// uuidParseReason explains why input could not be parsed as a UUID.
//
// The standard library's [uuid.Parse] collapses every failure into the single
// string "invalid uuid", where github.com/google/uuid distinguished a wrong
// length from wrong characters. That text is not internal — it is forwarded to
// clients as the Reason of a [domain.InvalidUUIDError], so letting the package
// swap change it would silently change a published error response. Reproduce
// the previous wording here instead, and keep this the only place that decides
// it.
//
// The accepted lengths are the five forms [uuid.Parse] takes: 32 unhyphenated,
// 36 canonical, 38 braced, and 45 for the "urn:uuid:" prefix.
func uuidParseReason(input string) string {
	switch len(input) {
	case 32, 36, 38, 45:
		return "invalid UUID format"
	default:
		return fmt.Sprintf("invalid UUID length: %d", len(input))
	}
}

// normalizeFieldsQueryParam strips the insignificant spaces from a partial
// fields expression so downstream consumers receive a canonical comma list.
//
// Allow-list validation of the sort/filter/fields expressions is intentionally
// not performed here: it lives in the domain layer (each entity's Validate,
// which the use-case runs), so the driving adapter only transports and
// normalizes the raw request. Invalid fields surface as a domain
// ValidationError that httpStatusForListError maps to 400.
func normalizeFieldsQueryParam(fields string) string {
	return strings.ReplaceAll(fields, " ", "")
}

// parseNextTokenQueryParams parses a string into a nextToken field.
func parseNextTokenQueryParams(nextToken string) (string, error) {
	if nextToken != "" {
		// The decoder's text ("invalid cursor: not base64") is not forwarded:
		// a token is opaque, and how it failed is nothing a client acts on.
		if _, _, _, err := domain.DecodeToken(nextToken, domain.TokenDirectionNext); err != nil {
			return "", &domain.InvalidTokenError{Message: "invalid next_token"}
		}
	}

	return nextToken, nil
}

// parsePrevTokenQueryParams parses a string into a prevToken field.
func parsePrevTokenQueryParams(prevToken string) (string, error) {
	if prevToken != "" {
		if _, _, _, err := domain.DecodeToken(prevToken, domain.TokenDirectionPrev); err != nil {
			return "", &domain.InvalidTokenError{Message: "invalid prev_token"}
		}
	}

	return prevToken, nil
}

// parseLimitQueryParams parses a string into a limit field.
func parseLimitQueryParams(limit string) (int, error) {
	var limitInt int
	var err error

	if limit == "" {
		return domain.PaginatorDefaultLimit, nil
	}

	// check if this is a valid integer
	if limitInt, err = strconv.Atoi(limit); err != nil {
		return 0, &domain.InvalidLimitError{MinLimit: domain.PaginatorMinLimit, MaxLimit: domain.PaginatorMaxLimit}
	}

	// Check if the limit is within the allowed range (must be greater than 0)
	if limitInt <= 0 || limitInt > domain.PaginatorMaxLimit {
		return 0, &domain.InvalidLimitError{MinLimit: domain.PaginatorMinLimit, MaxLimit: domain.PaginatorMaxLimit}
	}

	return limitInt, nil
}

// parseListQueryParams extracts and normalizes the list query params. It only
// transports/normalizes the request (sort and filter pass through verbatim,
// fields is space-normalized) and validates the pagination tokens and limit.
//
// Allow-list validation of sort/filter/fields is deliberately left to the
// domain layer (the entity's Validate, run by the use-case); a bad field then
// comes back as a domain ValidationError that httpStatusForListError maps to
// 400. This avoids validating the same expressions twice per request.
func parseListQueryParams(params map[string]any) (
	sort string,
	filter string,
	fields string,
	nextToken string,
	prevToken string,
	limit int,
	err error,
) {
	sort = params["sort"].(string)
	filter = params["filter"].(string)
	fields = normalizeFieldsQueryParam(params["fields"].(string))

	nextToken, err = parseNextTokenQueryParams(params["nextToken"].(string))
	if err != nil {
		return "", "", "", "", "", 0, err
	}

	prevToken, err = parsePrevTokenQueryParams(params["prevToken"].(string))
	if err != nil {
		return "", "", "", "", "", 0, err
	}

	limit, err = parseLimitQueryParams(params["limit"].(string))
	if err != nil {
		return "", "", "", "", "", 0, err
	}

	return sort, filter, fields, nextToken, prevToken, limit, nil
}

// httpStatusForListError maps an error returned while serving a list endpoint
// to an HTTP status code. Malformed query parameters surface either as a domain
// validation error (an invalid sort/filter/fields field rejected by the
// entity's Validate) or as one of the PostgreSQL errors triggered by a
// malformed expression; both are client faults and map to 400. Anything else is
// treated as an unexpected server-side failure.
func httpStatusForListError(err error) int {
	switch {
	case isErr[*domain.ValidationErrors](err),
		isErr[*domain.ValidationError](err),
		isErr[*domain.InvalidByteSequenceError](err),
		isErr[*domain.InvalidMessageFormatError](err),
		isErr[*domain.UndefinedColumnError](err),
		isErr[*domain.DatatypeMismatchError](err):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// isErr reports whether err, or any error it wraps, is of the concrete type T.
func isErr[T error](err error) bool {
	_, ok := errors.AsType[T](err)
	return ok
}

// getTokenFromContext returns the bearer token the auth middleware verified.
//
// A handler that acts on the token — rather than merely on who it identifies —
// must use this rather than reading the request again, so that the token that
// was authorised and the token that is acted on cannot differ.
func getTokenFromContext(ctx context.Context) (string, error) {
	token, ok := ctx.Value(middleware.JwtToken).(string)
	if !ok || token == "" {
		return "", &domain.InvalidJWTError{Message: "the verified token is missing from the context"}
	}

	return token, nil
}

// getUserIDFromContext extracts the user ID from the context.
// It expects the user ID to be stored in the JWT claims under the "sub" key.
// If the "sub" claim is missing or not a string, it returns an error.
// If the user ID is not a valid UUID, it returns an error.
// If the user ID is a nil UUID, it returns an error.
func getUserIDFromContext(ctx context.Context) (uuid.UUID, error) {
	claimsValue := ctx.Value(middleware.JwtClaims)
	if claimsValue == nil {
		return uuid.Nil(), &domain.InvalidJWTError{Message: "JWT claims are missing from context"}
	}

	claims, ok := claimsValue.(map[string]any)
	if !ok {
		return uuid.Nil(), &domain.InvalidJWTError{Message: "JWT claims are not in the expected format"}
	}

	userIDstring, ok := claims["sub"].(string)
	if !ok {
		return uuid.Nil(), &domain.InvalidJWTError{Message: "sub claim is missing or not a string"}
	}

	userID, err := uuid.Parse(userIDstring)
	if err != nil {
		return uuid.Nil(), &domain.InvalidJWTError{Message: uuidParseReason(userIDstring)}
	}

	return userID, nil
}

// getJWTExpiration extracts the expiration time from a JWT string.
// It returns the expiration time as a time.Time and an error if the token is invalid
// or the 'exp' claim is missing.
func getJWTExpiration(tokenString string) (time.Time, error) {
	// Parse the token without verifying the signature. We only want to read the claims.
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse token: %w", err)
	}

	// Get the claims from the parsed token.
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return time.Time{}, fmt.Errorf("invalid token claims")
	}

	// Use the helper function to get the expiration time.
	exp, err := claims.GetExpirationTime()
	if err != nil {
		// This error is returned if 'exp' exists but is not a valid number.
		return time.Time{}, fmt.Errorf("could not get expiration time from claims: %w", err)
	}

	// Check if the expiration claim was present.
	if exp == nil {
		return time.Time{}, fmt.Errorf("token does not have an expiration 'exp' claim")
	}

	// The expiration time is valid, return it directly as time.Time.
	return exp.Time, nil
}

// decodeJSONBody decodes one JSON value from the request body into dst.
//
// Unknown fields are refused. A field the API dropped -- the IdP redirect
// URLs, the password on PUT /users -- used to be accepted and silently
// ignored, so a client kept sending what it believed was being honoured. Now
// it is told. Anything after the first value is refused too; the decoder
// used to read one value and ignore the rest.
//
// The error is for [respond.WriteDecodeError], which turns it into one 400
// wording (or 413 when the body was cut by the size bound).
func decodeJSONBody(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		if name, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
			// The field name is the caller's, quoted by the decoder; the
			// wording that goes out is ours.
			return &domain.InvalidRequestError{Message: "unknown field " + name}
		}

		return err
	}

	if dec.More() {
		return &domain.InvalidRequestError{Message: "unexpected data after the JSON body"}
	}

	return nil
}
