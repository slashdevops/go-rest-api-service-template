//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/assert"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

var (
	authLoginEndpoint    = newAPIEndpoint(http.MethodPost, "/auth/login")
	authLogoutEndpoint   = newAPIEndpoint(http.MethodDelete, "/auth/logout")
	authRefreshEndpoint  = newAPIEndpoint(http.MethodPost, "/auth/refresh")
	authRegisterEndpoint = newAPIEndpoint(http.MethodPost, "/auth/register")
	authReVerifyEndpoint = newAPIEndpoint(http.MethodPost, "/auth/verify")

	// The token travels in the Authorization header. It used to be a path
	// segment on GET /auth/verify/{token}, which put a live credential in this
	// service's request log.
	authVerifyConfirmEndpoint = newAPIEndpoint(http.MethodPost, "/auth/verify/confirm")
)

func TestAuthRegisterUser(t *testing.T) {
	t.Run("test_register_single_user", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		firstName, lastName, email := generateUserData(t)
		user := map[string]any{
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   generatePassword(t),
		}

		response, err := sendHTTPRequest(t, ctx, authRegisterEndpoint, user)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusCreated, response.StatusCode, "Expected status code 201. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		t.Cleanup(func() {
			deleteUserByEmailFromDB(t, email)
		})

		assert.Equal(t, domain.AuthnUserRegisteredSuccessfully, apiResp.Message, "Expected success message")
		assert.Equal(t, authRegisterEndpoint.method, apiResp.Method, "Expected method to be set")
		assert.Equal(t, authRegisterEndpoint.Path(), apiResp.Path, "Expected path to be set")
	})

	t.Run("test_register_user_twice_answers_the_same_way", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		firstName, lastName, email := generateUserData(t)
		user := map[string]any{
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   generatePassword(t),
		}

		// 1. Register the user
		firstResponse, err := sendHTTPRequest(t, ctx, authRegisterEndpoint, user)
		assert.NoError(t, err, "Failed to send request")
		defer firstResponse.Body.Close()

		assert.Equal(t, firstResponse.StatusCode, http.StatusCreated, "Expected status code 201. Got %d. Message: %s", firstResponse.StatusCode, readResponseBody(t, firstResponse))

		apiFirstResp, err := parserResponseBody[payload.HTTPMessage](t, firstResponse)
		if err != nil {
			t.Errorf("Failed to parse response body: %v", err)
		}

		assert.Equal(t, http.StatusCreated, firstResponse.StatusCode, "Expected status code 201. Got %d. Message: %s", firstResponse.StatusCode, readResponseBody(t, firstResponse))
		assert.Equal(t, domain.AuthnUserRegisteredSuccessfully, apiFirstResp.Message, "Expected success message")
		assert.Equal(t, authRegisterEndpoint.method, apiFirstResp.Method, "Expected method to be set")
		assert.Equal(t, authRegisterEndpoint.Path(), apiFirstResp.Path, "Expected path to be set")

		// 2. Try to register the same user again
		secondResponse, err := sendHTTPRequest(t, ctx, authRegisterEndpoint, user)
		assert.NoError(t, err, "Failed to send request")
		defer secondResponse.Body.Close()

		// Registering the same address twice answers exactly as the first
		// attempt did. It used to answer 409 "user: already exists:
		// email=<address>", which made this endpoint an account oracle: one
		// unauthenticated request said whether an address was registered.
		//
		// The owner is told by email instead, which nobody probing can see.
		// TestRegistrationDoesNotConfirmTheAddress covers that half.
		apiSecondResp, err := parserResponseBody[payload.HTTPMessage](t, secondResponse)
		assert.NoError(t, err, "Failed to parse response body")

		assert.Equal(t, http.StatusCreated, secondResponse.StatusCode,
			"a second registration must answer like the first. Got %d. Message: %s",
			secondResponse.StatusCode, apiSecondResp.Message)
		assert.Equal(t, apiFirstResp.Message, apiSecondResp.Message,
			"a second registration must carry the same message as the first")
		assert.NotContains(t, apiSecondResp.Message, email,
			"the response must not echo the address that was probed")

		assert.Equal(t, authRegisterEndpoint.method, apiSecondResp.Method, "Expected method to be set")
		assert.Equal(t, authRegisterEndpoint.Path(), apiSecondResp.Path, "Expected path to be set")

		t.Cleanup(func() {
			deleteUserByEmailFromDB(t, email)
		})
	})

	t.Run("test_create_and_verify_user", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Register the user
		firstName, lastName, email := generateUserData(t)
		user := map[string]any{
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   generatePassword(t),
		}

		// 1. Register the user
		response, err := sendHTTPRequest(t, ctx, authRegisterEndpoint, user)
		assert.NoError(t, err, "Failed to send request")

		assert.Equal(t, response.StatusCode, http.StatusCreated, "Expected status code 201. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
		apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		assert.Equal(t, domain.AuthnUserRegisteredSuccessfully, apiResp.Message, "Expected success message")
		assert.Equal(t, authRegisterEndpoint.method, apiResp.Method, "Expected method to be set")
		assert.Equal(t, apiResp.Path, authRegisterEndpoint.Path(), "Expected path to be set")
		assert.Equal(t, http.StatusCreated, apiResp.StatusCode, "Expected status code 201. Got %d. Message: %s", apiResp.StatusCode, readResponseBody(t, response))

		// wait for the verification email to be sent
		time.Sleep(500 * time.Millisecond)

		// 2. Verify the user
		verificationToken := verificationTokenFromEmail(t, verifyEmailAddress, email)
		assert.NotEmpty(t, verificationToken, "Expected a verification token in the email")

		verificationRawResponse, err := confirmVerification(t, verificationToken)
		assert.NoError(t, err, "Failed to send request")

		assert.Equal(t, http.StatusOK, verificationRawResponse.StatusCode, "Expected status code 200. Got %d. Message: %s", verificationRawResponse.StatusCode, readResponseBody(t, verificationRawResponse))

		verificationResponse, err := parserResponseBody[payload.HTTPMessage](t, verificationRawResponse)
		assert.NoError(t, err, "Failed to parse response body")

		assert.Equal(t, verificationResponse.StatusCode, http.StatusOK, "Expected status code 200.")
		assert.Equal(t, domain.AuthnUserVerifiedSuccessfully, verificationResponse.Message, "Expected verification success message")
		assert.Equal(t, http.MethodPost, verificationResponse.Method, "Expected method to be set")
		assert.Equal(t, authVerifyConfirmEndpoint.Path(), verificationResponse.Path, "Expected path to be set")

		t.Cleanup(func() {
			deleteUserByEmailFromDB(t, email)
		})
	})

	t.Run("test_create_and_verify_user_and_then_reverify", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Register the user
		firstName, lastName, email := generateUserData(t)
		user := map[string]any{
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   generatePassword(t),
		}

		// 1. Register the user
		response, err := sendHTTPRequest(t, ctx, authRegisterEndpoint, user)
		assert.NoError(t, err, "Failed to send request")

		assert.Equal(t, response.StatusCode, http.StatusCreated, "Expected status code 201. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
		apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		assert.Equal(t, domain.AuthnUserRegisteredSuccessfully, apiResp.Message, "Expected success message")
		assert.Equal(t, authRegisterEndpoint.method, apiResp.Method, "Expected method to be set")
		assert.Equal(t, apiResp.Path, authRegisterEndpoint.Path(), "Expected path to be set")
		assert.Equal(t, http.StatusCreated, apiResp.StatusCode, "Expected status code 201. Got %d. Message: %s", apiResp.StatusCode, readResponseBody(t, response))

		// wait for the verification email to be sent
		time.Sleep(500 * time.Millisecond)

		// 2. Verify the user
		verificationToken := verificationTokenFromEmail(t, verifyEmailAddress, email)
		assert.NotEmpty(t, verificationToken, "Expected a verification token in the email")

		verificationRawResponse, err := confirmVerification(t, verificationToken)
		assert.NoError(t, err, "Failed to send request")

		assert.Equal(t, http.StatusOK, verificationRawResponse.StatusCode, "Expected status code 200. Got %d. Message: %s", verificationRawResponse.StatusCode, readResponseBody(t, verificationRawResponse))

		verificationResponse, err := parserResponseBody[payload.HTTPMessage](t, verificationRawResponse)
		assert.NoError(t, err, "Failed to parse response body")

		assert.Equal(t, verificationResponse.StatusCode, http.StatusOK, "Expected status code 200.")
		assert.Equal(t, domain.AuthnUserVerifiedSuccessfully, verificationResponse.Message, "Expected verification success message")
		assert.Equal(t, http.MethodPost, verificationResponse.Method, "Expected method to be set")
		assert.Equal(t, authVerifyConfirmEndpoint.Path(), verificationResponse.Path, "Expected path to be set")

		// 3. Re-Verify the user
		reVerifyPayload := map[string]any{
			"email": email,
		}

		reVerifyResponse, err := sendHTTPRequest(t, ctx, authReVerifyEndpoint, reVerifyPayload)
		assert.NoError(t, err, "Failed to send request")
		assert.Equal(t, reVerifyResponse.StatusCode, http.StatusOK, "Expected status code 200. Got %d. Message: %s", reVerifyResponse.StatusCode, readResponseBody(t, reVerifyResponse))
		defer reVerifyResponse.Body.Close()

		reVerifyAPIResp, err := parserResponseBody[payload.HTTPMessage](t, reVerifyResponse)
		assert.NoError(t, err, "Failed to parse response body")

		assert.Equal(t, reVerifyResponse.StatusCode, http.StatusOK, "Expected status code 200.")
		assert.Equal(t, domain.AuthnUserVerificationEmailSent, reVerifyAPIResp.Message, "Expected verification email sent message")
		assert.Equal(t, authReVerifyEndpoint.method, reVerifyAPIResp.Method)
		assert.Equal(t, authReVerifyEndpoint.Path(), reVerifyAPIResp.Path)

		t.Cleanup(func() {
			deleteUserByEmailFromDB(t, email)
		})
	})
}

func TestAuthLoginUser(t *testing.T) {
	t.Run("test_login_user_with_verification", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		userID := uuid.NewV7()
		firstName, lastName, email := generateUserData(t)
		user := map[string]any{
			"id":         userID.String(),
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   generatePassword(t),
		}

		// t.Logf("User Email: %s, Password: %s", user["email"], user["password"])

		// 1. Register the user
		response, err := sendHTTPRequest(t, ctx, authRegisterEndpoint, user)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		assert.Equal(t, response.StatusCode, http.StatusCreated, "Expected status code 201. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		assert.Equal(t, domain.AuthnUserRegisteredSuccessfully, apiResp.Message, "Expected success message")
		assert.Equal(t, authRegisterEndpoint.method, apiResp.Method, "Expected method to be set")
		assert.Equal(t, authRegisterEndpoint.Path(), apiResp.Path, "Expected path to be set")

		// 2. Verify the user
		// wait for the verification email to be sent
		time.Sleep(500 * time.Millisecond)

		verificationToken := verificationTokenFromEmail(t, verifyEmailAddress, email)
		assert.NotEmpty(t, verificationToken, "Expected a verification token in the email")

		verificationRawResponse, err := confirmVerification(t, verificationToken)
		assert.NoError(t, err, "Failed to send request")
		assert.Equal(t, http.StatusOK, verificationRawResponse.StatusCode, "Expected status code 200. Got %d. Message: %s", verificationRawResponse.StatusCode, readResponseBody(t, verificationRawResponse))

		verificationResponse, err := parserResponseBody[payload.HTTPMessage](t, verificationRawResponse)
		assert.NoError(t, err, "Failed to parse response body")

		assert.Equal(t, http.StatusOK, verificationResponse.StatusCode, "Expected status code 200.")
		assert.Equal(t, domain.AuthnUserVerifiedSuccessfully, verificationResponse.Message, "Expected verification success message")
		assert.Equal(t, http.MethodPost, verificationResponse.Method, "Expected method to be set")
		assert.Equal(t, authVerifyConfirmEndpoint.Path(), verificationResponse.Path, "Expected path to be set")

		// 3. Login the user
		// wait for login verification in the database
		time.Sleep(500 * time.Millisecond)

		loginUser := map[string]any{
			"email":    user["email"],
			"password": user["password"],
		}

		loginResponse, err := sendHTTPRequest(t, ctx, authLoginEndpoint, loginUser)
		assert.NoError(t, err)
		defer loginResponse.Body.Close()
		assert.Equal(t, http.StatusOK, loginResponse.StatusCode, "Expected status code 200. Got %d. Message: %s", loginResponse.StatusCode, readResponseBody(t, loginResponse))

		loginAPIResp, err := parserResponseBody[payload.LoginUserResponse](t, loginResponse)
		assert.NoError(t, err)

		assert.Equal(t, userID, loginAPIResp.UserID, "Expected user ID to be set")
		assert.Equal(t, domain.TokenTypeBearer, loginAPIResp.TokenType, "Expected token type to be Bearer")

		t.Cleanup(func() {
			deleteUserByEmailFromDB(t, email)
		})
	})

	t.Run("test_login_user_without_verification", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		userID := uuid.NewV7()
		firstName, lastName, email := generateUserData(t)
		user := map[string]any{
			"id":         userID.String(),
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   generatePassword(t),
		}

		// t.Logf("User Email: %s, Password: %s", user["email"], user["password"])

		// 1. Register the user
		response, err := sendHTTPRequest(t, ctx, authRegisterEndpoint, user)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		assert.Equal(t, response.StatusCode, http.StatusCreated, "Expected status code 201. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		assert.Equal(t, apiResp.Method, authRegisterEndpoint.method, "Expected method to be set")
		assert.Equal(t, apiResp.Path, authRegisterEndpoint.Path(), "Expected path to be set")

		// 2. Login the user
		// wait for login verification in the database
		time.Sleep(500 * time.Millisecond)

		loginUser := map[string]any{
			"email":    user["email"],
			"password": user["password"],
		}

		loginResponse, err := sendHTTPRequest(t, ctx, authLoginEndpoint, loginUser)
		assert.NoError(t, err)

		defer loginResponse.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, loginResponse.StatusCode, "Expected status code 401. Got %d. Message: %s", loginResponse.StatusCode, readResponseBody(t, loginResponse))

		loginAPIResp, err := parserResponseBody[payload.HTTPMessage](t, loginResponse)
		assert.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, loginAPIResp.StatusCode, "Expected status code 401. Got %d. Message: %s", loginAPIResp.StatusCode, readResponseBody(t, loginResponse))
		assert.Equal(t, loginAPIResp.Path, authLoginEndpoint.Path(), "Expected path to be set")
		assert.Equal(t, loginAPIResp.Method, authLoginEndpoint.method, "Expected method to be set")

		t.Cleanup(func() {
			deleteUserByEmailFromDB(t, email)
		})
	})
}

func TestAuthReVerifyUser(t *testing.T) {
	t.Run("test_register_user_then_delete_it_and_reverify_user", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Register the user
		firstName, lastName, email := generateUserData(t)
		user := map[string]any{
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   generatePassword(t),
		}

		response, err := sendHTTPRequest(t, ctx, authRegisterEndpoint, user)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusCreated, response.StatusCode, "Expected status code 201. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

		apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parse response body")

		assert.Equal(t, domain.AuthnUserRegisteredSuccessfully, apiResp.Message, "Expected success message")
		assert.Equal(t, authRegisterEndpoint.method, apiResp.Method, "Expected method to be set")
		assert.Equal(t, authRegisterEndpoint.Path(), apiResp.Path, "Expected path to be set")

		// 2. Delete the user
		t.Cleanup(func() {
			deleteUserByEmailFromDB(t, email)
		})

		// 3. Re-verify the user
		reVerifyPayload := map[string]any{
			"email": user["email"],
		}

		reVerifyResponse, err := sendHTTPRequest(t, ctx, authReVerifyEndpoint, reVerifyPayload)
		assert.NoError(t, err)

		assert.Equal(t, http.StatusOK, reVerifyResponse.StatusCode, "Expected status code 200. Got %d. Message: %s", reVerifyResponse.StatusCode, readResponseBody(t, reVerifyResponse))
		defer reVerifyResponse.Body.Close()

		reVerifyAPIResp, err := parserResponseBody[payload.HTTPMessage](t, reVerifyResponse)
		assert.NoError(t, err)

		assert.Equal(t, http.StatusOK, reVerifyResponse.StatusCode, "Expected status code 200.")

		assert.Equal(t, domain.AuthnUserVerificationEmailSent, reVerifyAPIResp.Message)
		assert.Equal(t, authReVerifyEndpoint.method, reVerifyAPIResp.Method)
		assert.Equal(t, authReVerifyEndpoint.Path(), reVerifyAPIResp.Path)
	})

	t.Run("test_verify_user_does_not_exist", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		reVerifyPayload := map[string]any{
			"email": "does.notexist@mail.com",
		}

		reVerifyResponse, err := sendHTTPRequest(t, ctx, authReVerifyEndpoint, reVerifyPayload)
		assert.NoError(t, err)

		assert.Equal(t, http.StatusOK, reVerifyResponse.StatusCode, "Expected status code 404. Got %d. Message: %s", reVerifyResponse.StatusCode, readResponseBody(t, reVerifyResponse))

		defer reVerifyResponse.Body.Close()

		reVerifyAPIResp, err := parserResponseBody[payload.HTTPMessage](t, reVerifyResponse)
		assert.NoError(t, err)

		assert.Equal(t, http.StatusOK, reVerifyResponse.StatusCode, "Expected status code 200.")

		assert.Equal(t, domain.AuthnUserVerificationEmailSent, reVerifyAPIResp.Message)
		assert.Equal(t, authReVerifyEndpoint.method, reVerifyAPIResp.Method)
		assert.Equal(t, authReVerifyEndpoint.Path(), reVerifyAPIResp.Path)
	})
}

func TestAuthRefreshTokens(t *testing.T) {
	t.Run("test_refresh_token", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// 1. Register the user
		userID := uuid.NewV7()
		firstName, lastName, email := generateUserData(t)
		user := map[string]any{
			"id":         userID.String(),
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   generatePassword(t),
		}

		// 1. Register the user
		response, err := sendHTTPRequest(t, ctx, authRegisterEndpoint, user)
		assert.NoError(t, err, "Failed to send request")

		assert.Equal(t, response.StatusCode, http.StatusCreated, "Expected status code 201. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
		apiResp, err := parserResponseBody[payload.HTTPMessage](t, response)
		assert.NoError(t, err, "Failed to parser response body")

		assert.Equal(t, domain.AuthnUserRegisteredSuccessfully, apiResp.Message, "Expected success message")
		assert.Equal(t, authRegisterEndpoint.method, apiResp.Method, "Expected method to be set")
		assert.Equal(t, apiResp.Path, authRegisterEndpoint.Path(), "Expected path to be set")
		assert.Equal(t, http.StatusCreated, apiResp.StatusCode, "Expected status code 201. Got %d. Message: %s", apiResp.StatusCode, readResponseBody(t, response))

		// wait for the verification email to be sent
		time.Sleep(500 * time.Millisecond)

		// 2. Verify the user
		verificationToken := verificationTokenFromEmail(t, verifyEmailAddress, email)
		assert.NotEmpty(t, verificationToken, "Expected a verification token in the email")

		verificationRawResponse, err := confirmVerification(t, verificationToken)
		assert.NoError(t, err, "Failed to send request")

		assert.Equal(t, http.StatusOK, verificationRawResponse.StatusCode, "Expected status code 200. Got %d. Message: %s", verificationRawResponse.StatusCode, readResponseBody(t, verificationRawResponse))

		verificationResponse, err := parserResponseBody[payload.HTTPMessage](t, verificationRawResponse)
		assert.NoError(t, err, "Failed to parse response body")

		assert.Equal(t, verificationResponse.StatusCode, http.StatusOK, "Expected status code 200.")
		assert.Equal(t, domain.AuthnUserVerifiedSuccessfully, verificationResponse.Message, "Expected verification success message")
		assert.Equal(t, http.MethodPost, verificationResponse.Method, "Expected method to be set")
		assert.Equal(t, authVerifyConfirmEndpoint.Path(), verificationResponse.Path, "Expected path to be set")

		// 3. Login the user
		// wait for login verification in the database
		time.Sleep(500 * time.Millisecond)

		loginUser := map[string]any{
			"email":    user["email"],
			"password": user["password"],
		}

		loginResponse, err := sendHTTPRequest(t, ctx, authLoginEndpoint, loginUser)
		assert.NoError(t, err)
		defer loginResponse.Body.Close()

		assert.Equal(t, http.StatusOK, loginResponse.StatusCode, "Expected status code 200. Got %d. Message: %s", loginResponse.StatusCode, readResponseBody(t, loginResponse))
		loginAPIResp, err := parserResponseBody[payload.LoginUserResponse](t, loginResponse)
		assert.NoError(t, err)

		assert.Equal(t, user["id"], loginAPIResp.UserID.String(), "Expected user ID to be set")
		assert.Equal(t, domain.TokenTypeBearer, loginAPIResp.TokenType, "Expected token type to be Bearer")
		assert.NotEmpty(t, loginAPIResp.AccessToken, "Expected access token to be set")
		assert.NotEmpty(t, loginAPIResp.RefreshToken, "Expected refresh token to be set")

		// 4. Assign permissions to the user for POST /auth/refresh

		// 5. Refresh the token
		refreshTokenPayload := map[string]any{
			"refresh_token": loginAPIResp.RefreshToken,
		}

		// user the refresh token to get a new access token
		refreshTokenHeader := map[string]string{
			"Authorization": "Bearer " + loginAPIResp.RefreshToken,
		}

		refreshResponse, err := sendHTTPRequest(t, ctx, authRefreshEndpoint, refreshTokenPayload, refreshTokenHeader)
		assert.NoError(t, err)
		defer refreshResponse.Body.Close()

		assert.Equal(t, http.StatusOK, refreshResponse.StatusCode, "Expected status code 200. Got %d. Message: %s", refreshResponse.StatusCode, readResponseBody(t, refreshResponse))

		refreshAPIResp, err := parserResponseBody[payload.RefreshTokenResponse](t, refreshResponse)
		assert.NoError(t, err)

		assert.NotEmpty(t, refreshAPIResp.AccessToken, "Expected access token to be set")
		assert.NotEmpty(t, refreshAPIResp.TokenType, "Expected token type to be set")

		t.Cleanup(func() {
			deleteUserByEmailFromDB(t, email)
		})
	})
}

// TestAuthRegisterUser_EdgeCases tests registration with various edge cases
func TestAuthRegisterUser_EdgeCases(t *testing.T) {
	t.Run("invalid_email_format", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		firstName, lastName, _ := generateUserData(t)
		user := map[string]any{
			"email":      "invalid-email",
			"first_name": firstName,
			"last_name":  lastName,
			"password":   generatePassword(t),
		}

		response, err := sendHTTPRequest(t, ctx, authRegisterEndpoint, user)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusBadRequest, response.StatusCode, "Expected status code 400. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})

	t.Run("missing_required_fields", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		testCases := []struct {
			name    string
			payload map[string]any
		}{
			{
				name: "missing_email",
				payload: map[string]any{
					"first_name": "John",
					"last_name":  "Doe",
					"password":   generatePassword(t),
				},
			},
			{
				name: "missing_first_name",
				payload: map[string]any{
					"email":     "test@example.com",
					"last_name": "Doe",
					"password":  generatePassword(t),
				},
			},
			{
				name: "missing_password",
				payload: map[string]any{
					"email":      "test@example.com",
					"first_name": "John",
					"last_name":  "Doe",
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				response, err := sendHTTPRequest(t, ctx, authRegisterEndpoint, tc.payload)
				assert.NoError(t, err, "Failed to send request")
				defer response.Body.Close()

				assert.Equal(t, http.StatusBadRequest, response.StatusCode, "Expected status code 400. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
			})
		}
	})

	t.Run("weak_password", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		firstName, lastName, email := generateUserData(t)
		user := map[string]any{
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   "123", // weak password
		}

		response, err := sendHTTPRequest(t, ctx, authRegisterEndpoint, user)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusBadRequest, response.StatusCode, "Expected status code 400. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})

	t.Run("name_too_long", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		_, _, email := generateUserData(t)
		user := map[string]any{
			"email":      email,
			"first_name": strings.Repeat("a", 26), // exceeds max length of 25
			"last_name":  "Doe",
			"password":   generatePassword(t),
		}

		response, err := sendHTTPRequest(t, ctx, authRegisterEndpoint, user)
		assert.NoError(t, err, "Failed to send request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusBadRequest, response.StatusCode, "Expected status code 400. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	})
}

// TestAuthLoginUser_EdgeCases tests login with various edge cases
func TestAuthLoginUser_EdgeCases(t *testing.T) {
	t.Run("invalid_credentials", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// Create and verify a user first
		userID := uuid.NewV7()
		firstName, lastName, email := generateUserData(t)
		password := generatePassword(t)
		user := map[string]any{
			"id":         userID.String(),
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   password,
		}

		response, err := sendHTTPRequest(t, ctx, authRegisterEndpoint, user)
		assert.NoError(t, err)
		defer response.Body.Close()
		assert.Equal(t, http.StatusCreated, response.StatusCode)

		time.Sleep(500 * time.Millisecond)
		verificationToken := verificationTokenFromEmail(t, verifyEmailAddress, email)
		_, err = confirmVerification(t, verificationToken)
		assert.NoError(t, err)

		time.Sleep(500 * time.Millisecond)

		// Try to login with wrong password
		loginUser := map[string]any{
			"email":    email,
			"password": "wrongpassword123!",
		}

		loginResponse, err := sendHTTPRequest(t, ctx, authLoginEndpoint, loginUser)
		assert.NoError(t, err)
		defer loginResponse.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, loginResponse.StatusCode, "Expected status code 401. Got %d. Message: %s", loginResponse.StatusCode, readResponseBody(t, loginResponse))

		t.Cleanup(func() {
			deleteUserByEmailFromDB(t, email)
		})
	})

	t.Run("non_existent_user", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		loginUser := map[string]any{
			"email":    "nonexistent@example.com",
			"password": generatePassword(t),
		}

		loginResponse, err := sendHTTPRequest(t, ctx, authLoginEndpoint, loginUser)
		assert.NoError(t, err)
		defer loginResponse.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, loginResponse.StatusCode, "Expected status code 401. Got %d. Message: %s", loginResponse.StatusCode, readResponseBody(t, loginResponse))
	})

	t.Run("missing_credentials", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		testCases := []struct {
			name    string
			payload map[string]any
		}{
			{
				name:    "missing_email",
				payload: map[string]any{"password": generatePassword(t)},
			},
			{
				name:    "missing_password",
				payload: map[string]any{"email": "test@example.com"},
			},
			{
				name:    "empty_credentials",
				payload: map[string]any{},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				response, err := sendHTTPRequest(t, ctx, authLoginEndpoint, tc.payload)
				assert.NoError(t, err)
				defer response.Body.Close()

				assert.Equal(t, http.StatusBadRequest, response.StatusCode, "Expected status code 400. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
			})
		}
	})
}

// TestAuthLogout tests the logout endpoint
func TestAuthLogout(t *testing.T) {
	t.Run("logout_successfully", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// Create, verify, and login a user
		userID := uuid.NewV7()
		firstName, lastName, email := generateUserData(t)
		user := map[string]any{
			"id":         userID.String(),
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   generatePassword(t),
		}

		response, err := sendHTTPRequest(t, ctx, authRegisterEndpoint, user)
		assert.NoError(t, err)
		defer response.Body.Close()
		assert.Equal(t, http.StatusCreated, response.StatusCode)

		time.Sleep(500 * time.Millisecond)
		verificationToken := verificationTokenFromEmail(t, verifyEmailAddress, email)
		_, err = confirmVerification(t, verificationToken)
		assert.NoError(t, err)

		time.Sleep(500 * time.Millisecond)

		loginUser := map[string]any{
			"email":    user["email"],
			"password": user["password"],
		}

		loginResponse, err := sendHTTPRequest(t, ctx, authLoginEndpoint, loginUser)
		assert.NoError(t, err)
		defer loginResponse.Body.Close()
		assert.Equal(t, http.StatusOK, loginResponse.StatusCode)

		loginAPIResp, err := parserResponseBody[payload.LoginUserResponse](t, loginResponse)
		assert.NoError(t, err)

		// Logout using access token
		logoutHeader := map[string]string{
			"Authorization": "Bearer " + loginAPIResp.AccessToken,
		}

		logoutResponse, err := sendHTTPRequest(t, ctx, authLogoutEndpoint, nil, logoutHeader)
		assert.NoError(t, err)
		defer logoutResponse.Body.Close()

		assert.Equal(t, http.StatusOK, logoutResponse.StatusCode, "Expected status code 200. Got %d. Message: %s", logoutResponse.StatusCode, readResponseBody(t, logoutResponse))

		logoutAPIResp, err := parserResponseBody[payload.HTTPMessage](t, logoutResponse)
		assert.NoError(t, err)
		assert.Equal(t, domain.AuthnUserLoggedOutSuccessfully, logoutAPIResp.Message)

		t.Cleanup(func() {
			deleteUserByEmailFromDB(t, email)
		})
	})

	t.Run("logout_without_authorization", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		logoutResponse, err := sendHTTPRequest(t, ctx, authLogoutEndpoint, nil)
		assert.NoError(t, err)
		defer logoutResponse.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, logoutResponse.StatusCode, "Expected status code 401. Got %d. Message: %s", logoutResponse.StatusCode, readResponseBody(t, logoutResponse))
	})

	t.Run("logout_with_invalid_token", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		logoutHeader := map[string]string{
			"Authorization": "Bearer invalid.token.here",
		}

		logoutResponse, err := sendHTTPRequest(t, ctx, authLogoutEndpoint, nil, logoutHeader)
		assert.NoError(t, err)
		defer logoutResponse.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, logoutResponse.StatusCode, "Expected status code 401. Got %d. Message: %s", logoutResponse.StatusCode, readResponseBody(t, logoutResponse))
	})
}

// TestAuthRefreshToken_EdgeCases tests refresh token with edge cases
func TestAuthRefreshToken_EdgeCases(t *testing.T) {
	t.Run("refresh_with_invalid_token", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		refreshTokenPayload := map[string]any{
			"refresh_token": "invalid.token.here",
		}

		refreshTokenHeader := map[string]string{
			"Authorization": "Bearer invalid.token.here",
		}

		refreshResponse, err := sendHTTPRequest(t, ctx, authRefreshEndpoint, refreshTokenPayload, refreshTokenHeader)
		assert.NoError(t, err)
		defer refreshResponse.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, refreshResponse.StatusCode, "Expected status code 401. Got %d. Message: %s", refreshResponse.StatusCode, readResponseBody(t, refreshResponse))
	})

	t.Run("refresh_without_authorization_header", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		refreshTokenPayload := map[string]any{
			"refresh_token": "some.token.here",
		}

		refreshResponse, err := sendHTTPRequest(t, ctx, authRefreshEndpoint, refreshTokenPayload)
		assert.NoError(t, err)
		defer refreshResponse.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, refreshResponse.StatusCode, "Expected status code 401. Got %d. Message: %s", refreshResponse.StatusCode, readResponseBody(t, refreshResponse))
	})

	t.Run("refresh_with_access_token_instead", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// Create, verify, and login a user
		userID := uuid.NewV7()
		firstName, lastName, email := generateUserData(t)
		user := map[string]any{
			"id":         userID.String(),
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   generatePassword(t),
		}

		response, err := sendHTTPRequest(t, ctx, authRegisterEndpoint, user)
		assert.NoError(t, err)
		defer response.Body.Close()
		assert.Equal(t, http.StatusCreated, response.StatusCode)

		time.Sleep(500 * time.Millisecond)
		verificationToken := verificationTokenFromEmail(t, verifyEmailAddress, email)
		_, err = confirmVerification(t, verificationToken)
		assert.NoError(t, err)

		time.Sleep(500 * time.Millisecond)

		loginUser := map[string]any{
			"email":    user["email"],
			"password": user["password"],
		}

		loginResponse, err := sendHTTPRequest(t, ctx, authLoginEndpoint, loginUser)
		assert.NoError(t, err)
		defer loginResponse.Body.Close()
		assert.Equal(t, http.StatusOK, loginResponse.StatusCode)

		loginAPIResp, err := parserResponseBody[payload.LoginUserResponse](t, loginResponse)
		assert.NoError(t, err)

		// Try to refresh using access token instead of refresh token
		refreshTokenPayload := map[string]any{
			"refresh_token": loginAPIResp.AccessToken,
		}

		refreshTokenHeader := map[string]string{
			"Authorization": "Bearer " + loginAPIResp.AccessToken,
		}

		refreshResponse, err := sendHTTPRequest(t, ctx, authRefreshEndpoint, refreshTokenPayload, refreshTokenHeader)
		assert.NoError(t, err)
		defer refreshResponse.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, refreshResponse.StatusCode, "Expected status code 401. Got %d. Message: %s", refreshResponse.StatusCode, readResponseBody(t, refreshResponse))

		t.Cleanup(func() {
			deleteUserByEmailFromDB(t, email)
		})
	})
}

// TestAuthVerify_EdgeCases tests verification with edge cases
func TestAuthVerify_EdgeCases(t *testing.T) {
	t.Run("verify_with_invalid_token", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// Try to verify with an invalid token. It goes in the header now: the
		// path form leaked it into the request log.
		verifyResponse, err := sendHTTPRequest(t, ctx, authVerifyConfirmEndpoint, nil,
			map[string]string{"Authorization": "Bearer invalid.token.here"})
		assert.NoError(t, err)
		defer verifyResponse.Body.Close()

		// The auth middleware refuses it before the handler runs, so an
		// unreadable token is a 401 rather than the 400 the handler used to
		// answer after parsing the path itself.
		assert.Equal(t, http.StatusUnauthorized, verifyResponse.StatusCode, "Expected status code 401. Got %d. Message: %s", verifyResponse.StatusCode, readResponseBody(t, verifyResponse))
	})

	t.Run("verify_already_verified_user", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// Create and verify a user
		userID := uuid.NewV7()
		firstName, lastName, email := generateUserData(t)
		user := map[string]any{
			"id":         userID.String(),
			"email":      email,
			"first_name": firstName,
			"last_name":  lastName,
			"password":   generatePassword(t),
		}

		response, err := sendHTTPRequest(t, ctx, authRegisterEndpoint, user)
		assert.NoError(t, err)
		defer response.Body.Close()
		assert.Equal(t, http.StatusCreated, response.StatusCode)

		time.Sleep(500 * time.Millisecond)
		verificationToken := verificationTokenFromEmail(t, verifyEmailAddress, email)
		assert.NotEmpty(t, verificationToken)

		// First verification
		verificationResponse1, err := confirmVerification(t, verificationToken)
		assert.NoError(t, err)
		defer verificationResponse1.Body.Close()
		assert.Equal(t, http.StatusOK, verificationResponse1.StatusCode)

		// Try to verify again with the same token
		verificationResponse2, err := confirmVerification(t, verificationToken)
		assert.NoError(t, err)
		defer verificationResponse2.Body.Close()

		// The link is single-use: the second click presents a spent token and is
		// refused as an invalid token, before the account is even looked at.
		// It used to answer with the account's state ("already verified"),
		// which the token then kept doing until it expired.
		assert.Equal(t, http.StatusUnauthorized, verificationResponse2.StatusCode, "Expected 401 for reusing a verification token. Got %d. Message: %s", verificationResponse2.StatusCode, readResponseBody(t, verificationResponse2))

		apiResp, err := parserResponseBody[payload.HTTPMessage](t, verificationResponse2)
		assert.NoError(t, err, "Failed to parse response body")
		assert.NotContains(t, apiResp.Message, "already verified", "a spent token must not report the account's state")

		t.Cleanup(func() {
			deleteUserByEmailFromDB(t, email)
		})
	})
}
