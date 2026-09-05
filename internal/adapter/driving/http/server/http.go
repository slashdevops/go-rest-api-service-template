// Package server provides an HTTP server implementation with graceful shutdown and TLS support.
// It listens for OS signals to gracefully shut down or reload the server.
package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"

	"github.com/slashdevops/go-rest-api-service-template/internal/config"
)

type HTTPServerConfig struct {
	Ctx         context.Context
	HTTPHandler http.Handler
	Config      *config.HTTPServerConfig
}

type HTTPServer struct {
	ctx        context.Context
	httpServer *http.Server
	conf       *config.HTTPServerConfig

	stopChan chan struct{}

	// Protect stopChan from concurrent access
	mu       sync.Mutex
	isClosed bool
}

func NewHTTPServer(conf HTTPServerConfig) *HTTPServer {
	if conf.Ctx == nil {
		conf.Ctx = context.Background()
	}

	addr := fmt.Sprintf("%s:%d", conf.Config.Address.Value, conf.Config.Port.Value)

	ref := &HTTPServer{
		ctx: conf.Ctx,
		httpServer: &http.Server{
			Addr:    addr,
			Handler: conf.HTTPHandler,

			// ReadHeaderTimeout is the Slowloris bound and is always set: Go
			// applies no header deadline of its own, so without it a client can
			// hold a connection open forever by trickling headers one byte at a
			// time. It never touches the handler.
			ReadHeaderTimeout: conf.Config.ReadHeaderTimeout.Value,

			// IdleTimeout reaps keep-alive connections between requests. It
			// cannot interrupt a request in flight.
			IdleTimeout: conf.Config.IdleTimeout.Value,

			MaxHeaderBytes: conf.Config.MaxHeaderBytes.Value,

			// ReadTimeout and WriteTimeout default to 0 (disabled) and are
			// opt-in, because both bound something this service legitimately
			// does slowly — see the Default* comments in config/httpserver.go.
			ReadTimeout:  conf.Config.ReadTimeout.Value,
			WriteTimeout: conf.Config.WriteTimeout.Value,
		},
		conf:     conf.Config,
		stopChan: make(chan struct{}),
		isClosed: false,
	}

	// A write deadline caps *total* request duration, so it can abort a long
	// generation mid-flight. Operators who set it deserve to see it in the log
	// rather than discover it as a truncated response under load.
	if conf.Config.WriteTimeout.Value > 0 {
		slog.Warn(
			"http.server.write.timeout is set; it caps total request duration and can abort a long-running generation",
			"write_timeout", conf.Config.WriteTimeout.Value,
		)
	}

	return ref
}

func (ref *HTTPServer) Start() {
	slog.Info("starting http server", "address", ref.httpServer.Addr, "tls", ref.conf.TLSEnabled.Value)

	// Monitor for shutdown signal
	ref.monitorShutdown()

	if ref.conf.TLSEnabled.Value {
		if err := ref.setTLSConfig(); err != nil {
			slog.Error("failed to configure TLS", "error", err)
			ref.Stop()
			return
		}

		if err := ref.httpServer.ListenAndServeTLS(
			ref.conf.CertificateFile.Value.Name(),
			ref.conf.PrivateKeyFile.Value.Name(),
		); !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server error", "error", err)

			ref.Stop()
		}
	} else {
		if err := ref.httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server error", "error", err)

			ref.Stop()
		}
	}
}

func (ref *HTTPServer) Wait() <-chan struct{} {
	return ref.stopChan
}

func (ref *HTTPServer) Stop() {
	ref.mu.Lock()
	defer ref.mu.Unlock()

	// If already closed, don't try to send on the channel
	if ref.isClosed {
		slog.Debug("stop channel already closed")
		return
	}

	// Use a non-blocking send to avoid potential deadlocks
	select {
	case ref.stopChan <- struct{}{}:
		// Successfully sent stop signal
		slog.Debug("sent stop signal")
	default:
		// Channel is not receiving (buffer full)
		slog.Debug("stop channel not receiving")
	}
}

func (ref *HTTPServer) monitorShutdown() {
	go func() {
		slog.Info("http server monitoring for shutdown signal")

		<-ref.stopChan
		slog.Info("received programmatic shutdown signal")

		ctx, cancel := context.WithTimeout(ref.ctx, ref.conf.ShutdownTimeout.Value)
		defer cancel()

		if err := ref.httpServer.Shutdown(ctx); err != nil {
			slog.Error("http server shutdown with error", "error", err)
			os.Exit(1)
		}

		// Mark channel as closed to prevent sending on closed channel
		ref.mu.Lock()
		if !ref.isClosed {
			ref.isClosed = true
			close(ref.stopChan)
		}
		ref.mu.Unlock()

		slog.Info("http server shutdown complete")
	}()
}

// setTLSConfig sets the TLS configuration for the server.
func (ref *HTTPServer) setTLSConfig() error {
	slog.Info("configuring tls")
	if _, err := os.Stat(ref.conf.CertificateFile.Value.Name()); os.IsNotExist(err) {
		slog.Error(".crt file not found", "file", ref.conf.CertificateFile.Value.Name(), "error", err)
		return err
	}

	if _, err := os.Stat(ref.conf.PrivateKeyFile.Value.Name()); os.IsNotExist(err) {
		slog.Error(".key file not found", "file", ref.conf.PrivateKeyFile.Value.Name(), "error", err)
		return err
	}

	// CurvePreferences is intentionally left unset so that Go's stdlib
	// defaults apply — including the post-quantum hybrid KEMs added in
	// Go 1.26 (X25519MLKEM768, SecP384r1MLKEM1024).
	//
	// PreferServerCipherSuites is deliberately absent: crypto/tls has ignored
	// it since Go 1.18, which now trips staticcheck's SA1019. Do not re-add it.
	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		},
	}
	ref.httpServer.TLSConfig = tlsCfg
	ref.httpServer.TLSNextProto = make(map[string]func(*http.Server, *tls.Conn, http.Handler), 0)

	return nil
}
