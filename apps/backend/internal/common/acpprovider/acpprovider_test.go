package acpprovider

import "testing"

func TestBuildGatewayAuth_WithKey(t *testing.T) {
	gw := BuildGatewayAuth("gateway", "Kandev", "http://localhost:20128/v1", "sk-secret")
	if gw.MethodID != "gateway" {
		t.Fatalf("MethodID = %q", gw.MethodID)
	}
	meta, ok := gw.Meta["gateway"].(map[string]any)
	if !ok {
		t.Fatalf("meta.gateway missing: %#v", gw.Meta)
	}
	if meta["baseUrl"] != "http://localhost:20128/v1" {
		t.Errorf("baseUrl = %v", meta["baseUrl"])
	}
	if meta["providerName"] != "Kandev" {
		t.Errorf("providerName = %v", meta["providerName"])
	}
	headers, ok := meta["headers"].(map[string]any)
	if !ok || headers["Authorization"] != "Bearer sk-secret" {
		t.Errorf("headers = %v", meta["headers"])
	}
}

func TestBuildGatewayAuth_NoKeyOmitsHeaders(t *testing.T) {
	gw := BuildGatewayAuth("gateway", "", "https://router.example/v1", "")
	meta := gw.Meta["gateway"].(map[string]any)
	if _, present := meta["headers"]; present {
		t.Errorf("headers should be omitted without a key: %v", meta)
	}
	if _, present := meta["providerName"]; present {
		t.Errorf("providerName should be omitted when empty: %v", meta)
	}
}

func TestValidateBaseURL(t *testing.T) {
	for _, ok := range []string{"http://localhost:20128/v1", "https://r.example/v1"} {
		if err := ValidateBaseURL(ok); err != nil {
			t.Errorf("ValidateBaseURL(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "   ", "/v1", "ftp://h/v1", "localhost:20128", "http://"} {
		if err := ValidateBaseURL(bad); err == nil {
			t.Errorf("ValidateBaseURL(%q) = nil, want error", bad)
		}
	}
}

func TestClientAuthMeta(t *testing.T) {
	if ClientAuthMeta()["gateway"] != true {
		t.Fatal("ClientAuthMeta must advertise gateway:true")
	}
}

func TestValidateCredentialedBaseURL(t *testing.T) {
	ok := []string{
		"https://router.example/v1", "http://localhost:20128/v1",
		"http://127.0.0.1/v1", "http://[::1]:8080/v1",
	}
	for _, raw := range ok {
		if err := ValidateCredentialedBaseURL(raw); err != nil {
			t.Errorf("ValidateCredentialedBaseURL(%q) = %v, want nil", raw, err)
		}
	}
	bad := []string{"http://router.example/v1", "http://10.0.0.4:9000/v1", "http://host.docker.internal/v1", ""}
	for _, raw := range bad {
		if err := ValidateCredentialedBaseURL(raw); err == nil {
			t.Errorf("ValidateCredentialedBaseURL(%q) = nil, want error", raw)
		}
	}
}

func TestIsLoopbackBaseURL(t *testing.T) {
	loopback := []string{
		"http://localhost:20128/v1", "http://127.0.0.1:20128/v1",
		"https://127.5.5.5/v1", "http://[::1]:8080/v1", "http://LocalHost/v1",
	}
	for _, raw := range loopback {
		if !IsLoopbackBaseURL(raw) {
			t.Errorf("IsLoopbackBaseURL(%q) = false, want true", raw)
		}
	}
	for _, raw := range []string{"https://router.example/v1", "http://10.0.0.4:9000/v1", "http://host.docker.internal/v1"} {
		if IsLoopbackBaseURL(raw) {
			t.Errorf("IsLoopbackBaseURL(%q) = true, want false", raw)
		}
	}
}

func TestRewriteLoopbackHostForDocker(t *testing.T) {
	got, rewritten := RewriteLoopbackHostForDocker("http://localhost:20128/v1")
	if !rewritten || got != "http://host.docker.internal:20128/v1" {
		t.Errorf("rewrite with port = (%q, %v)", got, rewritten)
	}
	got, rewritten = RewriteLoopbackHostForDocker("http://127.0.0.1/v1")
	if !rewritten || got != "http://host.docker.internal/v1" {
		t.Errorf("rewrite no port = (%q, %v)", got, rewritten)
	}
	got, rewritten = RewriteLoopbackHostForDocker("https://router.example/v1")
	if rewritten || got != "https://router.example/v1" {
		t.Errorf("non-loopback should be untouched = (%q, %v)", got, rewritten)
	}
}
