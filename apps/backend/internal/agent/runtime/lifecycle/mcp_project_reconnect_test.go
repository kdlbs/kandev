package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

type projectMCPProfile struct{ agentName string }

func (p projectMCPProfile) ResolveProfile(context.Context, string) (*AgentProfileInfo, error) {
	return &AgentProfileInfo{ProfileID: "profile-1", AgentName: p.agentName}, nil
}

type projectMCPResolution struct{ called bool }

func (r *projectMCPResolution) Resolve(context.Context, mcpconfig.ResolutionContext, mcpconfig.Policy) (*mcpconfig.EffectiveMCPResolution, error) {
	r.called = true
	return &mcpconfig.EffectiveMCPResolution{DesiredRevision: 2}, nil
}

func TestProjectMCPProvidersDeferUntilProcessRestart(t *testing.T) {
	for _, agentName := range []string{"cursor-acp", "pi-acp"} {
		for _, change := range []string{"add", "remove"} {
			t.Run(agentName+"/"+change, func(t *testing.T) {
				client, actions, cleanup := newSessionMCPClient(t, false, false, true)
				defer cleanup()
				manager := newTestManager(t)
				manager.profileResolver = projectMCPProfile{agentName: agentName}
				resolver := &projectMCPResolution{}
				manager.mcpResolver = resolver
				repository := newSessionMCPStateFake()
				repository.state["session-1"] = mcpconfig.SessionMCPSelectionState{DesiredRevision: 2, AppliedRevision: 1, AttachmentAttemptID: "before", ApplyState: mcpconfig.SessionMCPApplyStatePendingIdle}
				manager.SetMCPSelectionStateRepository(repository)
				execution := newSessionMCPExecution(client)
				execution.WorkspacePath = t.TempDir()
				directory := ".cursor"
				if agentName == "pi-acp" {
					directory = ".pi"
				}
				projectPath := filepath.Join(execution.WorkspacePath, directory, "mcp.json")
				original := `{"mcpServers":{}}`
				if change == "remove" {
					original = `{"mcpServers":{"old":{"command":"node"}}}`
				}
				if err := os.MkdirAll(filepath.Dir(projectPath), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(projectPath, []byte(original), 0600); err != nil {
					t.Fatal(err)
				}
				if err := manager.executionStore.Add(execution); err != nil {
					t.Fatal(err)
				}
				if err := manager.applyPendingSessionMCP(context.Background(), execution.SessionID); err != nil {
					t.Fatal(err)
				}
				state, _ := repository.GetMCPSelectionState(context.Background(), execution.SessionID)
				data, err := os.ReadFile(projectPath)
				if err != nil || string(data) != original || state.ApplyState != mcpconfig.SessionMCPApplyStateDeferredRestart || state.AppliedRevision != 1 || state.AttachmentAttemptID != "before" || resolver.called {
					t.Fatalf("project setup was prematurely accepted: %+v resolver=%v %v", state, resolver.called, err)
				}
				if action := <-actions; action != "agent.initialize" {
					t.Fatalf("initial action: %s", action)
				}
				select {
				case action := <-actions:
					t.Fatalf("unexpected provider call: %s", action)
				default:
				}
			})
		}
	}
}
