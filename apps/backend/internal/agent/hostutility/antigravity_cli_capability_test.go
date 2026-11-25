package hostutility

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
)

func TestAntigravityUtilityRequiresCLIHelperCapability(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX CLI script")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("KANDEV_HOME_DIR", t.TempDir())
	binDir := t.TempDir()
	cliPath := filepath.Join(binDir, "agy")
	if err := os.WriteFile(cliPath, []byte(hostUtilityAntigravityCLIScript("1.5.15")), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	for _, tc := range []struct {
		name         string
		capabilities string
		wantError    bool
	}{
		{
			name:         "retained personal-auth helper is rejected",
			capabilities: `"antigravity_personal_auth_v1"`,
			wantError:    true,
		},
		{
			name:         "CLI-capable helper is accepted",
			capabilities: `"antigravity_personal_auth_v1","antigravity_cli_runtime_v1"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/identity" {
					_, _ = w.Write([]byte(`{"capabilities":[` + tc.capabilities + `]}`))
				}
			}))
			t.Cleanup(server.Close)
			host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			port, err := strconv.Atoi(portText)
			if err != nil {
				t.Fatal(err)
			}
			manager := &Manager{log: newTestLogger(t)}
			inst := &instance{
				agentType: agents.AntigravityACPAgentID,
				client:    agentctlclient.NewClient(host, port, manager.log),
			}
			command, _, release, err := manager.acquireInferenceCommand(
				context.Background(), inst, agents.NewAntigravityACP(), agents.Command{},
			)
			if err != nil {
				if !tc.wantError || !strings.Contains(err.Error(), acpcompat.AntigravityCLIRuntimeCapability) {
					t.Fatalf("acquireInferenceCommand error = %v", err)
				}
				return
			}
			defer release()
			if tc.wantError {
				t.Fatal("retained helper was accepted for CLI utility inference")
			}
			if got := command.Args(); len(got) < 2 || got[0] != cliPath || got[1] != "acp" {
				t.Fatalf("utility command = %v, want qualified CLI command", got)
			}
		})
	}
}
