// Package core is the application hexagon for go-rest-api-service-template.
//
// Subpackages:
//
//   - core/domain   - pure entities, value objects, domain errors.
//   - core/port     - interfaces (driving and driven).
//   - core/usecase  - application services that implement driving ports
//     and consume driven ports.
//
// Source code under core/ must not import any infrastructure (HTTP, SQL,
// cache clients, mailers, OPA, JWT libs, OTEL exporters, etc.). The
// arch_test build tag enforces this invariant; see arch_test.go.
package core
