package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/pprof"

	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

// initTelemetry initializes the observability components (metrics, tracing)
func (a *App) initTelemetry(ctx context.Context) error {
	var err error

	slog.Info("initializing telemetry")

	// Create OpenTelemetry instance
	a.telemetry, err = o11y.New(ctx, a.configs.Telemetry)
	if err != nil {
		return fmt.Errorf("error creating OpenTelemetry: %w", err)
	}

	// Start telemetry services
	if err := a.telemetry.Start(); err != nil {
		return fmt.Errorf("error starting telemetry: %w", err)
	}

	slog.Info("telemetry started successfully")
	return nil
}

// startPprofServer starts the pprof server for debugging if enabled.
//
// Only five handlers are registered explicitly, but the server exposes more
// than five profiles: `/debug/pprof/` is a subtree pattern, so [pprof.Index]
// also serves every *named* profile in [runtime/pprof.Profiles] at
// `/debug/pprof/<name>` — `heap`, `allocs`, `goroutine`, `mutex`, `block`,
// `threadcreate`, and `goroutineleak`. Do not add a handler per profile.
//
// # goroutineleak
//
// `goroutineleak` went GA in Go 1.27 (it was a GOEXPERIMENT in 1.26) and is the
// first place to look for a stuck worker or a leaked request context: it reports
// goroutines blocked forever on a channel, [sync.Mutex] or [sync.Cond] that can
// never become unblocked, which a plain `goroutine` dump cannot distinguish from
// one that is merely idle.
//
//	go tool pprof http://localhost:6060/debug/pprof/goroutineleak
//
// The server is **off by default** and binds to localhost; enable it with
// `-http.server.pprof.enabled=true`. It listens on its own port (6060), not the
// API port, so nothing here is reachable from the public listener.
func (a *App) startPprofServer() {
	pprofAddr := fmt.Sprintf(
		"%s:%d",
		a.configs.HTTPServer.PprofAddress.Value,
		a.configs.HTTPServer.PprofPort.Value,
	)

	pprofURL := fmt.Sprintf("http://%s/debug/pprof", pprofAddr)
	slog.Info("starting pprof server", "url", pprofURL)

	pprofRouter := http.NewServeMux()

	// Register pprof handlers
	pprofRouter.HandleFunc("/debug/pprof/", pprof.Index)
	pprofRouter.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	pprofRouter.HandleFunc("/debug/pprof/profile", pprof.Profile)
	pprofRouter.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	pprofRouter.HandleFunc("/debug/pprof/trace", pprof.Trace)

	// Create the server
	a.pprofServer = &http.Server{
		Addr:    pprofAddr,
		Handler: pprofRouter,
	}

	if err := a.pprofServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("pprof server error", "error", err)
	}
}
