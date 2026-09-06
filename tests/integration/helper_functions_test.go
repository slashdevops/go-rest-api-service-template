//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/usecase"
)

const (
	defaultOllamaTagsURL = "http://localhost:11434/api/tags"
)

// apiEndpoint is a struct to hold endpoint information
// It contains the endpoint URL, path, and HTTP method
type apiEndpoint struct {
	apiURL     *url.URL
	requestURL *url.URL
	path       string
	method     string
}

func (e *apiEndpoint) String() string {
	return e.requestURL.Path
}

func (e *apiEndpoint) Path() string {
	return e.path
}

// Clone creates a new apiEndpoint with the same method and path
func (e *apiEndpoint) Clone() *apiEndpoint {
	return newAPIEndpoint(e.method, e.path)
}

// RewriteSlugs clones the apiEndpoint and replaces the slugs in the path
// with the provided slugs.
func (e *apiEndpoint) RewriteSlugs(slugs ...string) *apiEndpoint {
	if len(slugs) != 0 {
		// Clone the apiEndpoint
		eClone := newAPIEndpoint(e.method, e.path)

		eClone.path = replaceSlugs(e.path, slugs...)
		apiEndpointURL, err := url.Parse(apiEndpointURL)
		if err != nil {
			panic(fmt.Sprintf("❌ Failed to parse API endpoint URL: %v", err))
		}

		requestURL, err := url.Parse(apiEndpointURL.String() + eClone.path)
		if err != nil {
			panic(fmt.Sprintf("❌ Failed to parse request URL: %v", err))
		}

		eClone.requestURL = requestURL

		return eClone
	}

	// If no slugs are provided, return the original apiEndpoint
	return e
}

func (e *apiEndpoint) SetQueryParam(key, value string) {
	if e.requestURL == nil {
		panic("❌ requestURL is nil")
	}

	query := e.requestURL.Query()
	query.Set(key, value)
	e.requestURL.RawQuery = query.Encode()
}

func (e *apiEndpoint) SetQueryParams(params map[string]string) {
	if e.requestURL == nil {
		panic("❌ requestURL is nil")
	}

	query := e.requestURL.Query()
	for key, value := range params {
		query.Set(key, value)
	}
	e.requestURL.RawQuery = query.Encode()
}

// newAPIEndpoint is a helper function to create a new API endpoint
// It takes the HTTP method and path as parameters
func newAPIEndpoint(method, path string) *apiEndpoint {
	apiEndpointURL, err := url.Parse(apiEndpointURL)
	if err != nil {
		panic(fmt.Sprintf("❌ Failed to parse API endpoint URL: %v", err))
	}

	requestURL, err := url.Parse(apiEndpointURL.String() + path)
	if err != nil {
		panic(fmt.Sprintf("❌ Failed to parse request URL: %v", err))
	}

	return &apiEndpoint{
		apiURL:     apiEndpointURL,
		requestURL: requestURL,
		path:       path,
		method:     method,
	}
}

func newMailAPIEndpoint(method, path string) *apiEndpoint {
	apiEndpointURL, err := url.Parse(mailServerEndpointURL)
	if err != nil {
		panic(fmt.Sprintf("❌ Failed to parse API endpoint URL: %v", err))
	}

	requestURL, err := url.Parse(apiEndpointURL.String() + path)
	if err != nil {
		panic(fmt.Sprintf("❌ Failed to parse request URL: %v", err))
	}

	return &apiEndpoint{
		apiURL:     apiEndpointURL,
		requestURL: requestURL,
		method:     method,
	}
}

// sendHTTPRequest is a helper function to send HTTP requests
// It takes a testing.T object, an apiEndpoint object, and a request body
// It returns the HTTP response and an error if any
// It uses the default HTTP client to send the request
// It marshals the request body to JSON if provided
func sendHTTPRequest(t *testing.T, ctx context.Context, endpoint *apiEndpoint, body map[string]any, headers ...map[string]string) (*http.Response, error) {
	if t == nil {
		t = &testing.T{}
	}
	t.Helper()

	client := http.DefaultClient

	var jsonBody io.ReadWriter
	var err error
	if body != nil {
		jsonBody = new(bytes.Buffer)
		enc := json.NewEncoder(jsonBody)
		enc.SetEscapeHTML(false)
		err = enc.Encode(body)
		if err != nil {
			t.Errorf("Failed to encode request body: %v", err)
			return nil, err
		}

	}

	// t.Logf("Sending %s request to %s with body: %v", endpoint.method, endpoint.requestURL.String(), body)
	req, err := http.NewRequestWithContext(ctx, endpoint.method, endpoint.requestURL.String(), jsonBody)
	if err != nil {
		return nil, err
	}

	// obligatory headers
	req.Header.Set("Accept", "application/json")
	// The API refuses a body that is not declared as JSON (415), and every
	// body this helper sends is JSON.
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// optional headers
	if len(headers) > 0 {
		for key, value := range headers[0] {
			req.Header.Set(key, value)
		}
	}

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		t.Errorf("Failed to send request: %v", err)
		return nil, err
	}

	if resp == nil {
		err := fmt.Errorf("received a nil response")
		t.Errorf("Failed to get a response: %v", err)
		return nil, err
	}

	// Log the endpoint and response details
	t.Logf("HTTP Request: %s %s", endpoint.method, endpoint.requestURL.String())

	// Read and log the response body
	respBody, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()

	if readErr != nil {
		t.Logf("HTTP Response: Status=%d (failed to read body: %v)", resp.StatusCode, readErr)
	} else {
		t.Logf("HTTP Response: Status=%d, Body=%s", resp.StatusCode, string(respBody))
		// Restore the response body for further processing
		resp.Body = io.NopCloser(bytes.NewBuffer(respBody))
	}

	return resp, nil
}

// waitFor polls the given predicate until it returns true or the
// deadline elapses, with a small back-off between attempts. It is
// the preferred replacement for `time.Sleep(500 * time.Millisecond)`
// waits on external-state convergence (mail delivery, DB eventual
// consistency, etc.): faster in the happy path, clearer error on
// timeout. The msg is shown verbatim on failure.
//
// Example:
//
//	waitFor(t, 5*time.Second, "verification email arrived", func() bool {
//	    return countEmailsForRecipient(t, addr) >= 1
//	})
func waitFor(t *testing.T, timeout time.Duration, msg string, probe func() bool) {
	t.Helper()
	const minInterval = 25 * time.Millisecond
	const maxInterval = 250 * time.Millisecond

	deadline := time.Now().Add(timeout)
	interval := minInterval
	for {
		if probe() {
			return
		}
		if !time.Now().Before(deadline) {
			t.Fatalf("waitFor: %s — predicate did not become true within %s", msg, timeout)
		}
		time.Sleep(interval)
		// Exponential back-off, capped, so a slow predicate doesn't busy-loop.
		if interval = interval * 2; interval > maxInterval {
			interval = maxInterval
		}
	}
}

// requireOllamaAvailable skips the test when a local Ollama instance is not reachable.
// This keeps regular integration runs stable on environments without Ollama.
func requireOllamaAvailable(t *testing.T) {
	t.Helper()

	ollamaTagsURL := os.Getenv("OLLAMA_TAGS_URL")
	if strings.TrimSpace(ollamaTagsURL) == "" {
		ollamaTagsURL = defaultOllamaTagsURL
	}

	client := &http.Client{Timeout: 1500 * time.Millisecond}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ollamaTagsURL, nil)
	if err != nil {
		t.Skipf("Skipping Ollama-dependent test: invalid Ollama URL %q: %v", ollamaTagsURL, err)
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Skipf("Skipping Ollama-dependent test: Ollama API is not reachable at %s: %v", ollamaTagsURL, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Skipf("Skipping Ollama-dependent test: expected Ollama API status 200 at %s, got %d", ollamaTagsURL, resp.StatusCode)
	}
}

// parserResponseBody is a helper function generic to parse the response body
// It takes an HTTP response and a pointer to a struct to unmarshal the response into
// It returns the unmarshaled struct and an error if any
func parserResponseBody[T any](t *testing.T, resp *http.Response) (T, error) {
	if t == nil {
		t = &testing.T{}
	}
	t.Helper()

	var result T
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return result, err
	}
	defer resp.Body.Close()

	// t.Logf("Response: %+v", result)

	return result, nil
}

// generatePassword generates a random password of the specified length
// It uses the crypto/rand package to generate a secure random password
// It returns the generated password as a string
// generatePassword returns a password the API will actually accept.
//
// It draws at least one character from each class the password policy requires
// and shuffles the result. Drawing uniformly from the whole charset, which is
// what this did, is not good enough: with 12 characters from a 74-character set
// the chance of producing no digit is 17%, no special character 12%, and the
// chance of missing at least one required class is about 30%.
//
// That was a one-in-three chance of a spurious failure in EVERY test that logs
// in with a generated password — including tests whose subject has nothing to
// do with authentication. It surfaced as
//
//	POST /auth/login -> 400 "password must contain a mix of uppercase,
//	                         lowercase, numbers, and special characters"
//
// in a run of the whole suite, and passed on the next three runs of the same
// test. A suite that fails at random cannot be used to tell a regression from
// noise, which is the only reason it is worth running at all.
func generatePassword(t *testing.T, length ...int) string {
	t.Helper()

	classes := []string{
		"abcdefghijklmnopqrstuvwxyz",
		"ABCDEFGHIJKLMNOPQRSTUVWXYZ",
		"0123456789",
		"!@#$%^&*()_+",
	}

	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()_+"

	if len(length) == 0 {
		length = append(length, 12) // Default length
	}

	if length[0] < len(classes) {
		t.Fatalf("generatePassword: length %d cannot hold one character from each of the %d required classes", length[0], len(classes))
	}

	raw := make([]byte, length[0])
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		t.Fatalf("Failed to generate password: %v", err)
		return ""
	}

	password := make([]byte, length[0])

	// One guaranteed character per required class, then fill the rest freely.
	for i, class := range classes {
		password[i] = class[int(raw[i])%len(class)]
	}

	for i := len(classes); i < len(password); i++ {
		password[i] = charset[int(raw[i])%len(charset)]
	}

	// Shuffle, so the class of each position is not fixed. Fisher-Yates with
	// its own randomness rather than reusing raw, whose bytes are already
	// spoken for.
	swaps := make([]byte, len(password))
	if _, err := io.ReadFull(rand.Reader, swaps); err != nil {
		t.Fatalf("Failed to generate password: %v", err)
		return ""
	}

	for i := len(password) - 1; i > 0; i-- {
		j := int(swaps[i]) % (i + 1)
		password[i], password[j] = password[j], password[i]
	}

	return string(password)
}

// Helper function to generate UUID string for test data
func mustUUIDString(t *testing.T) string {
	t.Helper()
	id := uuid.NewV7()
	return id.String()
}

// generateRandomName generates a random name
// It uses a charset = "abcdefghijklmnopqrstuvwxyz"
// and the name has a dynamic length between 3 and 10 characters
func generateRandomName(t *testing.T, prefix string) string {
	t.Helper()

	const charset = "abcdefghijklmnopqrstuvwxyz"

	// Generate random length between 3 and 10
	randomBytes := make([]byte, 1)
	_, err := rand.Read(randomBytes)
	if err != nil {
		t.Fatalf("Failed to generate random bytes: %v", err)
		return ""
	}
	nameLength := 3 + int(randomBytes[0]%8)
	name := make([]byte, nameLength)

	_, err = io.ReadFull(rand.Reader, name)
	if err != nil {
		t.Fatalf("Failed to generate name: %v", err)
		return ""
	}

	for i := range name {
		name[i] = charset[int(name[i])%len(charset)]
	}

	if prefix != "" {
		prefix += "_"
	}

	return fmt.Sprintf("%s%s", prefix, string(name))
}

// generateUserData generates a random email address
// It uses a charset = "abcdefghijklmnopqrstuvwxyz"
// and the email has the patter word1.word2@<mailDomain>
// the mailDomain is an optional parameter and the size of the word1 and word2 is
// dynamic between 3 and 10 characters
func generateUserData(t *testing.T, mailDomain ...string) (firstName, lastName, email string) {
	t.Helper()

	const charset = "abcdefghijklmnopqrstuvwxyz"

	// if mailDomain is not provided, use a default domain
	if len(mailDomain) == 0 {
		mailDomain = append(mailDomain, "mail.com")
	}

	// Generate random length between 3 and 10
	randomBytes := make([]byte, 2)
	_, err := rand.Read(randomBytes)
	if err != nil {
		t.Fatalf("Failed to generate random bytes: %v", err)
		return "", "", ""
	}

	word1Length := 3 + int(randomBytes[0]%8)
	word2Length := 3 + int(randomBytes[1]%8)

	word1 := make([]byte, word1Length)
	word2 := make([]byte, word2Length)

	_, err = io.ReadFull(rand.Reader, word1)
	if err != nil {
		t.Fatalf("Failed to generate email: %v", err)
	}

	_, err = io.ReadFull(rand.Reader, word2)
	if err != nil {
		t.Fatalf("Failed to generate email: %v", err)
		return "", "", ""
	}

	for i := range word1 {
		word1[i] = charset[int(word1[i])%len(charset)]
	}
	for i := range word2 {
		word2[i] = charset[int(word2[i])%len(charset)]
	}

	firstName = string(word1)
	lastName = string(word2)
	email = fmt.Sprintf("%s.%s@%s", string(word1), string(word2), mailDomain[0])

	// Check if the email is valid
	_, err = mail.ParseAddress(email)
	if err != nil {
		t.Fatalf("Invalid email address: %v", err)
		return "", "", ""
	}

	return firstName, lastName, email
}

// getVerifyLinkFromEmail is a helper function to get the verification link from the email
// It takes a testing.T object, the sender's email address, and the recipient's email address
func getVerifyLinkFromEmail(t *testing.T, from, to string) string {
	mailSearchEndpoint := newMailAPIEndpoint(http.MethodGet, "/search")
	apiQueryParam := "query="
	apiQuery := fmt.Sprintf("From:%s To:%s", from, to)
	mailSearchEndpoint.requestURL.RawQuery = apiQueryParam + url.QueryEscape(apiQuery)

	resp, err := sendHTTPRequest(t, context.Background(), mailSearchEndpoint, nil)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status code 200, got %d", resp.StatusCode)
	}

	type searchResponse struct {
		Messages []struct {
			ID string `json:"ID"`
		} `json:"Messages"`
	}

	mailResponse, err := parserResponseBody[searchResponse](t, resp)
	if err != nil {
		t.Fatalf("Failed to parse response body: %v", err)
	}

	if len(mailResponse.Messages) == 0 {
		t.Fatalf("No emails found for From: %s, To: %s", from, to)
	}

	mailID := mailResponse.Messages[0].ID
	mailGetMessageSourceEndpoint := newMailAPIEndpoint(http.MethodGet, fmt.Sprintf("/message/%s/raw", mailID))

	rawContentResp, err := sendHTTPRequest(t, context.Background(), mailGetMessageSourceEndpoint, nil)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer rawContentResp.Body.Close()

	if rawContentResp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status code 200, got %d", rawContentResp.StatusCode)
	}

	rawContent, err := io.ReadAll(rawContentResp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	// The token is a query parameter, not a path segment: as a path segment it
	// was written into the API's own request log, and into the browser history
	// and Referer of whoever clicked the link.
	re := regexp.MustCompile(`http:\/\/[^\s]+\/verify\?token=[a-zA-Z0-9._-]+`)
	matches := re.FindStringSubmatch(string(rawContent))
	if len(matches) < 1 {
		t.Fatalf("No verification link found in the email content")
	}
	verifyLink := matches[0]

	// Check if the verification link is valid
	_, err = url.ParseRequestURI(verifyLink)
	if err != nil {
		t.Fatalf("Invalid verification link: %v", err)
	}

	return verifyLink
}

// verificationTokenFromEmail pulls the verification token out of the link the
// email carries.
//
// The link points at the FRONTEND, which is not running in the integration
// environment, so a test cannot simply follow it the way it used to when the
// link pointed straight at this API. It takes the token the page would have
// taken, and hands it over the way the page hands it over: in a header.
func verificationTokenFromEmail(t *testing.T, from, to string) string {
	t.Helper()

	link := getVerifyLinkFromEmail(t, from, to)

	parsed, err := url.Parse(link)
	if err != nil {
		t.Fatalf("Invalid verification link %q: %v", link, err)
	}

	token := parsed.Query().Get("token")
	if token == "" {
		t.Fatalf("No token query parameter in the verification link %q", link)
	}

	// The whole point of the change: the token must not be in the path.
	if strings.Contains(parsed.Path, token) {
		t.Fatalf("The verification token is in the URL path, where it reaches logs: %q", link)
	}

	return token
}

// confirmVerification presents a verification token the way the frontend does.
func confirmVerification(t *testing.T, token string) (*http.Response, error) {
	t.Helper()

	return sendHTTPRequest(t, context.Background(), authVerifyConfirmEndpoint, nil,
		map[string]string{"Authorization": "Bearer " + token})
}

// deleteAllEmails is a helper function to delete all emails from the mail server
// It takes a testing.T object and the sender's email address
// It returns an error if any
func deleteAllEmails() error {
	mailListEndpoint := newMailAPIEndpoint(http.MethodGet, "/messages")

	listResp, err := sendHTTPRequest(nil, context.Background(), mailListEndpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer listResp.Body.Close()

	if listResp.StatusCode != http.StatusOK {
		return fmt.Errorf("expected status code 200, got %d", listResp.StatusCode)
	}

	type listResponse struct {
		Messages []struct {
			ID string `json:"ID"`
		} `json:"Messages"`
	}

	mailListResponse, err := parserResponseBody[listResponse](nil, listResp)
	if err != nil {
		return fmt.Errorf("failed to parse response body: %v", err)
	}

	mailDeletePayload := make(map[string]any)
	mailIDsToDelete := make([]string, len(mailListResponse.Messages))

	for i, message := range mailListResponse.Messages {
		mailIDsToDelete[i] = message.ID
	}

	mailDeletePayload["IDs"] = strings.Join(mailIDsToDelete, ",")

	mailDeleteEndpoint := newMailAPIEndpoint(http.MethodDelete, "/messages")

	deleteResp, err := sendHTTPRequest(nil, context.Background(), mailDeleteEndpoint, mailDeletePayload)
	if err != nil {
		return fmt.Errorf("failed to delete emails: %v", err)
	}
	defer deleteResp.Body.Close()

	if deleteResp.StatusCode != http.StatusOK {
		return fmt.Errorf("expected status code 200, got %d", deleteResp.StatusCode)
	}

	return nil
}

// createUserInDB is a helper function to create a user in the database
// It takes a testing.T object, first name, last name, email, and password
// It returns the user ID and an error if any
func createUserInDB(t *testing.T, firstName, lastName, email, password string) uuid.UUID {
	t.Helper()

	query := `
        INSERT INTO users (id, first_name, last_name, email, password_hash, disabled)
        VALUES ($1, $2, $3, $4, $5, $6)
        RETURNING id;
    `

	userID := uuid.NewV7()
	hashPwd, err := usecase.HashAndSaltPassword(password)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	var createdUserID uuid.UUID
	err = testDBPool.QueryRow(
		context.Background(), query,
		userID,
		firstName,
		lastName,
		email,
		hashPwd,
		false,
	).Scan(&createdUserID)
	if err != nil {
		t.Fatalf("Failed to insert user into database: %v", err)
	}

	return createdUserID
}

// assignRoleToUserInDB links a user to a seeded role by name.
//
// A user inserted straight into the table has no roles, so CheckAuthz denies it
// before any handler runs — even /auth/refresh, which every logged-in user is
// meant to reach. Registering through the API would pick up AuthenticatedUser
// automatically (it is auto_assign), but that path needs a verification email;
// this is the shortcut for tests that only need the user to be authorised.
func assignRoleToUserInDB(t *testing.T, userID uuid.UUID, roleName string) {
	t.Helper()

	query := `
        INSERT INTO users_roles (users_id, roles_id)
        SELECT $1, id FROM roles WHERE name = $2 LIMIT 1
        ON CONFLICT DO NOTHING;
    `

	tag, err := testDBPool.Exec(context.Background(), query, userID, roleName)
	if err != nil {
		t.Fatalf("Failed to assign role %q to user: %v", roleName, err)
	}

	if tag.RowsAffected() == 0 {
		t.Fatalf("Role %q was not assigned; does a role with that name exist?", roleName)
	}
}

// enableUserByEmailFromDB is a helper function to enable a user in the database
// It takes a testing.T object and the user's email address
// It returns an error if any
func enableUserByEmailFromDB(t *testing.T, email string) {
	t.Helper()

	query := `UPDATE users SET disabled = false WHERE email = $1;`
	_, err := testDBPool.Exec(context.Background(), query, email)
	if err != nil {
		t.Errorf("Failed to enable user in database: %v", err)
	}
}

// countUsersByEmailInDB counts the accounts holding an address.
//
// Read from the table rather than inferred from a response: registration
// answers the same way whether or not the address was taken, so the only way to
// see that a second account was NOT created is to look.
func countUsersByEmailInDB(t *testing.T, email string) int {
	t.Helper()

	var count int

	query := `SELECT COUNT(*) FROM users WHERE email = $1;`
	if err := testDBPool.QueryRow(context.Background(), query, email).Scan(&count); err != nil {
		t.Fatalf("Failed to count users: %v", err)
	}

	return count
}

// emailWithSubjectExists reports whether the mail server holds a message to
// this address with this subject.
func emailWithSubjectExists(t *testing.T, to, subject string) bool {
	t.Helper()

	endpoint := newMailAPIEndpoint(http.MethodGet, "/search")
	endpoint.requestURL.RawQuery = "query=" + url.QueryEscape(fmt.Sprintf("to:%s subject:%q", to, subject))

	response, err := sendHTTPRequest(t, context.Background(), endpoint, nil)
	if err != nil {
		t.Fatalf("Failed to search the mail server: %v", err)
	}
	defer response.Body.Close()

	type searchResponse struct {
		Messages []struct {
			ID string `json:"ID"`
		} `json:"Messages"`
	}

	found, err := parserResponseBody[searchResponse](t, response)
	if err != nil {
		t.Fatalf("Failed to parse the mail search response: %v", err)
	}

	return len(found.Messages) > 0
}

// setLocalAccountInDB marks whether an account authenticates with a password of
// its own or through an identity provider.
//
// Set directly rather than through the API because there is no endpoint for it:
// it is normally decided at registration by which flow was used.
func setLocalAccountInDB(t *testing.T, email string, local bool) {
	t.Helper()

	query := `UPDATE users SET local_account = $1 WHERE email = $2;`
	if _, err := testDBPool.Exec(context.Background(), query, local, email); err != nil {
		t.Fatalf("Failed to set local_account in database: %v", err)
	}
}

// isTokenRevokedInDB reports whether a jti has been recorded on the denylist.
//
// Read straight from the table rather than inferred from an endpoint's answer:
// a single-use token is spent as a side effect, so the only way to see that it
// happened is to look at where it was written.
func isTokenRevokedInDB(t *testing.T, jti string) bool {
	t.Helper()

	var exists bool

	query := `SELECT EXISTS (SELECT 1 FROM revoked_tokens WHERE jti = $1);`
	if err := testDBPool.QueryRow(context.Background(), query, jti).Scan(&exists); err != nil {
		t.Fatalf("Failed to read revoked_tokens: %v", err)
	}

	return exists
}

// ============================================================================
// CLEANUP HELPER FUNCTIONS
// ============================================================================
//
// IMPORTANT: Test Cleanup Order and Best Practices
//
// Go's t.Cleanup() executes cleanup functions in LIFO (Last In, First Out) order.
// This means the LAST cleanup registered runs FIRST.
//
// For tests that create relationships between entities (users↔roles, roles↔policies, etc.),
// you MUST unlink relationships BEFORE deleting the entities.
//
// CORRECT PATTERN (using LIFO to your advantage):
//
//   // 1. Create resources
//   userID := createUser(...)
//   roleID := createRole(...)
//
//   // 2. Link them
//   linkUserToRole(userID, roleID)
//
//   // 3. Register cleanup in REVERSE order of what you want to happen
//   t.Cleanup(func() { deleteUserByIDFromDB(t, adminToken.UserID) })  // Runs 4th (last)
//   t.Cleanup(func() { deleteRoleByIDFromDB(t, roleID) })             // Runs 3rd
//   t.Cleanup(func() { deleteUserByIDFromDB(t, userID) })             // Runs 2nd
//   t.Cleanup(func() { unlinkRolesFromUserViaDB(t, userID, []uuid.UUID{roleID}) }) // Runs 1st (FIRST!)
//
// This ensures cleanup happens in the correct order:
//   1. Unlink relationships
//   2. Delete dependent entities
//   3. Delete independent entities
//   4. Delete admin/setup users last
//
// ALTERNATIVE: Register cleanup immediately after creating resources
//
//   userID := createUser(...)
//   t.Cleanup(func() { deleteUserByIDFromDB(t, userID) })  // Register immediately
//
//   roleID := createRole(...)
//   t.Cleanup(func() { deleteRoleByIDFromDB(t, roleID) })  // Register immediately
//
//   linkUserToRole(userID, roleID)
//   t.Cleanup(func() { unlinkRolesFromUserViaDB(t, userID, []uuid.UUID{roleID}) })  // Unlinks run first
//
// ============================================================================

// deleteUserByEmailFromDB is a helper function to delete a user from the database
// It takes a testing.T object and the user's email address
// It returns an error if any
func deleteUserByEmailFromDB(t *testing.T, email string) {
	t.Helper()

	query := `DELETE FROM users WHERE email = $1;`
	_, err := testDBPool.Exec(context.Background(), query, email)
	if err != nil {
		t.Errorf("Failed to delete user from database: %v", err)
	}
}

// deleteUserByIDFromDB is a helper function to delete a user from the database
// It takes a testing.T object and the user's ID
// It returns an error if any
func deleteUserByIDFromDB(t *testing.T, userID uuid.UUID) {
	t.Helper()

	query := `DELETE FROM users WHERE id = $1;`
	_, err := testDBPool.Exec(context.Background(), query, userID)
	if err != nil {
		t.Errorf("Failed to delete user from database: %v", err)
	}
}

// deleteRoleByIDFromDB is a helper function to delete a role from the database
// It takes a testing.T object and the role's ID
// It returns an error if any
func deleteRoleByIDFromDB(t *testing.T, roleID uuid.UUID) {
	t.Helper()

	query := `DELETE FROM roles WHERE id = $1;`
	_, err := testDBPool.Exec(context.Background(), query, roleID)
	if err != nil {
		t.Errorf("Failed to delete role from database: %v", err)
	}
}

// deletePolicyByIDFromDB is a helper function to delete a policy from the database
// It takes a testing.T object and the policy's ID
// It returns an error if any
func deletePolicyByIDFromDB(t *testing.T, policyID uuid.UUID) {
	t.Helper()

	query := `DELETE FROM policies WHERE id = $1;`
	_, err := testDBPool.Exec(context.Background(), query, policyID)
	if err != nil {
		t.Errorf("Failed to delete policy from database: %v", err)
	}
}

// deleteProjectByIDFromDB is a helper function to delete a project from the database
// It takes a testing.T object and the project's ID
// It returns an error if any
func deleteProjectByIDFromDB(t *testing.T, projectID uuid.UUID) {
	t.Helper()

	query := `DELETE FROM projects WHERE id = $1;`
	_, err := testDBPool.Exec(context.Background(), query, projectID)
	if err != nil {
		t.Errorf("Failed to delete project from database: %v", err)
	}
}

// unlinkRolesFromUserViaDB is a helper function to unlink roles from a user via direct database deletion
// This should be used in cleanup functions before deleting users or roles
func unlinkRolesFromUserViaDB(t *testing.T, userID uuid.UUID, roleIDs []uuid.UUID) {
	t.Helper()

	if len(roleIDs) == 0 {
		return
	}

	query := `DELETE FROM users_roles WHERE users_id = $1 AND roles_id = ANY($2);`
	_, err := testDBPool.Exec(context.Background(), query, userID, roleIDs)
	if err != nil {
		t.Errorf("Failed to unlink roles from user in database: %v", err)
	}
}

// unlinkPoliciesFromRoleViaDB is a helper function to unlink policies from a role via direct database deletion
// This should be used in cleanup functions before deleting roles or policies
func unlinkPoliciesFromRoleViaDB(t *testing.T, roleID uuid.UUID, policyIDs []uuid.UUID) {
	t.Helper()

	if len(policyIDs) == 0 {
		return
	}

	query := `DELETE FROM roles_policies WHERE roles_id = $1 AND policies_id = ANY($2);`
	_, err := testDBPool.Exec(context.Background(), query, roleID, policyIDs)
	if err != nil {
		t.Errorf("Failed to unlink policies from role in database: %v", err)
	}
}

// unlinkAllRolesFromUserViaDB is a helper function to unlink all roles from a user
// This should be used in cleanup functions before deleting a user
func unlinkAllRolesFromUserViaDB(t *testing.T, userID uuid.UUID) {
	t.Helper()

	query := `DELETE FROM users_roles WHERE users_id = $1;`
	_, err := testDBPool.Exec(context.Background(), query, userID)
	if err != nil {
		t.Errorf("Failed to unlink all roles from user in database: %v", err)
	}
}

// unlinkAllPoliciesFromRoleViaDB is a helper function to unlink all policies from a role
// This should be used in cleanup functions before deleting a role
func unlinkAllPoliciesFromRoleViaDB(t *testing.T, roleID uuid.UUID) {
	t.Helper()

	query := `DELETE FROM roles_policies WHERE roles_id = $1;`
	_, err := testDBPool.Exec(context.Background(), query, roleID)
	if err != nil {
		t.Errorf("Failed to unlink all policies from role in database: %v", err)
	}
}

// unlinkAllUsersFromProjectViaDB is a helper function to unlink all users from a project
// This should be used in cleanup functions before deleting a project
func unlinkAllUsersFromProjectViaDB(t *testing.T, projectID uuid.UUID) {
	t.Helper()

	query := `DELETE FROM projects_users WHERE projects_id = $1;`
	_, err := testDBPool.Exec(context.Background(), query, projectID)
	if err != nil {
		t.Errorf("Failed to unlink all users from project in database: %v", err)
	}
}

// deleteProjectFromDBByName is a helper function to delete a project from the database by its name
func deleteProjectFromDBByName(t *testing.T, projectName string) {
	t.Helper()

	query := `DELETE FROM projects WHERE name = $1;`
	_, err := testDBPool.Exec(context.Background(), query, projectName)
	if err != nil {
		t.Errorf("Failed to delete project from database: %v", err)
	}
}

// removeAPIEndpointFromURL is a helper function to remove the API endpoint from the URL
// It takes a URL string and returns the URL string without the API endpoint
func removeAPIEndpointFromURL(urlStr string) string {
	return strings.TrimPrefix(urlStr, apiEndpointURL)
}

// getAdminUserTokens is a helper function to get the admin user tokens
// It takes a testing.T object and the admin user's email address
// It returns the access token and refresh token as strings
func getAdminUserTokens(t *testing.T) payload.LoginUserResponse {
	t.Helper()

	ctx := t.Context()

	tx, txErr := testDBPool.Begin(ctx)
	if txErr != nil {
		t.Fatalf("Failed to begin transaction: %v", txErr)
	}

	// 1. Insert a user into the database
	query1 := `
        INSERT INTO users (id, first_name, last_name, email, password_hash, disabled, admin)
        VALUES ($1, $2, $3, $4, $5, $6, $7);
    `

	userID := uuid.NewV7()

	firstName, lastName, email := generateUserData(t)
	password := generatePassword(t)
	hashPwd, err := usecase.HashAndSaltPassword(password)
	assert.NoError(t, err, "Failed to hash password")

	_, txErr = tx.Exec(
		context.Background(), query1,
		userID,
		firstName,
		lastName,
		email,
		hashPwd,
		false,
		true, // admin = true
	)
	if txErr != nil {
		t.Fatalf("Failed to insert user into database: %v", txErr)
	}

	// 2. Get the role_id for the admin role and assign it to the user
	query2 := `
    WITH
        role_id AS (
            SELECT id FROM roles WHERE name = 'Administrator' LIMIT 1
        )
        INSERT INTO users_roles (users_id, roles_id)
        SELECT $1, id FROM role_id;
    `

	_, txErr = tx.Exec(context.Background(), query2, userID)
	if txErr != nil {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("Failed to rollback transaction: %v", err)
		}
	} else {
		if err := tx.Commit(ctx); err != nil {
			t.Errorf("Failed to commit transaction: %v", err)
		}
	}

	// 3. Login the user
	// wait for login verification in the database
	// time.Sleep(200 * time.Millisecond)

	loginUser := map[string]any{
		"email":    email,
		"password": password,
	}

	loginResponse, err := sendHTTPRequest(t, ctx, authLoginEndpoint, loginUser)
	assert.NoError(t, err)
	if loginResponse != nil {
		defer loginResponse.Body.Close()
	}

	require.NotNil(t, loginResponse, "Login response should not be nil")
	assert.Equal(t, loginResponse.StatusCode, http.StatusOK, "Expected status code 200 OK. Got %d. Message: %s", loginResponse.StatusCode, readResponseBody(t, loginResponse))
	loginAPIResp, err := parserResponseBody[payload.LoginUserResponse](t, loginResponse)
	assert.NoError(t, err)

	assert.NotEmpty(t, loginAPIResp.AccessToken, "Expected access token to be present")
	assert.NotEmpty(t, loginAPIResp.RefreshToken, "Expected refresh token to be present")
	assert.NotEmpty(t, loginAPIResp.UserID, "Expected user ID to be present")

	assert.Equal(t, loginAPIResp.UserID.String(), userID.String(), "Expected user ID to match")
	assert.Equal(t, loginAPIResp.TokenType, domain.TokenTypeBearer, "Expected token type to be Bearer")

	return loginAPIResp
}

// replaceSlugs is a helper function to build a path with multiple slugs.
// NOTE: This is used for testing purposes only.
// replace the slugs in the path with the slugs provided in the order they are provided.
// if no slugs are provided, the first slug found in the path will be replaced.
func replaceSlugs(path string, slugs ...string) string {
	re := regexp.MustCompile(`\{([^}]+)\}`)

	var val string
	if len(slugs) == 0 {
		val = ""
	} else {
		for _, slug := range slugs {
			found := re.FindStringSubmatchIndex(path)
			if found == nil {
				break
			}

			start, end := found[0], found[1]
			path = strings.Replace(path, path[start:end], slug, 1)
		}

		val = path
	}

	return val
}

// readResponseBody is a helper function to read the response body
// It takes a testing.T object and an HTTP response
// It returns the response body as a byte slice and an error if any
// messageOf pulls the "message" field out of an API error body, so a test can
// compare what the caller is actually told rather than the whole envelope
// (which carries a timestamp and the path and therefore never compares equal).
func messageOf(body string) (string, error) {
	var envelope struct {
		Message string `json:"message"`
	}

	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		return "", err
	}

	return envelope.Message, nil
}

func readResponseBody(t *testing.T, resp *http.Response) string {
	if t == nil {
		t = &testing.T{}
	}
	t.Helper()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("Failed to read response body: %v", err)
		return ""
	}

	// Close the original body
	resp.Body.Close()

	// Create a new ReadCloser and replace the response body
	resp.Body = io.NopCloser(bytes.NewBuffer(body))

	return string(body)
}

// createProjectInDB is a helper function to create a project in the database
func createProjectInDB(t *testing.T, name string, description string, id ...uuid.UUID) domain.Project {
	t.Helper()

	if len(id) == 0 {
		id = append(id, uuid.NewV7())
	}

	query := `
        INSERT INTO projects (id, name, description, disabled)
        VALUES ($1, $2, $3, false)
        RETURNING id, name, description, disabled;
    `

	var project domain.Project
	err := testDBPool.QueryRow(
		context.Background(), query,
		id[0],
		name,
		description,
	).Scan(&project.ID, &project.Name, &project.Description, &project.Disabled)
	if err != nil {
		t.Fatalf("Failed to insert project into database: %v", err)
	}

	return project
}

// getProjectFromDBByName is a helper function to get a project from the database by its name
func getProjectFromDBByName(t *testing.T, projectName string) domain.Project {
	t.Helper()

	query := `
        SELECT id, name, description, system, disabled, created_at, updated_at
        FROM projects
        WHERE name = $1;
    `
	row := testDBPool.QueryRow(context.Background(), query, projectName)

	var project domain.Project
	err := row.Scan(&project.ID, &project.Name, &project.Description, &project.System, &project.Disabled, &project.CreatedAt, &project.UpdatedAt)
	if err != nil {
		t.Fatalf("Failed to get project from database: %v", err)
	}

	return project
}

// getIDPTypeFromDBByName is a helper function to get the IDP type from the database by its name
func getIDPTypeFromDBByName(t *testing.T, typeName string) domain.IDPTypes {
	t.Helper()

	query := `
		SELECT id, name, description, scopes, COALESCE(user_info_api_url, ''), kind, COALESCE(issuer_hint, ''), system, created_at, updated_at, serial_id
		FROM idp_types
		WHERE name = $1
	`

	row := testDBPool.QueryRow(context.Background(), query, typeName)

	var idpType domain.IDPTypes
	err := row.Scan(
		&idpType.ID,
		&idpType.Name,
		&idpType.Description,
		&idpType.Scopes,
		&idpType.UserInfoAPIURL,
		&idpType.Kind,
		&idpType.IssuerHint,
		&idpType.System,
		&idpType.CreatedAt,
		&idpType.UpdatedAt,
		&idpType.SerialID,
	)
	if err != nil {
		t.Fatalf("Failed to get IDP type from database: %v", err)
	}

	return idpType
}

// createIDPInDB is a helper function to create an IDP in the database
func createIDPInDB(t *testing.T, idpTypeID uuid.UUID, name, description string, id ...uuid.UUID) *domain.IDP {
	t.Helper()

	var idpID uuid.UUID
	var err error

	if len(id) > 0 {
		idpID = id[0]
	} else {
		idpID = uuid.NewV7()
	}

	query := `
		INSERT INTO idps (
			id, idp_types, name, description, callback_url,
			issuer_url, logo, client_id, client_secret
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err = testDBPool.Exec(
		context.Background(), query,
		idpID,
		idpTypeID,
		name,
		description,
		"http://localhost:5173/auth/idp/"+idpID.String()+"/callback",
		"https://accounts.google.com",
		"https://example.com/logo.png",
		"test-client-id",
		"test-client-secret",
	)
	if err != nil {
		t.Fatalf("Failed to create IDP in database: %v", err)
	}

	// Retrieve the created IDP to return full model
	return getIDPFromDBByID(t, idpID)
}

// getIDPFromDBByID is a helper function to get an IDP from the database by its ID
func getIDPFromDBByID(t *testing.T, idpID uuid.UUID) *domain.IDP {
	t.Helper()

	query := `
		SELECT
			i.id, i.name, i.description, i.callback_url,
			COALESCE(i.issuer_url, ''), i.enabled, i.auto_provision,
			i.logo, i.client_id, i.created_at, i.updated_at,
			i.serial_id,
			it.id, it.name, it.description, it.scopes,
			COALESCE(it.user_info_api_url, ''), it.kind, COALESCE(it.issuer_hint, ''),
			it.system, it.created_at,
			it.updated_at, it.serial_id
		FROM idps i
		INNER JOIN idp_types it ON i.idp_types = it.id
		WHERE i.id = $1
	`

	row := testDBPool.QueryRow(context.Background(), query, idpID)

	var idp domain.IDP
	err := row.Scan(
		&idp.ID,
		&idp.Name,
		&idp.Description,
		&idp.CallbackURL,
		&idp.IssuerURL,
		&idp.Enabled,
		&idp.AutoProvision,
		&idp.Logo,
		&idp.ClientID,
		&idp.CreatedAt,
		&idp.UpdatedAt,
		&idp.SerialID,
		&idp.IDPType.ID,
		&idp.IDPType.Name,
		&idp.IDPType.Description,
		&idp.IDPType.Scopes,
		&idp.IDPType.UserInfoAPIURL,
		&idp.IDPType.Kind,
		&idp.IDPType.IssuerHint,
		&idp.IDPType.System,
		&idp.IDPType.CreatedAt,
		&idp.IDPType.UpdatedAt,
		&idp.IDPType.SerialID,
	)
	if err != nil {
		t.Fatalf("Failed to get IDP from database: %v", err)
	}

	return &idp
}

// deleteIDPByIDFromDB is a helper function to delete an IDP from the database by its ID
func deleteIDPByIDFromDB(t *testing.T, idpID uuid.UUID) {
	t.Helper()

	query := `DELETE FROM idps WHERE id = $1`

	_, err := testDBPool.Exec(context.Background(), query, idpID)
	if err != nil {
		t.Fatalf("Failed to delete IDP from database: %v", err)
	}
}

// deleteIDPByNameFromDB is a helper function to delete an IDP from the database by its name
func deleteIDPByNameFromDB(t *testing.T, name string) {
	t.Helper()

	query := `DELETE FROM idps WHERE name = $1`

	_, err := testDBPool.Exec(context.Background(), query, name)
	if err != nil {
		t.Fatalf("Failed to delete IDP from database: %v", err)
	}
}

// assertJSONFieldsAreSnakeCase verifies that all fields in a JSON response are in snake_case format.
// It checks that expected snake_case fields exist and that PascalCase variants do NOT exist.
// For nested objects and arrays, it recursively validates all fields.
func assertJSONFieldsAreSnakeCase(t *testing.T, response *http.Response, expectedFields []string) {
	t.Helper()

	// Read the response body
	body := readResponseBody(t, response)

	// Parse into map to check raw field names
	var data map[string]any
	err := json.Unmarshal([]byte(body), &data)
	require.NoError(t, err, "Failed to unmarshal response body")

	// Check that all expected snake_case fields exist
	for _, field := range expectedFields {
		assert.Contains(t, data, field, "Expected snake_case field '%s' not found in response", field)

		// Generate potential PascalCase/camelCase variants and ensure they don't exist
		pascalCase := toPascalCase(field)
		if pascalCase != field {
			assert.NotContains(t, data, pascalCase, "Found PascalCase field '%s' but expected snake_case '%s'", pascalCase, field)
		}

		camelCase := toCamelCase(field)
		if camelCase != field {
			assert.NotContains(t, data, camelCase, "Found camelCase field '%s' but expected snake_case '%s'", camelCase, field)
		}
	}

	// Recursively check nested objects and arrays
	checkNestedFields(t, data)
}

// checkNestedFields recursively validates that nested objects and arrays use snake_case
func checkNestedFields(t *testing.T, data any) {
	t.Helper()

	switch v := data.(type) {
	case map[string]any:
		// Check all keys in the map
		for key, value := range v {
			// A permission map is keyed by resource pattern ("/users", "*",
			// "/projects/*/models"): data, not a field name. Only the values
			// beneath it are checked.
			if strings.HasPrefix(key, "/") || key == "*" {
				checkNestedFields(t, value)
				continue
			}
			// Verify the key itself is in snake_case (or a known exception)
			if !isValidSnakeCase(key) && !isKnownException(key) {
				t.Errorf("Field '%s' is not in snake_case format", key)
			}

			// Recursively check the value
			checkNestedFields(t, value)
		}
	case []any:
		// Check all items in the array
		for _, item := range v {
			checkNestedFields(t, item)
		}
	}
}

// isValidSnakeCase checks if a string is in valid snake_case format
func isValidSnakeCase(s string) bool {
	// snake_case pattern: lowercase letters, numbers, and underscores
	// Must start with a letter or underscore
	matched, _ := regexp.MatchString(`^[a-z_][a-z0-9_]*$`, s)
	return matched
}

// isKnownException checks if a field name is a known exception (like custom keys in authz, permissions, etc.)
func isKnownException(s string) bool {
	// Some fields might be user-defined or dynamic (e.g., in permissions maps)
	// Common exceptions in JSON responses:

	// 1. UUID keys (used as map keys in permissions, authz, etc.)
	if _, err := uuid.Parse(s); err == nil {
		return true
	}

	// 2. Wildcard or special operation keys
	if s == "*" {
		return true
	}

	// 3. HTTP-related fields that are intentionally not snake_case
	// (add more as needed)

	return false
}

// toPascalCase converts snake_case to PascalCase
func toPascalCase(s string) string {
	parts := strings.Split(s, "_")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "")
}

// toCamelCase converts snake_case to camelCase
func toCamelCase(s string) string {
	parts := strings.Split(s, "_")
	for i, part := range parts {
		if i > 0 && len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "")
}

// countRolesLinkedToPolicy returns how many roles are linked to a policy.
func countRolesLinkedToPolicy(t *testing.T, policyID uuid.UUID) int {
	t.Helper()

	var n int
	query := `SELECT count(*) FROM roles_policies WHERE policies_id = $1;`
	if err := testDBPool.QueryRow(context.Background(), query, policyID).Scan(&n); err != nil {
		t.Fatalf("Failed to count roles linked to policy: %v", err)
	}

	return n
}

// isRoleLinkedToPolicy reports whether a specific role is linked to a policy.
func isRoleLinkedToPolicy(t *testing.T, policyID, roleID uuid.UUID) bool {
	t.Helper()

	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM roles_policies WHERE policies_id = $1 AND roles_id = $2);`
	if err := testDBPool.QueryRow(context.Background(), query, policyID, roleID).Scan(&exists); err != nil {
		t.Fatalf("Failed to check role-policy link: %v", err)
	}

	return exists
}

// unlinkAllRolesFromPolicyViaDB removes every role link for a policy, for cleanup.
func unlinkAllRolesFromPolicyViaDB(t *testing.T, policyID uuid.UUID) {
	t.Helper()

	query := `DELETE FROM roles_policies WHERE policies_id = $1;`
	if _, err := testDBPool.Exec(context.Background(), query, policyID); err != nil {
		t.Errorf("Failed to unlink all roles from policy in database: %v", err)
	}
}
