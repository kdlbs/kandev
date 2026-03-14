package mcpconfig

import "testing"

func TestToACPServers_Stdio(t *testing.T) {
	resolved := []ResolvedServer{
		{
			Name:    "github",
			Type:    ServerTypeStdio,
			Command: "npx",
			Args:    []string{"-y", "@mcp/github"},
			Env:     map[string]string{"GITHUB_TOKEN": "secret123"},
		},
	}

	result := ToACPServers(resolved)

	if len(result) != 1 {
		t.Fatalf("expected 1 server, got %d", len(result))
	}
	srv := result[0]
	if srv.Name != "github" {
		t.Errorf("Name = %q, want %q", srv.Name, "github")
	}
	if srv.Type != "stdio" {
		t.Errorf("Type = %q, want %q", srv.Type, "stdio")
	}
	if srv.Command != "npx" {
		t.Errorf("Command = %q, want %q", srv.Command, "npx")
	}
	if len(srv.Args) != 2 || srv.Args[0] != "-y" || srv.Args[1] != "@mcp/github" {
		t.Errorf("Args = %v, want [-y @mcp/github]", srv.Args)
	}
	if srv.Env["GITHUB_TOKEN"] != "secret123" {
		t.Errorf("Env[GITHUB_TOKEN] = %q, want %q", srv.Env["GITHUB_TOKEN"], "secret123")
	}
}

func TestToACPServers_StdioEnvIsCopied(t *testing.T) {
	original := map[string]string{"KEY": "val"}
	resolved := []ResolvedServer{
		{Name: "srv", Type: ServerTypeStdio, Command: "cmd", Env: original},
	}

	result := ToACPServers(resolved)

	// Mutate the original to verify it was cloned
	original["KEY"] = "mutated"
	if result[0].Env["KEY"] != "val" {
		t.Error("Env was not cloned; mutation of source affected result")
	}
}

func TestToACPServers_SSE(t *testing.T) {
	resolved := []ResolvedServer{
		{
			Name:    "remote",
			Type:    ServerTypeSSE,
			URL:     "https://mcp.example.com/sse",
			Headers: map[string]string{"Authorization": "Bearer tok"},
		},
	}

	result := ToACPServers(resolved)

	if len(result) != 1 {
		t.Fatalf("expected 1 server, got %d", len(result))
	}
	srv := result[0]
	if srv.Type != "sse" {
		t.Errorf("Type = %q, want %q", srv.Type, "sse")
	}
	if srv.URL != "https://mcp.example.com/sse" {
		t.Errorf("URL = %q, want %q", srv.URL, "https://mcp.example.com/sse")
	}
	if srv.Headers["Authorization"] != "Bearer tok" {
		t.Errorf("Headers[Authorization] = %q, want %q", srv.Headers["Authorization"], "Bearer tok")
	}
}

func TestToACPServers_HTTP(t *testing.T) {
	resolved := []ResolvedServer{
		{
			Name:    "http-srv",
			Type:    ServerTypeHTTP,
			URL:     "https://mcp.example.com/http",
			Headers: map[string]string{"X-Api-Key": "key123"},
		},
	}

	result := ToACPServers(resolved)

	if len(result) != 1 {
		t.Fatalf("expected 1 server, got %d", len(result))
	}
	if result[0].Type != "http" {
		t.Errorf("Type = %q, want %q", result[0].Type, "http")
	}
	if result[0].Headers["X-Api-Key"] != "key123" {
		t.Errorf("Headers[X-Api-Key] = %q, want %q", result[0].Headers["X-Api-Key"], "key123")
	}
}

func TestToACPServers_StreamableHTTP(t *testing.T) {
	resolved := []ResolvedServer{
		{
			Name: "stream-srv",
			Type: ServerTypeStreamableHTTP,
			URL:  "https://mcp.example.com/stream",
		},
	}

	result := ToACPServers(resolved)

	if len(result) != 1 {
		t.Fatalf("expected 1 server, got %d", len(result))
	}
	if result[0].Type != "streamable_http" {
		t.Errorf("Type = %q, want %q", result[0].Type, "streamable_http")
	}
	if result[0].URL != "https://mcp.example.com/stream" {
		t.Errorf("URL = %q, want %q", result[0].URL, "https://mcp.example.com/stream")
	}
}

func TestToACPServers_MixedTypes(t *testing.T) {
	resolved := []ResolvedServer{
		{Name: "stdio-srv", Type: ServerTypeStdio, Command: "cmd"},
		{Name: "sse-srv", Type: ServerTypeSSE, URL: "https://sse"},
		{Name: "http-srv", Type: ServerTypeHTTP, URL: "https://http"},
		{Name: "stream-srv", Type: ServerTypeStreamableHTTP, URL: "https://stream"},
	}

	result := ToACPServers(resolved)

	if len(result) != 4 {
		t.Fatalf("expected 4 servers, got %d", len(result))
	}

	expected := []struct {
		name, typ string
	}{
		{"stdio-srv", "stdio"},
		{"sse-srv", "sse"},
		{"http-srv", "http"},
		{"stream-srv", "streamable_http"},
	}
	for i, exp := range expected {
		if result[i].Name != exp.name {
			t.Errorf("result[%d].Name = %q, want %q", i, result[i].Name, exp.name)
		}
		if result[i].Type != exp.typ {
			t.Errorf("result[%d].Type = %q, want %q", i, result[i].Type, exp.typ)
		}
	}
}

func TestToACPServers_Empty(t *testing.T) {
	result := ToACPServers(nil)
	if len(result) != 0 {
		t.Errorf("expected empty result for nil input, got %d", len(result))
	}

	result = ToACPServers([]ResolvedServer{})
	if len(result) != 0 {
		t.Errorf("expected empty result for empty input, got %d", len(result))
	}
}

func TestToACPServers_NilEnvAndHeaders(t *testing.T) {
	resolved := []ResolvedServer{
		{Name: "no-env", Type: ServerTypeStdio, Command: "cmd", Env: nil},
		{Name: "no-headers", Type: ServerTypeSSE, URL: "https://sse", Headers: nil},
	}

	result := ToACPServers(resolved)

	if len(result) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(result))
	}
	// nil maps should remain nil (cloneStringMap returns nil for nil input)
	if result[0].Env != nil {
		t.Errorf("expected nil Env for stdio server with nil env, got %v", result[0].Env)
	}
	if result[1].Headers != nil {
		t.Errorf("expected nil Headers for SSE server with nil headers, got %v", result[1].Headers)
	}
}

func TestToACPServers_ArgsAreCopied(t *testing.T) {
	original := []string{"arg1", "arg2"}
	resolved := []ResolvedServer{
		{Name: "srv", Type: ServerTypeStdio, Command: "cmd", Args: original},
	}

	result := ToACPServers(resolved)

	// Mutate original to verify it was copied
	original[0] = "mutated"
	if result[0].Args[0] != "arg1" {
		t.Error("Args were not copied; mutation of source affected result")
	}
}
