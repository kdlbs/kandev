package loginpty

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
)

func TestAntigravityCLILoginUsesResolvedCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shared login endpoint requires a POSIX shell")
	}
	cliPath := antigravityLoginCLI(t, "1.5.15")
	agent := agents.NewAntigravityACP()
	command := agent.LoginCommand()
	if command == nil {
		t.Fatal("LoginCommand() = nil for a compatible CLI")
	}
	want := []string{cliPath, "acp", "--login"}
	if !reflect.DeepEqual(command.Cmd, want) {
		t.Fatalf("login command = %#v, want absolute argv %#v", command.Cmd, want)
	}

	mgr, router := antigravityLoginRouter(t, agent)
	response := antigravityLoginStart(router, "antigravity-cli")
	if response.Code != http.StatusOK {
		t.Fatalf("start status = %d: %s", response.Code, response.Body.String())
	}
	var status Status
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stopSession(t, mgr, status.ID) })
	if len(status.Cmd) < len(want) {
		t.Fatalf("PTY command = %#v, want wrapped CLI command", status.Cmd)
	}
	if got := status.Cmd[len(status.Cmd)-len(want):]; !reflect.DeepEqual(got, want) {
		t.Fatalf("PTY command suffix = %#v, want %#v", got, want)
	}
}

func TestAntigravityCLILoginRejectsChangedRuntime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shared login endpoint requires a POSIX shell")
	}
	cliPath := antigravityLoginCLI(t, "1.5.15")
	agent := agents.NewAntigravityACP()
	if agent.LoginCommand() == nil {
		t.Fatal("LoginCommand() = nil before runtime change")
	}
	if err := os.WriteFile(cliPath, []byte(antigravityLoginCLIScript("1.4.9")), 0o755); err != nil {
		t.Fatal(err)
	}
	mgr, router := antigravityLoginRouter(t, agent)
	response := antigravityLoginStart(router, "antigravity-cli")
	if response.Code != http.StatusConflict {
		t.Fatalf("stale login status = %d, want 409: %s", response.Code, response.Body.String())
	}
	var payload map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["error_code"] != "login_runtime_changed" {
		t.Fatalf("error_code = %q, want login_runtime_changed", payload["error_code"])
	}
	if len(mgr.sessions) != 0 {
		t.Fatalf("started %d login PTYs after runtime changed", len(mgr.sessions))
	}
}

func antigravityLoginCLI(t *testing.T, version string) string {
	t.Helper()
	t.Setenv("KANDEV_HOME_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	dir := filepath.Join(t.TempDir(), "Antigravity CLI")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "agy")
	if err := os.WriteFile(path, []byte(antigravityLoginCLIScript(version)), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SHELL", "/bin/sh")
	return path
}

func antigravityLoginCLIScript(version string) string {
	return fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo 'agy %s'; exit 0; fi\n"+
		"if [ \"$1\" = \"acp\" ] && [ \"$2\" = \"--help\" ]; then printf 'Usage: agy acp [flags]\\n\\nFlags:\\n  --process-per-session  Serve each session from an isolated worker process\\n'; exit 0; fi\nexit 0\n", version)
}

func antigravityLoginRouter(t *testing.T, agent *agents.AntigravityACP) (*Manager, *gin.Engine) {
	t.Helper()
	mgr := newTestManager(t, nil)
	t.Cleanup(func() { _ = mgr.StopAll() })
	reg := registry.NewRegistry(mgr.log)
	if err := reg.Register(agent); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandlers(mgr, reg, mgr.log.Zap(), nil).RegisterRoutes(router)
	return mgr, router
}

func antigravityLoginStart(router http.Handler, variant string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"command_variant": variant})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/agent-login/agents/antigravity-acp/start", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
