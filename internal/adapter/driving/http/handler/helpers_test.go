package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/middleware"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

func TestParseUUIDQueryParams(t *testing.T) {
	testID := uuid.NewV7()

	tests := []struct {
		input    string
		expected uuid.UUID
		err      error
	}{
		{"", uuid.Nil(), &domain.InvalidUUIDError{Message: "input is empty"}},
		{"invalid-uuid", uuid.Nil(), &domain.InvalidUUIDError{UUID: "invalid-uuid", Message: "invalid UUID length: 12"}},
		{uuid.Nil().String(), uuid.Nil(), &domain.InvalidUUIDError{UUID: uuid.Nil().String(), Message: "input is nil"}},
		{testID.String(), testID, nil},
	}

	for _, test := range tests {
		result, err := parseUUIDQueryParams(test.input)
		assert.Equal(t, test.expected, result)
		assert.Equal(t, test.err, err)
	}
}

func TestNormalizeFieldsQueryParam(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"id,name", "id,name"},
		{"id, name, email", "id,name,email"},
		{"  id ,  name ", "id,name"},
	}

	for _, test := range tests {
		assert.Equal(t, test.want, normalizeFieldsQueryParam(test.in))
	}
}

func TestHTTPStatusForListError(t *testing.T) {
	// Sort/filter/fields allow-list validation now lives in the domain layer,
	// so a bad field comes back from the use-case as a domain validation error;
	// together with the pg errors a malformed expression triggers, these are
	// client faults that must map to 400. Everything else is a 500.
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"validation_errors", &domain.ValidationErrors{Errors: []domain.ValidationError{{Field: "sort", Message: "field not allowed"}}}, http.StatusBadRequest},
		{"validation_error", &domain.ValidationError{Field: "sort", Message: "field not allowed"}, http.StatusBadRequest},
		{"invalid_byte_sequence", &domain.InvalidByteSequenceError{}, http.StatusBadRequest},
		{"invalid_message_format", &domain.InvalidMessageFormatError{}, http.StatusBadRequest},
		{"undefined_column", &domain.UndefinedColumnError{}, http.StatusBadRequest},
		{"datatype_mismatch", &domain.DatatypeMismatchError{}, http.StatusBadRequest},
		{"wrapped_validation_errors", fmt.Errorf("list failed: %w", &domain.ValidationErrors{Errors: []domain.ValidationError{{Field: "filter"}}}), http.StatusBadRequest},
		{"unknown_error", errors.New("boom"), http.StatusInternalServerError},
		{"not_found_is_not_a_list_client_fault", &domain.RoleNotFoundError{}, http.StatusInternalServerError},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, httpStatusForListError(test.err))
		})
	}
}

func TestParseNextTokenQueryParams(t *testing.T) {
	testID := uuid.NewV7()
	validToken := domain.EncodeToken(testID, 10, domain.TokenDirectionNext)

	tests := []struct {
		name      string
		nextToken string
		expected  string
		err       error
	}{
		{
			name:      "Empty token",
			nextToken: "",
			expected:  "",
			err:       nil,
		},
		{
			name:      "Invalid token",
			nextToken: "invalid",
			expected:  "",
			err:       &domain.InvalidTokenError{Message: "invalid token: illegal base64 data at input byte 4"},
		},
		{
			name:      "Valid token",
			nextToken: validToken,
			expected:  validToken,
			err:       nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := parseNextTokenQueryParams(test.nextToken)

			if test.err == nil {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.IsType(t, test.err, err)
				// assert.Contains(t, err.Error(), test.err.Error())
			}

			assert.Equal(t, test.expected, result)
		})
	}
}

func TestParsePrevTokenQueryParams(t *testing.T) {
	testID := uuid.NewV7()
	validPrevToken := domain.EncodeToken(testID, 10, domain.TokenDirectionPrev)
	validNextToken := domain.EncodeToken(testID, 10, domain.TokenDirectionNext)

	tests := []struct {
		name      string
		prevToken string
		expected  string
		err       error
	}{
		{
			name:      "Empty token",
			prevToken: "",
			expected:  "",
			err:       nil,
		},
		{
			name:      "Invalid token (bad format)",
			prevToken: "invalid", // Input that causes a base64 decoding error
			expected:  "",
			err:       &domain.InvalidTokenError{Message: "invalid prev_token"},
		},
		{
			name:      "Valid token (encoded as Prev)",
			prevToken: validPrevToken,
			expected:  validPrevToken,
			err:       nil,
		},
		{
			name:      "Invalid token (direction mismatch - next token for prev param)",
			prevToken: validNextToken,
			expected:  "", // On error, the function returns an empty string for the token
			err:       &domain.InvalidTokenError{Message: "invalid prev_token"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := parsePrevTokenQueryParams(test.prevToken)

			if test.err == nil {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.IsType(t, test.err, err)
				// Check if the actual error message contains the expected message
				// This provides flexibility if the error messages don't match exactly
				// but the core issue is the same.
				assert.Contains(t, err.Error(), test.err.Error())
			}

			assert.Equal(t, test.expected, result)
		})
	}
}

func TestParseLimitQueryParams(t *testing.T) {
	tests := []struct {
		name     string
		limit    string
		expected int
		err      error
	}{
		{
			name:     "empty string",
			limit:    "",
			expected: domain.PaginatorDefaultLimit,
			err:      nil,
		},
		{
			name:     "invalid string",
			limit:    "invalid",
			expected: 0,
			err:      &domain.InvalidLimitError{MinLimit: domain.PaginatorMinLimit, MaxLimit: domain.PaginatorMaxLimit},
		},
		{
			name:     "zero returns error",
			limit:    "0",
			expected: 0,
			err:      &domain.InvalidLimitError{MinLimit: domain.PaginatorMinLimit, MaxLimit: domain.PaginatorMaxLimit},
		},
		{
			name:     "valid limit",
			limit:    "5",
			expected: 5,
			err:      nil,
		},
		{
			name:     "negative limit",
			limit:    "-1",
			expected: 0,
			err:      &domain.InvalidLimitError{MinLimit: domain.PaginatorMinLimit, MaxLimit: domain.PaginatorMaxLimit},
		},
		{
			name:     "max limit",
			limit:    "1000",
			expected: domain.PaginatorMaxLimit,
			err:      nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := parseLimitQueryParams(test.limit)
			assert.Equal(t, test.expected, result)

			if test.err == nil {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.IsType(t, test.err, err)
			}
		})
	}
}

func TestParseListQueryParams(t *testing.T) {
	t.Run("TestParseListQueryParams", func(t *testing.T) {
		testID := uuid.NewV7()

		params := map[string]any{
			"sort":      "name ASC",
			"filter":    "status='active'",
			"fields":    "id, name",
			"nextToken": domain.EncodeToken(testID, 10, domain.TokenDirectionNext),
			"prevToken": domain.EncodeToken(testID, 10, domain.TokenDirectionPrev),
			"limit":     "5",
		}

		sort, filter, fields, nextToken, prevToken, limit, err := parseListQueryParams(params)
		assert.NoError(t, err)
		assert.Equal(t, "name ASC", sort)
		assert.Equal(t, "status='active'", filter)
		assert.Equal(t, "id,name", fields)
		assert.Equal(t, domain.EncodeToken(testID, 10, domain.TokenDirectionNext), nextToken)
		assert.Equal(t, domain.EncodeToken(testID, 10, domain.TokenDirectionPrev), prevToken)
		assert.Equal(t, 5, limit)
	})

	t.Run("passes_sort_and_filter_through_without_validating", func(t *testing.T) {
		// Allow-list validation of sort/filter/fields moved to the domain
		// layer, so the transport no longer rejects unknown fields here; it
		// passes sort/filter through verbatim and only space-normalizes fields.
		params := map[string]any{
			"sort":      "unknown_field DESC",
			"filter":    "unknown='x'",
			"fields":    "unknown, other",
			"nextToken": "",
			"prevToken": "",
			"limit":     "5",
		}

		sort, filter, fields, _, _, _, err := parseListQueryParams(params)
		assert.NoError(t, err)
		assert.Equal(t, "unknown_field DESC", sort)
		assert.Equal(t, "unknown='x'", filter)
		assert.Equal(t, "unknown,other", fields)
	})

	t.Run("rejects_invalid_limit", func(t *testing.T) {
		params := map[string]any{
			"sort":      "",
			"filter":    "",
			"fields":    "",
			"nextToken": "",
			"prevToken": "",
			"limit":     "not-a-number",
		}

		_, _, _, _, _, _, err := parseListQueryParams(params)
		assert.Error(t, err)
		assert.IsType(t, &domain.InvalidLimitError{}, err)
	})
}

func TestGetUserIDFromContext(t *testing.T) {
	testID := uuid.NewV7()

	tests := []struct {
		name        string
		setupCtx    func() context.Context
		expected    uuid.UUID
		expectedErr error
	}{
		{
			name: "missing_jwt_claims",
			setupCtx: func() context.Context {
				return context.Background()
			},
			expected:    uuid.Nil(),
			expectedErr: &domain.InvalidJWTError{Message: "JWT claims are missing from context"},
		},
		{
			name: "invalid_claims_format",
			setupCtx: func() context.Context {
				ctx := context.Background()
				return context.WithValue(ctx, middleware.JwtClaims, "invalid-format")
			},
			expected:    uuid.Nil(),
			expectedErr: &domain.InvalidJWTError{Message: "JWT claims are not in the expected format"},
		},
		{
			name: "missing_sub_claim",
			setupCtx: func() context.Context {
				ctx := context.Background()
				claims := map[string]any{
					"exp": 1234567890,
					"iat": 1234567890,
				}
				return context.WithValue(ctx, middleware.JwtClaims, claims)
			},
			expected:    uuid.Nil(),
			expectedErr: &domain.InvalidJWTError{Message: "sub claim is missing or not a string"},
		},
		{
			name: "sub_claim_not_string",
			setupCtx: func() context.Context {
				ctx := context.Background()
				claims := map[string]any{
					"sub": 123456,
					"exp": 1234567890,
				}
				return context.WithValue(ctx, middleware.JwtClaims, claims)
			},
			expected:    uuid.Nil(),
			expectedErr: &domain.InvalidJWTError{Message: "sub claim is missing or not a string"},
		},
		{
			name: "invalid_uuid_format",
			setupCtx: func() context.Context {
				ctx := context.Background()
				claims := map[string]any{
					"sub": "invalid-uuid",
					"exp": 1234567890,
				}
				return context.WithValue(ctx, middleware.JwtClaims, claims)
			},
			expected:    uuid.Nil(),
			expectedErr: &domain.InvalidJWTError{Message: "invalid UUID length: 12"},
		},
		{
			name: "valid_user_id",
			setupCtx: func() context.Context {
				ctx := context.Background()
				claims := map[string]any{
					"sub": testID.String(),
					"exp": 1234567890,
				}
				return context.WithValue(ctx, middleware.JwtClaims, claims)
			},
			expected:    testID,
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := test.setupCtx()
			result, err := getUserIDFromContext(ctx)

			assert.Equal(t, test.expected, result)
			if test.expectedErr == nil {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.IsType(t, test.expectedErr, err)
				assert.Contains(t, err.Error(), test.expectedErr.Error())
			}
		})
	}
}

func TestGetJWTExpiration(t *testing.T) {
	tests := []struct {
		name       string
		tokenStr   string
		wantErr    bool
		errMessage string
		wantTime   int64 // Unix timestamp for easy comparison
	}{
		{
			name:     "Valid JWT with expiration",
			tokenStr: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiZXhwIjoxNTg5ODM0NzU5fQ.QQ_3Yinmu5YLx5mXLkDxhIgdYkCckuiGjHYrBHDLpVQ",
			wantErr:  false,
			wantTime: 1589834759, // May 18, 2020 7:12:39 PM GMT
		},
		{
			name:       "Invalid JWT format",
			tokenStr:   "not-a-valid-jwt",
			wantErr:    true,
			errMessage: "failed to parse token",
		},
		{
			name:       "JWT without expiration claim",
			tokenStr:   "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIn0.Gfx6VO9tcxwk6xqx9yYzSfebfeakZp5JYIgP_edcw_A",
			wantErr:    true,
			errMessage: "does not have an expiration",
		},
		{
			name:       "JWT with invalid expiration type",
			tokenStr:   "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiZXhwIjoibm90LWEtbnVtYmVyIn0.T12ink_7E10QxZvAQ2Y6Z_OPLMk2FeSTMhyW9MKR3Z0",
			wantErr:    true,
			errMessage: "could not get expiration time from claims",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expTime, err := getJWTExpiration(tt.tokenStr)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMessage)
				assert.True(t, expTime.IsZero(), "Expected zero time for error case")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantTime, expTime.Unix(), "Expiration time does not match expected value")
				assert.False(t, expTime.IsZero(), "Expected non-zero time for success case")
			}
		})
	}
}
