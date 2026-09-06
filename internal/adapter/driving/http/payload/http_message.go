package payload

import "time"

// HTTPMessage represents a message to be sent to the client trough HTTP REST API.
//
//	@Description	HTTPMessage represents a message to be sent to the client trough HTTP REST API.
type HTTPMessage struct {
	Timestamp time.Time `json:"timestamp" example:"2021-07-01T00:00:00Z" format:"date-time"`
	Message   string    `json:"message" example:"success" format:"string"`
	// RequestID is the id the X-Request-ID response header carries, repeated
	// here so a client that only kept the body can still quote it. A 500
	// says nothing else; this is what an operator joins to the log.
	RequestID string `json:"request_id,omitempty" example:"01a075b7-8dda-7bdb-94e2-8000a61c171c" format:"uuid"`

	// Code is a stable, machine-readable reason, present only where a client
	// has to BRANCH on it. Message is prose and may be reworded; this is not.
	//
	// It exists because a 401 has two meanings a client must treat differently:
	// an expired access token should be refreshed and the request retried,
	// while a revoked one must not be — the refresh token was revoked in the
	// same breath, so the retry burns two requests and fails anyway. Nothing in
	// the old response distinguished them, and a client cannot be asked to
	// match on English.
	//
	// Empty on every response that does not need one, so no existing client
	// sees a change.
	Code       string `json:"code,omitempty" example:"token_revoked" format:"string"`
	Method     string `json:"method" example:"GET" format:"string"`
	Path       string `json:"path" example:"/api/v1/users" format:"string"`
	StatusCode int    `json:"status_code" example:"200" format:"int32"`
}

// String returns the message as a string.
func (e *HTTPMessage) String() string {
	return e.Message
}

// Error returns the message as an error.
func (e *HTTPMessage) Error() string {
	return e.Message
}
