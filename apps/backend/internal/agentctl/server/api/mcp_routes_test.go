package api

import "testing"

func TestMCPRoutesPreserveHTTPAndSSEPaths(t *testing.T) {
	s := newTestServerWithMCP(t)

	wants := map[string]bool{
		"GET /sse":      true,
		"POST /message": true,
		"POST /mcp":     true,
	}
	for _, route := range s.router.Routes() {
		delete(wants, route.Method+" "+route.Path)
	}
	if len(wants) != 0 {
		t.Fatalf("missing MCP routes: %v", wants)
	}
}
