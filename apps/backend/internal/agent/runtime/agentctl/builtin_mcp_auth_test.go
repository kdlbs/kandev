package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
)

func TestCreateInstanceRequestCarriesEnforcementFlag(t *testing.T) {
	t.Cleanup(func() { SetBuiltinMCPAuthEnforced(false) })
	for _, enforced := range []bool{true, false} {
		SetBuiltinMCPAuthEnforced(enforced)
		srv, got := captureServer(t, jsonResponder(http.StatusCreated, `{"id":"inst-1","port":41234}`))
		req := &CreateInstanceRequest{WorkspacePath: "/workspace/task-1"}
		if _, err := newTestControlClient(t, srv).CreateInstance(context.Background(), req); err != nil {
			t.Fatalf("CreateInstance: %v", err)
		}
		var body map[string]any
		if err := json.Unmarshal(got.Body, &body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		value, present := body["builtin_mcp_auth_enforced"].(bool)
		if enforced != (present && value) {
			t.Fatalf("enforced=%v: serialized flag = %#v", enforced, body["builtin_mcp_auth_enforced"])
		}
		if req.BuiltinMCPAuthEnforced {
			t.Fatal("CreateInstance must not mutate the caller's request")
		}
	}
}

func TestClientBuiltinMCPCredential(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		want    string
		wantErr bool
	}{
		{name: "current agentctl", status: http.StatusOK, body: `{"credential":"cred-1","enforced":true}`, want: "cred-1"},
		{name: "older agentctl without the route", status: http.StatusNotFound, body: `{}`},
		{name: "rejected control credential", status: http.StatusUnauthorized, body: `{}`, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var path string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(srv.Close)
			host, port := splitTestServerHostPort(t, srv)
			client := NewClient(host, port, newTestLogger())
			t.Cleanup(client.Close)
			got, err := client.BuiltinMCPCredential(context.Background())
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("credential = %q, want %q", got, tc.want)
			}
			if path != types.BuiltinMCPCredentialPath {
				t.Fatalf("path = %q", path)
			}
		})
	}
}
