package api

import "testing"

func TestInjectKandevMcpServersUsesConfiguredHost(t *testing.T) {
	srv := newTestServer(t)
	srv.cfg.MCPHost = "192.0.2.10"
	srv.cfg.Port = 43210

	got := srv.injectKandevMcpServers(nil)
	if got[0].URL != "http://192.0.2.10:43210/mcp" {
		t.Fatalf("injected HTTP URL = %q, want configured listener host", got[0].URL)
	}
	if got[1].URL != "http://192.0.2.10:43210/sse" {
		t.Fatalf("injected SSE URL = %q, want configured listener host", got[1].URL)
	}
}
