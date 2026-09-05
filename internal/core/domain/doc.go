// Package domain holds the pure business entities, value objects,
// and command/query inputs of go-rest-api-service-template.
//
// Code in this package must not import:
//
//   - any internal/adapter/... package
//   - any internal/app, internal/http, internal/repository,
//     internal/llmclient, internal/jwtvalidator, internal/opa,
//     internal/templates package
//   - net/http, database/sql, or any database/cache/mailer/policy
//     SDK
//
// Domain types may carry validation logic and may surface domain
// errors. They must not carry transport annotations such as
// `json:"…"` or swaggo `@Description` — those live in the payload
// package owned by the corresponding driving adapter.
//
// The arch_test build tag enforces the import rules; see
// internal/core/arch_test.go.
package domain
