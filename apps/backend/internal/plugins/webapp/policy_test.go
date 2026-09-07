package webapp

import (
	"strings"
	"testing"
)

func TestBuildContentSecurityPolicyIsGrantBoundAndOpaque(t *testing.T) {
	policy, err := BuildContentSecurityPolicy([]string{"https://api.example.com:443", "https://api.example.com:443"}, []string{"http://127.0.0.1:38429", "tauri://localhost", "http://tauri.localhost"})
	if err != nil {
		t.Fatalf("BuildContentSecurityPolicy() unexpected error: %v", err)
	}
	for _, required := range []string{
		"sandbox allow-scripts allow-forms",
		"default-src 'none'",
		"form-action 'none'",
		"base-uri 'none'",
		"object-src 'none'",
		"script-src 'self' 'unsafe-inline'",
		"connect-src 'self' https://api.example.com:443",
		"frame-ancestors http://127.0.0.1:38429 tauri://localhost http://tauri.localhost",
	} {
		if !strings.Contains(policy, required) {
			t.Fatalf("policy %q missing %q", policy, required)
		}
	}
	if strings.Count(policy, "api.example.com:443") != 3 {
		t.Fatalf("policy includes a duplicate or missing origin: %q", policy)
	}
	if strings.Contains(policy, "'unsafe-eval'") || strings.Contains(policy, "script-src https://") {
		t.Fatalf("policy enables eval or remote scripts: %q", policy)
	}
}

func TestNormalizeNetworkOriginsRejectsNonHTTPSAndPaths(t *testing.T) {
	if _, err := NormalizeNetworkOrigins([]string{"http://example.com"}); err == nil {
		t.Fatal("NormalizeNetworkOrigins(http) succeeded, want error")
	}
	if _, err := NormalizeNetworkOrigins([]string{"https://example.com/path"}); err == nil {
		t.Fatal("NormalizeNetworkOrigins(path) succeeded, want error")
	}
}

func TestBuildContentSecurityPolicyRejectsWildcardFrameAncestors(t *testing.T) {
	for _, origin := range []string{"http://localhost:*", "http://*:38429", "http://*"} {
		if _, err := BuildContentSecurityPolicy(nil, []string{origin}); err == nil {
			t.Fatalf("BuildContentSecurityPolicy(%q) succeeded, want wildcard rejection", origin)
		}
	}
}
