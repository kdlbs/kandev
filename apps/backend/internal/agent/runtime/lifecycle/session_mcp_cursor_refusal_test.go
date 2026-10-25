package lifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// With a signing key present, Cursor and Pi would receive a Kandev entry that
// carries no token, so project MCP materialization refuses before writing.
func TestProjectMCPRefusesDeferredCarrierWithSigningKey(t *testing.T) {
	for _, tc := range []struct{ agentID, dir string }{
		{"cursor-acp", ".cursor"},
		{"pi-acp", ".pi"},
	} {
		t.Run(tc.agentID, func(t *testing.T) {
			mgr := newTestManager(t)
			mgr.secretStore = &keyedStore{key: launchKey()}
			execution := &AgentExecution{
				ID: "exec-1", TaskID: "task-1", SessionID: "session-1", AgentProfileID: "profile-1",
				WorkspacePath: t.TempDir(), ExecutorType: "local",
				metadata: map[string]interface{}{}, standalonePort: 45678,
			}
			agentConfig, ok := mgr.registry.Get(tc.agentID)
			if !ok {
				t.Fatalf("%s missing from test registry", tc.agentID)
			}
			profile := &AgentProfileInfo{ProfileID: "profile-1", CursorMCPAuthEnabled: true}

			err := mgr.materializeRuntimeProjectMCP(context.Background(), execution, agentConfig, profile, "local")

			var refusal *LaunchRefusal
			if !errors.As(err, &refusal) || refusal.Code != CodeMCPTokenCarrierUnavailable ||
				refusal.Cause != CauseWorkspaceFileCarrierDeferred {
				t.Fatalf("err = %v, want carrier_unavailable/%s", err, CauseWorkspaceFileCarrierDeferred)
			}
			if _, statErr := os.Stat(filepath.Join(execution.WorkspacePath, tc.dir)); !os.IsNotExist(statErr) {
				t.Fatalf("workspace file written despite refusal: %v", statErr)
			}
		})
	}
}
