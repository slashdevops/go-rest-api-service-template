package domain

import (
	"strings"
	"uuid"
)

//go:fix inline
func MakePointer[T any](v T) *T {
	return new(v)
}

// EnsureUUIDV7 ensures the ID is set, generating a new V7 UUID if it's nil.
//
// The returned error is always nil. The standard library's [uuid.NewV7] cannot
// fail — it panics if the CSPRNG does — so there is nothing left to report. The
// two-value signature is kept so the package swap does not have to touch the 22
// call sites and their error branches; collapsing it is a separate change.
func EnsureUUIDV7(id uuid.UUID) (uuid.UUID, error) {
	if id == uuid.Nil() {
		return uuid.NewV7(), nil
	}

	return id, nil
}

// UUIDVersion reports the version nibble of id.
//
// The standard library [uuid] package exposes no Version method: per RFC 9562
// the version is the high nibble of byte 6. Derive it here and nowhere else —
// [IsUUIDV7] and [ValidateUUID] enforce the v7-everywhere rule the whole ID
// scheme rests on, and a second copy of this shift is a second place for it to
// drift.
func UUIDVersion(id uuid.UUID) int {
	return int(id[6] >> 4)
}

// IsUUIDV7 checks if the given UUID is a version 7 UUID.
func IsUUIDV7(id uuid.UUID) bool {
	if id == uuid.Nil() {
		return false
	}

	return UUIDVersion(id) == 7
}

// joinStringsComma renders a list for an error message that tells the caller
// what to use instead of only what was wrong.
func joinStringsComma(in []string) string {
	return strings.Join(in, ", ")
}
