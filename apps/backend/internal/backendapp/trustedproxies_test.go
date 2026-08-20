package backendapp

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
)

func TestConfigureTrustedProxiesUsesTypedStartupConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("server:\n  trustedProxies:\n    - 10.0.0.0/8\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KANDEV_SERVER_PORT", "")
	t.Setenv("KANDEV_TRUSTED_PROXIES", "")
	cfg, err := config.LoadWithPath(configPath)
	if err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	configureTrustedProxies(router, testLogger(t), cfg.Server.TrustedProxies)
	router.GET("/", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.5:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if got := rec.Body.String(); got != "203.0.113.7" {
		t.Fatalf("typed config ClientIP = %q, want 203.0.113.7", got)
	}
}

func TestResolveTrustedProxies(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		trusted []string
		invalid []string
	}{
		{name: "unset", raw: "", trusted: nil, invalid: nil},
		{name: "blank", raw: "   ", trusted: nil, invalid: nil},
		{name: "single CIDR", raw: "10.0.0.0/8", trusted: []string{"10.0.0.0/8"}, invalid: nil},
		{name: "multiple CIDRs with whitespace", raw: "10.0.0.0/8, 192.168.0.0/16", trusted: []string{"10.0.0.0/8", "192.168.0.0/16"}, invalid: nil},
		{name: "bare IPv4", raw: "10.0.0.1", trusted: []string{"10.0.0.1"}, invalid: nil},
		{name: "bare IPv6", raw: "2001:db8::1", trusted: []string{"2001:db8::1"}, invalid: nil},
		{name: "IPv6 CIDR", raw: "2001:db8::/32", trusted: []string{"2001:db8::/32"}, invalid: nil},
		{name: "invalid entry", raw: "not-a-cidr", trusted: nil, invalid: []string{"not-a-cidr"}},
		{name: "invalid IP", raw: "999.1.1.1", trusted: nil, invalid: []string{"999.1.1.1"}},
		{name: "mixed valid and invalid fails closed", raw: "10.0.0.0/8, not-a-cidr", trusted: nil, invalid: []string{"not-a-cidr"}},
		{name: "trailing comma fails closed", raw: "10.0.0.0/8,", trusted: nil, invalid: []string{emptyTrustedProxyEntry}},
		{name: "leading comma fails closed", raw: ",10.0.0.0/8", trusted: nil, invalid: []string{emptyTrustedProxyEntry}},
		{name: "doubled comma fails closed", raw: "10.0.0.0/8,,192.168.0.0/16", trusted: nil, invalid: []string{emptyTrustedProxyEntry}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trusted, invalid := resolveTrustedProxies(tt.raw)
			if !reflect.DeepEqual(trusted, tt.trusted) {
				t.Fatalf("resolveTrustedProxies(%q) trusted = %v, want %v", tt.raw, trusted, tt.trusted)
			}
			if !reflect.DeepEqual(invalid, tt.invalid) {
				t.Fatalf("resolveTrustedProxies(%q) invalid = %v, want %v", tt.raw, invalid, tt.invalid)
			}
		})
	}
}

// clientIPThroughConfig builds a router, applies configureTrustedProxies with
// the current environment, and returns ClientIP for a request from peer.
func clientIPThroughConfig(t *testing.T, peer, forwardedFor, xRealIP string) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	configureTrustedProxies(router, testLogger(t))
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, c.ClientIP())
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = peer
	if forwardedFor != "" {
		req.Header.Set("X-Forwarded-For", forwardedFor)
	}
	if xRealIP != "" {
		req.Header.Set("X-Real-IP", xRealIP)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec.Body.String()
}

func TestConfigureTrustedProxiesHonorsForwardedForWhenPeerTrusted(t *testing.T) {
	t.Setenv(trustedProxiesEnv, "10.0.0.0/8")
	if got := clientIPThroughConfig(t, "10.0.0.5:1234", "203.0.113.7", ""); got != "203.0.113.7" {
		t.Fatalf("trusted peer ClientIP = %q, want 203.0.113.7", got)
	}
}

func TestConfigureTrustedProxiesUnsetIgnoresHeader(t *testing.T) {
	t.Setenv(trustedProxiesEnv, "")
	if got := clientIPThroughConfig(t, "10.0.0.5:1234", "203.0.113.7", ""); got != "10.0.0.5" {
		t.Fatalf("unset ClientIP = %q, want peer 10.0.0.5", got)
	}
}

func TestConfigureTrustedProxiesUntrustedPeerIgnoresHeader(t *testing.T) {
	t.Setenv(trustedProxiesEnv, "192.168.0.0/16")
	if got := clientIPThroughConfig(t, "10.0.0.5:1234", "203.0.113.7", ""); got != "10.0.0.5" {
		t.Fatalf("untrusted peer ClientIP = %q, want peer 10.0.0.5", got)
	}
}

func TestConfigureTrustedProxiesInvalidEntryWarnsAndFallsBack(t *testing.T) {
	t.Setenv(trustedProxiesEnv, "10.0.0.0/8, not-a-cidr")
	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("observer logger: %v", err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	configureTrustedProxies(router, log)
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, c.ClientIP())
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.5:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if got := rec.Body.String(); got != "10.0.0.5" {
		t.Fatalf("invalid env ClientIP = %q, want peer 10.0.0.5", got)
	}
	warned := false
	for _, entry := range logs.All() {
		if !strings.Contains(entry.Message, "trusted-proxy setting") {
			continue
		}
		for _, field := range entry.Context {
			if field.Key == "invalid_entries" && strings.Contains(fmt.Sprint(field.Interface), "not-a-cidr") {
				warned = true
			}
		}
	}
	if !warned {
		t.Fatalf("expected startup warning naming not-a-cidr, got logs: %v", logs.All())
	}
}
