package domain

import (
	"uuid"
)

// RateLimitNotFoundError represents an error when a rate limit cannot be found.
type RateLimitNotFoundError struct {
	Name    string // optional: search by name
	Message string // optional: additional context
	ID      uuid.UUID
}

func (e *RateLimitNotFoundError) Error() string {
	base := &BaseNotFoundError{
		Entity:  "rate limit",
		ID:      e.ID,
		Message: e.Message,
	}

	if e.Name != "" {
		base.Field = "name"
		base.Value = e.Name
	}

	return base.Error()
}

// RateLimitAlreadyExistsError represents a duplicate name.
//
// This is what unique_rate_limit_name becomes on the way out. The constraint
// name is a CONTRACT: renaming it in a later migration without updating the
// constant in repositorypg turns this documented 409 back into a 500.
type RateLimitAlreadyExistsError struct {
	Name    string
	Message string
	ID      uuid.UUID
}

func (e *RateLimitAlreadyExistsError) Error() string {
	base := &BaseAlreadyExistsError{
		Entity:  "rate limit",
		ID:      e.ID,
		Message: e.Message,
	}

	if e.Name != "" {
		base.Field = "name"
		base.Value = e.Name
	}

	return base.Error()
}

// SystemRateLimitError is what the tr_restrict_delete_update_on_system_rate_limits
// trigger becomes on the way out: a 403, not a 500.
type SystemRateLimitError struct {
	Name        string // optional: rule name
	Action      string // optional: what was attempted
	RateLimitID uuid.UUID
}

func (e *SystemRateLimitError) Error() string {
	return (&BaseProtectedError{
		Entity: "rate limit",
		ID:     e.RateLimitID,
		Name:   e.Name,
		Action: e.Action,
	}).Error()
}

// InvalidRateLimitTargetError is returned when a rule names a route this service
// does not serve.
//
// Catching it at write time is the entire reason the check exists. A rule for a
// path that does not exist is not inert -- it looks correct in a listing, it
// reports no error, and it silently protects nothing. The failure mode is a
// limit an operator believes is in place.
type InvalidRateLimitTargetError struct {
	Target string
	Method string // optional: the verb that is not registered on that path
	Reason string
}

func (e *InvalidRateLimitTargetError) Error() string {
	reason := e.Reason
	if reason == "" {
		reason = "no route matches this target"
	}

	if e.Method != "" {
		reason = e.Method + " " + e.Target + ": " + reason
	}

	return (&BaseInvalidFieldError{
		Entity: "rate limit",
		Field:  "target",
		Value:  e.Target,
		Reason: reason,
	}).Error()
}

// InvalidRateLimitStrategyError is returned when a strategy string cannot be
// turned into a limiter.
//
// It carries the valid values because the alternative -- silently defaulting --
// would enforce a limiter the operator did not ask for, with nothing in the
// response, the logs or the metrics to say so.
type InvalidRateLimitStrategyError struct {
	Strategy string
	Valid    []string
}

func (e *InvalidRateLimitStrategyError) Error() string {
	reason := "must be one of " + joinStrategies()
	if len(e.Valid) > 0 {
		reason = "must be one of " + joinStringsComma(e.Valid)
	}

	return (&BaseInvalidFieldError{
		Entity: "rate limit",
		Field:  "strategy",
		Value:  e.Strategy,
		Reason: reason,
	}).Error()
}
