package server

import (
	"net/http"
	"testing"
	"time"

	"github.com/slashdevops/go-rest-api-service-template/internal/config"
)

// TestNewHTTPServerAppliesTimeouts guards the wiring, not the values. The
// configuration being correct is useless if it never reaches the http.Server —
// which is exactly the state this server shipped in until now: it was built
// with only Addr and Handler, leaving no bound at all on a Slowloris-style
// header trickle.
func TestNewHTTPServerAppliesTimeouts(t *testing.T) {
	conf := config.NewHTTPServerConfig()
	conf.ReadHeaderTimeout.Value = 7 * time.Second
	conf.ReadTimeout.Value = 30 * time.Second
	conf.WriteTimeout.Value = 45 * time.Second
	conf.IdleTimeout.Value = 90 * time.Second
	conf.MaxHeaderBytes.Value = 65536

	srv := NewHTTPServer(HTTPServerConfig{
		HTTPHandler: http.NewServeMux(),
		Config:      conf,
	})

	if got := srv.httpServer.ReadHeaderTimeout; got != 7*time.Second {
		t.Errorf("ReadHeaderTimeout not applied: want 7s, got %v", got)
	}
	if got := srv.httpServer.ReadTimeout; got != 30*time.Second {
		t.Errorf("ReadTimeout not applied: want 30s, got %v", got)
	}
	if got := srv.httpServer.WriteTimeout; got != 45*time.Second {
		t.Errorf("WriteTimeout not applied: want 45s, got %v", got)
	}
	if got := srv.httpServer.IdleTimeout; got != 90*time.Second {
		t.Errorf("IdleTimeout not applied: want 90s, got %v", got)
	}
	if got := srv.httpServer.MaxHeaderBytes; got != 65536 {
		t.Errorf("MaxHeaderBytes not applied: want 65536, got %d", got)
	}
}

// TestNewHTTPServerDefaultsLeaveBodyTimeoutsOff pins the deliberate part of the
// design: the header and idle bounds are on out of the box, while ReadTimeout
// and WriteTimeout stay off because both would cut off work this service does
// legitimately — a large bulk-ingest upload, and a generation that may retry up
// to http.client.max.retries times at http.client.timeout each.
func TestNewHTTPServerDefaultsLeaveBodyTimeoutsOff(t *testing.T) {
	srv := NewHTTPServer(HTTPServerConfig{
		HTTPHandler: http.NewServeMux(),
		Config:      config.NewHTTPServerConfig(),
	})

	if srv.httpServer.ReadHeaderTimeout <= 0 {
		t.Errorf("ReadHeaderTimeout must be set by default, got %v", srv.httpServer.ReadHeaderTimeout)
	}
	if srv.httpServer.IdleTimeout <= 0 {
		t.Errorf("IdleTimeout must be set by default, got %v", srv.httpServer.IdleTimeout)
	}
	if srv.httpServer.ReadTimeout != 0 {
		t.Errorf("ReadTimeout must default to 0, got %v", srv.httpServer.ReadTimeout)
	}
	if srv.httpServer.WriteTimeout != 0 {
		t.Errorf("WriteTimeout must default to 0, got %v", srv.httpServer.WriteTimeout)
	}
}
