package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestBuiltinSSHChecksHelperBeforeCreation(t *testing.T) {
	for _, supported := range []bool{false, true} {
		t.Run(map[bool]string{false: "old", true: "supported"}[supported], func(t *testing.T) {
			calls := 0
			helper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "Bearer token", r.Header.Get("Authorization"))
				if r.URL.Path == "/identity" {
					capabilities := []string{}
					if supported {
						capabilities = append(capabilities, p.BuiltinPackagesCapability)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"capabilities": capabilities, "builtin_mcp_catalog_version": 1})
					return
				}
				calls++
				_, _ = w.Write([]byte(`{"id":"instance","port":41234}`))
			}))
			t.Cleanup(helper.Close)
			ssh := newFakeSSHServer(t, nil)
			ssh.forwardTo(strings.TrimPrefix(helper.URL, "http://"))
			profile := builtinDispatchProfile(p.PackageCore)
			_, err := createRemoteAgentInstance(context.Background(), ssh.dial(t), 39429, "/workspace", "/agentctl", &ExecutorCreateRequest{TaskID: "task-a", McpProfile: &profile}, "token", newTestLogger())
			if supported {
				require.NoError(t, err)
				require.Equal(t, 1, calls)
			} else {
				require.Error(t, err)
				require.Zero(t, calls)
			}
		})
	}
}
