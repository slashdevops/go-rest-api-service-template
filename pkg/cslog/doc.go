// Package cslog extends Go's standard [log/slog] with custom log levels
// that the stdlib does not provide out of the box: Trace and Fatal.
//
// # Why the name?
//
// "cslog" reads as "custom slog" — a two-letter abbreviation of "custom"
// plus the stdlib package name. It is in the same spirit as the
// community's "x"-suffixed extension packages (errgroup, httputil,
// slogtest) but uses a leading prefix for closer visual proximity to
// the standard package name, so that imports and call sites stay
// instantly recognizable next to slog itself:
//
//	"log/slog"                            // stdlib
//	".../pkg/cslog"                       // this package
//
//	slog.Info(...)                        // stdlib
//	cslog.Trace(ctx, ...)                 // this package
//
// Go's style guide discourages opaque package names; this docs.go
// disambiguates the abbreviation.
//
// # What it adds
//
// Two custom slog levels:
//
//   - [LogLevelTrace] (slog.Level -8) — below stdlib's Debug (-4). Intended
//     for very verbose, development-time logging: raw SQL queries, HTTP
//     request and response bodies, internal-state dumps. Almost always
//     filtered out in production.
//   - [LogLevelFatal] (slog.Level 12) — above stdlib's Error (8). Intended
//     for unrecoverable conditions where the application cannot continue.
//
// Both [Trace] and [Fatal] take a [context.Context] so that structured
// values attached to the context (trace IDs, request IDs, etc.) flow
// into the log record automatically — the same shape as [slog.InfoContext].
//
// # Usage
//
//	cslog.Trace(ctx, "SQL query", "query", q, "args", args)
//	cslog.Fatal(ctx, "database connection lost", "error", err)
//
// # Enabling Trace output
//
// Trace records have a level below the default minimum, so they are
// suppressed unless the handler is configured to allow them. To see
// trace logs, set the handler's minimum level to -8:
//
//	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
//	    Level: slog.Level(-8),
//	})))
//
// # Where it is used
//
// The service routes verbose diagnostics — repository SQL queries,
// outbound HTTP payloads, cache key traces — through [Trace]. See the
// "ctrace" / "trace" log-level options exposed by the application
// config in
// [github.com/slashdevops/go-rest-api-service-template/internal/config] for the wiring.
package cslog
