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
