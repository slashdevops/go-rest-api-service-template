package handler

import (
	"net/http"
	"net/http/pprof"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/middleware"
)

// PprofHandler handles the pprof routes
type PprofHandler struct{}

// NewPprofHandler creates a new PprofHandler
func NewPprofHandler() *PprofHandler {
	return &PprofHandler{}
}

// RegisterRoutes registers the routes for the handler
func (ref *PprofHandler) RegisterRoutes(mux *http.ServeMux, middlewares ...middleware.Middleware) {
	mdw := middleware.Chain(middlewares...)

	mux.Handle("GET /debug/pprof/", mdw.ThenFunc(pprof.Index))
	mux.Handle("GET /debug/pprof/cmdline", mdw.ThenFunc(pprof.Cmdline))
	mux.Handle("GET /debug/pprof/profile", mdw.ThenFunc(pprof.Profile))
	mux.Handle("GET /debug/pprof/symbol", mdw.ThenFunc(pprof.Symbol))
	mux.Handle("GET /debug/pprof/trace", mdw.ThenFunc(pprof.Trace))
	mux.Handle("GET /debug/pprof/allocs", mdw.ThenFunc(pprof.Handler("allocs").ServeHTTP))
	mux.Handle("GET /debug/pprof/block", mdw.ThenFunc(pprof.Handler("block").ServeHTTP))
	mux.Handle("GET /debug/pprof/goroutine", mdw.ThenFunc(pprof.Handler("goroutine").ServeHTTP))
	mux.Handle("GET /debug/pprof/heap", mdw.ThenFunc(pprof.Handler("heap").ServeHTTP))
	mux.Handle("GET /debug/pprof/mutex", mdw.ThenFunc(pprof.Handler("mutex").ServeHTTP))
	mux.Handle("GET /debug/pprof/threadcreate", mdw.ThenFunc(pprof.Handler("threadcreate").ServeHTTP))
}
