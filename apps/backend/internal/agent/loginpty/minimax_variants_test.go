package loginpty

import (
	"bytes"
	"encoding/json"
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

// @covers AC-AGENTS-MINIMAX-001.2
func TestMiniMaxLoginCommandVariants(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shared login endpoint requires a POSIX shell")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "mcode"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/sh")
	for _, variant := range []string{"cn", "global", "", "global;echo unsafe"} {
		t.Run(variant, func(t *testing.T) {
			mgr := newTestManager(t, nil)
			t.Cleanup(func() { _ = mgr.StopAll() })
			reg := registry.NewRegistry(mgr.log)
			if err := reg.Register(agents.NewMiniMaxACP()); err != nil {
				t.Fatal(err)
			}
			gin.SetMode(gin.TestMode)
			router := gin.New()
			NewHandlers(mgr, reg, mgr.log.Zap(), nil).RegisterRoutes(router)
			body, err := json.Marshal(map[string]string{"command_variant": variant})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-login/agents/minimax-acp/start", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if variant == "global;echo unsafe" {
				if response.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, want 400", response.Code)
				}
				if len(mgr.sessions) != 0 {
					t.Fatal("unknown variant started a process")
				}
				return
			}
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", response.Code, response.Body.String())
			}
			var status Status
			if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
				t.Fatal(err)
			}
			want := agents.NewMiniMaxACP().LoginCommand().Cmd
			if variant != "" {
				want = append(want, "--region", variant)
			}
			if !reflect.DeepEqual(status.Cmd[len(status.Cmd)-len(want):], want) {
				t.Fatalf("command = %q, want suffix %q", status.Cmd, want)
			}
		})
	}
}
