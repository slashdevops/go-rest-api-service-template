package jwtvalidator

type InvalidClaimsError struct {
	Message string
}

func (e *InvalidClaimsError) Error() string {
	if e.Message != "" {
		return e.Message
	}

	return "invalid claims"
}

type InvalidTokenError struct {
	Message string
}

func (e *InvalidTokenError) Error() string {
	if e.Message != "" {
		return e.Message
	}

	return "invalid token"
}
