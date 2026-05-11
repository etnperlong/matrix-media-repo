package _routers

import (
	"net/http/httptest"
	"testing"

	"github.com/t2bot/matrix-media-repo/common/config"
)

func TestGetRemoteAddrUsesForwardedForWhenTrustAnyForwardEnabled(t *testing.T) {
	cfg := config.Get()
	oldTrustAnyForward := cfg.General.TrustAnyForward
	t.Cleanup(func() {
		cfg.General.TrustAnyForward = oldTrustAnyForward
	})

	cfg.General.TrustAnyForward = true

	req := httptest.NewRequest("GET", "http://example.com", nil)
	req.RemoteAddr = "198.51.100.10:8448"
	req.Header.Set("X-Forwarded-For", "203.0.113.7")

	if got := GetRemoteAddr(req); got != "203.0.113.7" {
		t.Fatalf("expected forwarded IP, got %q", got)
	}
}

func TestGetRemoteAddrUsesSocketHostWhenTrustAnyForwardDisabled(t *testing.T) {
	cfg := config.Get()
	oldTrustAnyForward := cfg.General.TrustAnyForward
	t.Cleanup(func() {
		cfg.General.TrustAnyForward = oldTrustAnyForward
	})

	cfg.General.TrustAnyForward = false

	req := httptest.NewRequest("GET", "http://example.com", nil)
	req.RemoteAddr = "198.51.100.10:8448"

	if got := GetRemoteAddr(req); got != "198.51.100.10" {
		t.Fatalf("expected socket host, got %q", got)
	}
}
