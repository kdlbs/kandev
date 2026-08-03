package client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExportACPDebugRequestsExactSessionAndBound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/debug/acp/acp-session-1/export" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("max_bytes") != "4096" {
			t.Errorf("max_bytes = %q", r.URL.Query().Get("max_bytes"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("zip-bytes"))
	}))
	defer server.Close()

	client := &Client{baseURL: server.URL, httpClient: server.Client()}
	response, err := client.ExportACPDebug(t.Context(), "acp-session-1", 4096)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response)
	_ = response.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "zip-bytes" {
		t.Fatalf("body = %q", body)
	}
}
