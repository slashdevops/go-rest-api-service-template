package domain

// InvalidLimitError represents an invalid pagination limit error
type InvalidLimitError struct {
	Message  string
	Limit    int // optional: the invalid limit value
	MinLimit int // optional: minimum allowed
	MaxLimit int // optional: maximum allowed
}

func (e *InvalidLimitError) Error() string {
	return (&BaseInvalidFieldError{
		Field:     "limit",
		Reason:    e.Message,
		MinLength: e.MinLimit,
		MaxLength: e.MaxLimit,
	}).Error()
}

// InvalidTokenError represents an invalid pagination token error
type InvalidTokenError struct {
	Token   string // optional: the invalid token
	Reason  string // optional: why token is invalid
	Message string
}

func (e *InvalidTokenError) Error() string {
	reason := e.Reason
	if reason == "" {
		reason = e.Message
	}

	return (&BaseInvalidFieldError{
		Field:  "token",
		Value:  e.Token,
		Reason: reason,
	}).Error()
}

// InvalidCursorError represents an invalid pagination cursor error
type InvalidCursorError struct {
	Cursor  string // optional: the invalid cursor
	Reason  string // optional: why cursor is invalid
	Message string
}

func (e *InvalidCursorError) Error() string {
	reason := e.Reason
	if reason == "" {
		reason = e.Message
	}

	return (&BaseInvalidFieldError{
		Field:  "cursor",
		Value:  e.Cursor,
		Reason: reason,
	}).Error()
}
