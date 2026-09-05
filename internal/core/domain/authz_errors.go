package domain

// UnauthorizedError represents an authorization error
type UnauthorizedError struct {
	Resource string // optional: which resource
	Action   string // optional: which action
	Message  string // optional: additional context
}

func (e *UnauthorizedError) Error() string {
	base := &BaseInvalidFieldError{
		Field:  "authorization",
		Reason: "unauthorized",
	}

	if e.Resource != "" && e.Action != "" {
		base.Reason = "cannot perform '" + e.Action + "' on resource '" + e.Resource + "'"
		if e.Message != "" {
			base.Reason += ": " + e.Message
		}
	} else if e.Resource != "" {
		base.Value = e.Resource
		base.Reason = "unauthorized access to resource"
		if e.Message != "" {
			base.Reason += ": " + e.Message
		}
	} else if e.Action != "" {
		base.Value = e.Action
		base.Reason = "unauthorized action"
		if e.Message != "" {
			base.Reason += ": " + e.Message
		}
	} else if e.Message != "" {
		base.Reason = e.Message
	}

	return base.Error()
}
