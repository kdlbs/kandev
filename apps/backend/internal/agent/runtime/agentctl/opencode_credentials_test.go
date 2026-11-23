package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// @covers AC-EXECUTORS-OPENCODE-AUTH-001.5
func TestOpenCodeCredentialOperationUsesProvisioningDeadline(t *testing.T) {
	for _, operation := range []string{"status", "import"} {
		t.Run(operation, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/identity" {
					_, _ = io.WriteString(w, `{"capabilities":["opencode-credential-bootstrap.v1"]}`)
					return
				}
				time.Sleep(100 * time.Millisecond)
				if r.Method == http.MethodGet {
					_, _ = io.WriteString(w, `{"version":1,"state":"empty"}`)
				} else {
					_, _ = io.WriteString(w, `{"success":true}`)
				}
			}))
			defer server.Close()
			client := newHTTPOnlyClient(server.URL)
			client.httpClient.Timeout = 25 * time.Millisecond
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			var err error
			if operation == "status" {
				_, err = client.OpenCodeCredentialStatus(ctx)
			} else {
				err = client.ImportOpenCodeCredentials(ctx, []byte(`[]`))
			}
			if err != nil {
				t.Fatalf("provisioning was cut short by the ordinary RPC timeout: %v", err)
			}
		})
	}
}

// @covers AC-EXECUTORS-OPENCODE-AUTH-001.7
func TestOpenCodeCredentialStatusProtocol(t *testing.T) {
	for _, tc := range []struct{ body, state string }{
		{`{"version":1,"state":"empty"}`, "empty"},
		{`{"version":1,"state":"present"}`, "present"},
		{`{"version":1,"state":"pending"}`, "pending"},
		{`{"version":2,"state":"present"}`, ""},
		{`{"version":1,"state":"synthetic-secret"}`, ""},
		{`synthetic-secret`, ""},
	} {
		t.Run(tc.body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/agent/opencode/credentials" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			state, err := newHTTPOnlyClient(server.URL).OpenCodeCredentialStatus(context.Background())
			if state != tc.state || (err == nil) != (tc.state != "") {
				t.Fatalf("state=%q err=%v", state, err)
			}
			if err != nil && strings.Contains(err.Error(), "synthetic-secret") {
				t.Fatal("status response leaked")
			}
		})
	}
}

func TestImportOpenCodeCredentialsRequiresAcknowledgement(t *testing.T) {
	for _, body := range []string{`{"success":false}`, `synthetic-secret`, `{"success":true}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/identity" {
					_, _ = io.WriteString(w, `{"capabilities":["opencode-credential-bootstrap.v1"]}`)
					return
				}
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			err := newHTTPOnlyClient(server.URL).ImportOpenCodeCredentials(context.Background(), []byte(`[]`))
			if (err == nil) != (body == `{"success":true}`) {
				t.Fatalf("unexpected import result: %v", err)
			}
			if err != nil && strings.Contains(err.Error(), "synthetic-secret") {
				t.Fatal("peer output leaked into diagnostics")
			}
		})
	}
}

// @covers AC-EXECUTORS-OPENCODE-AUTH-001.9
func TestOpenCodeCredentialCapabilityBeforeDelivery(t *testing.T) {
	for _, identity := range []string{`{}`, `{"capabilities":[]}`, `synthetic-secret`, `{"capabilities":["opencode-credential-bootstrap.v1"]}`} {
		t.Run(identity, func(t *testing.T) {
			delivered := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/identity" {
					_, _ = io.WriteString(w, identity)
					return
				}
				delivered = true
				_, _ = io.WriteString(w, `{"success":true}`)
			}))
			defer server.Close()
			err := newHTTPOnlyClient(server.URL).ImportOpenCodeCredentials(context.Background(), []byte(`[]`))
			supported := strings.Contains(identity, "opencode-credential-bootstrap.v1")
			if delivered != supported || (err == nil) != supported {
				t.Fatalf("supported=%v delivered=%v err=%v", supported, delivered, err)
			}
			if err != nil && strings.Contains(err.Error(), "synthetic-secret") {
				t.Fatal("identity response leaked")
			}
		})
	}
}
