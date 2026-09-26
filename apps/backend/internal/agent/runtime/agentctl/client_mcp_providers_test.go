package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSetMcpProvidersSendsReplacementRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/mcp/providers" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Providers []string `json:"mcp_providers"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if got, want := body.Providers, []string{"github", "gitlab"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Fatalf("providers = %v, want %v", got, want)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &Client{baseURL: server.URL, httpClient: server.Client()}
	if err := client.SetMcpProviders(context.Background(), []string{"github", "gitlab"}); err != nil {
		t.Fatalf("SetMcpProviders: %v", err)
	}
}
