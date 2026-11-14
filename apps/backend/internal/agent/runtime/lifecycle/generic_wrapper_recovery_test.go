package lifecycle

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/launchwrapper"
)

func TestGenericWrapperRecoveryUsesRetainedTemplate(t *testing.T) {
	for _, state := range []string{"valid", "missing", "unwrapped", "legacy"} {
		t.Run(state, func(t *testing.T) {
			retained := launchwrapper.Profile([]string{"retained-wrapper", launchwrapper.WorkDir})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if state == "legacy" {
					http.NotFound(w, r)
					return
				}
				if r.URL.Path == "/identity" {
					_, _ = w.Write([]byte(`{"capabilities":["launch-wrapper.v1"]}`))
					return
				}
				if state == "missing" {
					w.WriteHeader(http.StatusConflict)
					return
				}
				if state == "unwrapped" {
					_, _ = w.Write([]byte(`{"version":1}`))
					return
				}
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"version": 1, "launch_wrapper": retained}))
			}))
			defer server.Close()
			host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
			require.NoError(t, err)
			port, err := strconv.Atoi(portText)
			require.NoError(t, err)
			execution := &AgentExecution{agentctl: agentctlclient.NewClient(host, port, newTestLogger())}
			got, err := recoveredLaunchWrapper(context.Background(), execution, launchwrapper.Profile([]string{"edited-wrapper"}))
			switch state {
			case "valid":
				require.NoError(t, err)
				require.Equal(t, retained, got)
			case "unwrapped":
				require.NoError(t, err)
				require.True(t, got.IsZero())
			default:
				require.Error(t, err)
			}
		})
	}
}
