package handler

import (
	"net/http"
	"testing"
)

// TestHTTPStatusForHealth pins the codes a caller can act on.
//
// 206 rather than 200 for degraded so "working, but something is wrong" is
// distinguishable from "working"; 503 for unhealthy so a load balancer or a
// readiness probe can take the instance out of rotation. Verified against the
// running service by stopping each dependency in turn:
//
//	everything healthy   -> 200
//	cache stopped        -> 200   (fail-open: the request still succeeds)
//	database stopped     -> 503   (was 206, a success code)
func TestHTTPStatusForHealth(t *testing.T) {
	for status, want := range map[string]int{
		"healthy":   http.StatusOK,
		"degraded":  http.StatusPartialContent,
		"unhealthy": http.StatusServiceUnavailable,

		// Anything unrecognised is treated as serving. A health endpoint that
		// fails closed on a status it does not know would take a working
		// service out of rotation the first time a new state is added.
		"":          http.StatusOK,
		"brand-new": http.StatusOK,
	} {
		t.Run(status, func(t *testing.T) {
			if got := httpStatusForHealth(status); got != want {
				t.Errorf("httpStatusForHealth(%q) = %d, want %d", status, got, want)
			}
		})
	}
}
