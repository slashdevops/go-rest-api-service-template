// Package payload holds the HTTP transport types for the driving HTTP
// adapter: request bodies, response bodies, JSON envelopes, and the
// swagger annotations that document them. It is the wire shape of the
// API, distinct from the domain entities defined in
// [github.com/slashdevops/go-rest-api-service-template/internal/core/domain].
//
// # Why the name?
//
// Previously this package was named "dto" — a generic OOP pattern
// label (Data Transfer Object) that adds no information at the call
// site and reads poorly in Go, where call sites prefer descriptive
// package names ("payload.LoginUserRequest" beats
// "dto.LoginUserRequest"). Renamed in favour of "payload" — the actual
// concept: what travels over the wire.
//
// # Conventions
//
//   - Types in this package may carry `json:"…"` tags and `@Description`
//     swaggo annotations.
//   - Types in this package must not be imported from
//     internal/core/... (enforced by TestCoreHasNoInfraImports in
//     internal/core/arch_test.go).
//   - The mapping from core/domain values to payload values lives in
//     the HTTP handlers under
//     internal/adapter/driving/http/handler/. Domain types stay free
//     of transport-layer struct tags.
//
// A small number of types in this package are intentional type
// aliases for domain types (see e.g. models_params.go). Aliases let
// the swagger generator surface a stable transport name without
// duplicating the underlying definition.
package payload
