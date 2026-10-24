package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestBuiltinPromotionBindsWorkspaceBeforeCommand(t *testing.T) {
	for _, supported := range []bool{false, true} {
		t.Run(map[bool]string{false: "old", true: "supported"}[supported], func(t *testing.T) {
			calls := 0
			helper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.Equal(t, "/api/v1/mcp/builtin-binding", r.URL.Path)
				if !supported {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				var profile p.Context
				require.NoError(t, json.NewDecoder(r.Body).Decode(&profile))
				_ = json.NewEncoder(w).Encode(profile)
			}))
			t.Cleanup(helper.Close)
			_, execution := workspaceSourceTestManager(t, helper.URL, nil)
			mgr := newTestManager(t)
			require.NoError(t, mgr.executionStore.Add(execution))
			execution.TaskID, execution.WorkspaceID = "task-a", "workspace"
			mgr.profileResolver = &countingProfileResolver{info: &AgentProfileInfo{ProfileID: "profile", AgentName: "auggie"}}
			profile := builtinDispatchProfile(p.PackageCore, p.PackageConfiguration)
			req := &LaunchRequest{TaskID: execution.TaskID, SessionID: execution.SessionID, WorkspaceID: execution.WorkspaceID, AgentProfileID: "profile", IsPassthrough: true, McpProfile: &profile}
			err := mgr.promoteWorkspaceExecution(context.Background(), execution, req)
			if supported {
				require.NoError(t, err)
				require.NotEmpty(t, execution.AgentCommand)
				value, present := execution.metadataValue(p.BuiltinBindingMetadataKey)
				require.True(t, present)
				bound, err := p.ParseBuiltinBindingMetadata(value)
				require.NoError(t, err)
				expected, err := p.MarshalBuiltinBinding(profile)
				require.NoError(t, err)
				actual, err := p.MarshalBuiltinBinding(*bound)
				require.NoError(t, err)
				require.Equal(t, expected, actual)
			} else {
				require.Error(t, err)
				require.Empty(t, execution.AgentCommand)
			}
			require.Equal(t, 1, calls)
		})
	}
}
