package nomad

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeServerCA stores the httptest TLS server certificate as a PEM CA file.
func writeServerCA(t *testing.T, server *httptest.Server) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ca.pem")
	cert := server.Certificate()
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestClientWithCAFileTrustsPrivateCA(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	plain, err := NewClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plain.http.Get(server.URL); err == nil {
		t.Fatal("client without ca_file trusted a private CA")
	}

	client, err := NewClient(server.URL, "token", WithCAFile(writeServerCA(t, server)))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.http.Get(server.URL)
	if err != nil {
		t.Fatalf("client with ca_file rejected its own CA: %v", err)
	}
	_ = resp.Body.Close()
}

func TestNewClientRejectsBadCAFile(t *testing.T) {
	garbage := filepath.Join(t.TempDir(), "bad.pem")
	if err := os.WriteFile(garbage, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{"missing": filepath.Join(t.TempDir(), "nope.pem"), "garbage": garbage} {
		if _, err := NewClient("https://nomad.example.test:4646", "token", WithCAFile(path)); err == nil || !strings.Contains(err.Error(), "ca_file") {
			t.Fatalf("%s ca_file error = %v, want a ca_file error", name, err)
		}
	}
	if _, err := NewClient("http://127.0.0.1:4646", "token", WithCAFile(garbage)); err == nil {
		t.Fatal("ca_file accepted with an http address")
	}
}

func TestParseProfileCAFile(t *testing.T) {
	for name, build := range map[string]func() map[string]string{"pool": validPoolConfig, "legacy": validTestProfile} {
		cfg := build()
		cfg["ca_file"] = "/etc/kandev/nomad-ca.pem"
		profile, err := ParseProfileConfig(cfg)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if profile.CAFile != "/etc/kandev/nomad-ca.pem" {
			t.Fatalf("%s: CAFile = %q", name, profile.CAFile)
		}
		for _, bad := range []string{"relative/ca.pem", "/etc/kandev/../ca.pem", "/etc//ca.pem"} {
			cfg["ca_file"] = bad
			if _, err := ParseProfileConfig(cfg); err == nil {
				t.Fatalf("%s: ca_file %q accepted", name, bad)
			}
		}
		cfg["ca_file"] = "/etc/kandev/nomad-ca.pem"
		cfg["address"] = "http://127.0.0.1:4646"
		if _, err := ParseProfileConfig(cfg); err == nil {
			t.Fatalf("%s: ca_file accepted with an http address", name)
		}
	}
}
