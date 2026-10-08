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

// @covers AC-AGENTS-MINIMAX-001.2
func TestMiniMaxLoginReconnectPreservesSelectedCommand(t *testing.T) {
	for _, tc := range []struct {
		first, next string
		want        int
	}{
		{"", "", 200}, {"cn", "cn", 200}, {"global", "global", 200},
		{"", "global", 409}, {"cn", "global", 409}, {"global", "cn", 409},
		{"global", "", 409},
	} {
		t.Run(tc.first+"_to_"+tc.next, func(t *testing.T) {
			mgr, router := miniMaxLoginRouter(t)
			first := miniMaxLoginRequest(router, tc.first)
			if first.Code != 200 {
				t.Fatalf("initial status = %d: %s", first.Code, first.Body.String())
			}
			var original Status
			if err := json.Unmarshal(first.Body.Bytes(), &original); err != nil {
				t.Fatal(err)
			}
			next := miniMaxLoginRequest(router, tc.next)
			if next.Code != tc.want {
				t.Fatalf("reconnect status = %d, want %d: %s", next.Code, tc.want, next.Body.String())
			}
			var response struct {
				Status
				Code string `json:"error_code"`
			}
			if err := json.Unmarshal(next.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if tc.want == 409 && response.Code != "login_command_conflict" {
				t.Fatalf("unexpected error code: %s", next.Body.String())
			}
			if tc.want == 200 && response.ID != original.ID {
				t.Fatal("identical command did not reconnect")
			}
			live := mgr.GetByID(original.ID)
			if live == nil || !live.Status().Running || !reflect.DeepEqual(live.Status().Cmd, original.Cmd) {
				t.Fatal("original sign-in was replaced or stopped")
			}
		})
	}
}

func miniMaxLoginRouter(t *testing.T) (*Manager, *gin.Engine) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable fixture")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "mcode"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/sh")
	mgr := newTestManager(t, nil)
	t.Cleanup(func() { _ = mgr.StopAll() })
	reg := registry.NewRegistry(mgr.log)
	if err := reg.Register(agents.NewMiniMaxACP()); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandlers(mgr, reg, mgr.log.Zap(), nil).RegisterRoutes(router)
	return mgr, router
}

func miniMaxLoginRequest(router *gin.Engine, variant string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"command_variant": variant})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-login/agents/minimax-acp/start", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}
