//go:build unit

package payload

import (
	"encoding/json"
	"testing"
)

// liveShape is the body GET /users/{id}/authz answered with on a seeded
// database, copied verbatim. Using the real bytes rather than a hand-written
// example is the point: the type exists to describe what the endpoint actually
// sends, and an example invented alongside the type would agree with it for
// free.
const liveShape = `{"permissions":{"users":{"019822af-b448-73fb-89a1-447e8f8d1cde":{"*":["*"]}}}}`

func TestNewUserAuthzResponseTypesTheLiveShape(t *testing.T) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(liveShape), &raw); err != nil {
		t.Fatalf("decoding the captured body: %v", err)
	}

	got, err := NewUserAuthzResponse(raw)
	if err != nil {
		t.Fatalf("the live shape should type cleanly: %v", err)
	}

	methods := got.Permissions["users"]["019822af-b448-73fb-89a1-447e8f8d1cde"]["*"]
	if len(methods) != 1 || methods[0] != "*" {
		t.Errorf("expected the global grant to survive, got %v", methods)
	}
}

// TestNewUserAuthzResponseRoundTripsToTheSameBytes is the assertion that makes
// this safe to ship: typing the body must not change it.
//
// The endpoint has been answering with the untyped map since it was written, so
// any client reading it today is reading these bytes. A type that reorders,
// renames or drops anything would be a breaking change disguised as
// documentation.
func TestNewUserAuthzResponseRoundTripsToTheSameBytes(t *testing.T) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(liveShape), &raw); err != nil {
		t.Fatalf("decoding the captured body: %v", err)
	}

	typed, err := NewUserAuthzResponse(raw)
	if err != nil {
		t.Fatalf("typing the captured body: %v", err)
	}

	encoded, err := json.Marshal(typed)
	if err != nil {
		t.Fatalf("re-encoding: %v", err)
	}

	// Compared structurally, not byte for byte: Go sorts map keys on output and
	// the captured string is in the order the database produced. Order is not
	// part of the contract; the content is.
	var before, after any
	if err := json.Unmarshal([]byte(liveShape), &before); err != nil {
		t.Fatalf("decoding the original: %v", err)
	}

	if err := json.Unmarshal(encoded, &after); err != nil {
		t.Fatalf("decoding the round trip: %v", err)
	}

	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)

	if string(beforeJSON) != string(afterJSON) {
		t.Errorf("typing changed the body:\n before %s\n after  %s", beforeJSON, afterJSON)
	}
}

// TestNewUserAuthzResponseReportsAShapeItCannotType pins the branch the handler
// relies on: a mismatch is an error it can fall back from, never a panic and
// never a silently emptied permission set.
func TestNewUserAuthzResponseReportsAShapeItCannotType(t *testing.T) {
	// Methods as a bare string rather than a list -- the most plausible drift,
	// and one a permissive walker would happily discard.
	raw := map[string]any{
		"permissions": map[string]any{
			"users": map[string]any{"some-id": map[string]any{"*": "*"}},
		},
	}

	if _, err := NewUserAuthzResponse(raw); err == nil {
		t.Error("a permission set whose methods are not a list should be reported, " +
			"not quietly dropped: permissions are the last thing to lose silently")
	}
}

// TestNewAuthzPermissionsTypesTheUnwrappedShape covers the other two endpoints
// that carry a permission set.
//
// /me/authz and /auth/login send it WITHOUT the outer "permissions" key --
// usecase.Authn strips that level -- so they need the unwrapped form. Captured
// from both live endpoints on a seeded database; they agree.
func TestNewAuthzPermissionsTypesTheUnwrappedShape(t *testing.T) {
	const live = `{"users":{"019822af-b448-73fb-89a1-447e8f8d1cde":{"*":["*"]}}}`

	var raw map[string]any
	if err := json.Unmarshal([]byte(live), &raw); err != nil {
		t.Fatalf("decoding the captured body: %v", err)
	}

	got, err := NewAuthzPermissions(raw)
	if err != nil {
		t.Fatalf("the live shape should type cleanly: %v", err)
	}

	if methods := got["users"]["019822af-b448-73fb-89a1-447e8f8d1cde"]["*"]; len(methods) != 1 {
		t.Errorf("expected the global grant to survive, got %v", methods)
	}
}

// TestAnUnrecognisedShapeYieldsAnEmptySetRatherThanAPartialOne is the property
// the handlers depend on when they log and carry on.
//
// Both /me/authz and /auth/login send whatever comes back even when the
// conversion failed, so what comes back on failure has to be EMPTY. A partial
// set would be worse than none: the client would offer exactly the controls
// that happened to parse, with nothing to say the rest were lost.
//
// Empty is safe because these sets are a client-side hint. Authorization is
// decided by CheckAuthz on every request, from the same data, server-side -- so
// an empty set hides controls the caller may use and can never reveal one they
// may not.
func TestAnUnrecognisedShapeYieldsAnEmptySetRatherThanAPartialOne(t *testing.T) {
	raw := map[string]any{
		"users": map[string]any{
			"good-id": map[string]any{"*": []any{"GET"}},
			"bad-id":  map[string]any{"*": "GET"}, // a string where a list belongs
		},
	}

	got, err := NewAuthzPermissions(raw)
	if err == nil {
		t.Fatal("a set with a malformed entry must be reported")
	}

	if len(got) != 0 {
		t.Errorf("a failed conversion must yield an empty set, not the entries "+
			"that happened to parse; got %v", got)
	}
}
