package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestBuiltinPassthroughAcceptedEvidenceUsesSealedBinding(t *testing.T) {
	for _, outcome := range []string{"accepted", "failed", "superseded"} {
		t.Run(outcome, func(t *testing.T) {
			manager, eventBus := createTestManagerWithTracking()
			execution := createTestExecution("execution", "task-a", "session")
			execution.WorkspaceID = "workspace"
			profile := builtinDispatchProfile(p.PackageCore, p.PackageConfiguration)
			encoded, err := p.MarshalBuiltinBinding(profile)
			require.NoError(t, err)
			execution.setMetadataValue(p.BuiltinBindingMetadataKey, encoded)
			require.NoError(t, manager.executionStore.Add(execution))
			operation := &preparedMCPOperation{servers: []types.McpServer{
				{Name: "kandev", Type: "http", URL: "http://127.0.0.1:1234/mcp", BuiltinPackage: p.PackageCore},
				{Name: "kandev-configuration", Type: "http", URL: "http://127.0.0.1:1234/mcp/packages/configuration", BuiltinPackage: p.PackageConfiguration},
			}}
			runner := &mcpOperationRunner{start: func() (*process.InteractiveProcessInfo, error) {
				if outcome == "failed" {
					return nil, errors.New("start failed")
				}
				if outcome == "superseded" {
					execution.beginStartupAttempt()
				}
				return &process.InteractiveProcessInfo{ID: "process", SessionID: "session"}, nil
			}}
			_, err = manager.startPassthroughWithMCP(context.Background(), execution, runner, process.InteractiveStartRequest{}, operation)
			if outcome != "accepted" {
				require.Empty(t, eventBus.getStreamEvents())
				require.Equal(t, outcome == "failed", err != nil)
				return
			}
			require.NoError(t, err)
			events := eventBus.getStreamEvents()
			require.Len(t, events, 1)
			var history streams.MCPAttachmentHistory
			history.StartAttempt(*events[0].Data.MCPAttachmentAttempt)
			require.NotNil(t, history.BuiltinBinding)
			expected, parseErr := p.ParseBuiltinBindingMetadata(encoded)
			require.NoError(t, parseErr)
			require.Equal(t, expected.BuiltinPackages, history.BuiltinBinding.Manifest)
			for _, server := range history.Current.Servers {
				require.Nil(t, server.ConnectedAt)
			}
		})
	}
}

type mcpOperationRunner struct {
	executionPassthroughRunner
	start func() (*process.InteractiveProcessInfo, error)
}

func (r *mcpOperationRunner) Start(context.Context, process.InteractiveStartRequest) (*process.InteractiveProcessInfo, error) {
	return r.start()
}

func TestMCPPassthroughEvidenceUsesConfirmedOperation(t *testing.T) {
	for _, outcome := range []string{"accepted", "failed", "superseded"} {
		t.Run(outcome, func(t *testing.T) {
			manager, eventBus := createTestManagerWithTracking()
			execution := createTestExecution("execution", "task", "session")
			if err := manager.executionStore.Add(execution); err != nil {
				t.Fatal(err)
			}
			repository := newSessionMCPStateFake()
			manager.SetMCPSelectionStateRepository(repository)
			operation := prepareMCPResolution(&mcpconfig.EffectiveMCPResolution{DesiredRevision: 2, Setup: &mcpconfig.MCPSetupSnapshot{Version: 1, Servers: []mcpconfig.MCPSetupServer{}},
				Servers:   []mcpconfig.ResolvedServer{{Name: "healthy", Type: mcpconfig.ServerTypeHTTP, URL: "https://user:password@example.test/mcp?token=private"}},
				Decisions: []mcpconfig.MCPResolutionDecision{{RuntimeName: "protected", DefinitionID: "definition", ReasonCode: "secret_resolution_failed"}},
			})
			runner := &mcpOperationRunner{start: func() (*process.InteractiveProcessInfo, error) {
				if outcome == "failed" {
					return nil, errors.New("start failed")
				}
				if outcome == "superseded" {
					execution.beginStartupAttempt()
				}
				return &process.InteractiveProcessInfo{ID: "process", SessionID: "session"}, nil
			}}
			_, err := manager.startPassthroughWithMCP(context.Background(), execution, runner, process.InteractiveStartRequest{}, operation)
			events := eventBus.getStreamEvents()
			state, _ := repository.GetMCPSelectionState(context.Background(), "session")
			if outcome != "accepted" {
				if len(events) != 0 || state.AppliedRevision != 0 {
					t.Fatal("unsuccessful or superseded start claimed delivery")
				}
				if (outcome == "failed") != (err != nil) {
					t.Fatalf("start error=%v", err)
				}
				return
			}
			if err != nil || len(events) != 1 {
				t.Fatalf("start evidence: %v %+v", err, events)
			}
			attempt := events[0].Data.MCPAttachmentAttempt
			if attempt == nil || state.AttachmentAttemptID != attempt.AttemptID || state.AppliedRevision != 2 {
				t.Fatalf("accepted identity: %+v %+v", attempt, state)
			}
			if len(attempt.Servers) != 2 || attempt.Servers[0].Status != streams.MCPAttachmentStatusDelivered || attempt.Servers[1].Status != streams.MCPAttachmentStatusUnavailable {
				t.Fatalf("delivery evidence: %+v", attempt.Servers)
			}
			encoded, _ := json.Marshal(attempt)
			if strings.Contains(string(encoded), "password") || strings.Contains(string(encoded), "private") || strings.Contains(string(encoded), "token=") {
				t.Fatal("secret entered attempt")
			}
		})
	}
}

func TestMCPPassthroughReservedServerIsNotDelivered(t *testing.T) {
	operation := prepareMCPResolution(&mcpconfig.EffectiveMCPResolution{Servers: []mcpconfig.ResolvedServer{{Name: "workspace", Type: mcpconfig.ServerTypeStdio, Command: "node", DefinitionID: "reserved"}, {Name: "healthy", Type: mcpconfig.ServerTypeStdio, Command: "node", DefinitionID: "healthy"}}})
	filterPassthroughMCPServers(operation, mcpconfig.ClaudeStrategy{})
	if len(operation.servers) != 1 || operation.servers[0].Name != "healthy" {
		t.Fatal("reserved entry entered provider config")
	}
	attempt := buildPassthroughMCPAttempt(&AgentExecution{ID: "execution"}, "process", operation)
	if len(attempt.Servers) != 2 || attempt.Servers[0].Name != "healthy" || attempt.Servers[0].Status != streams.MCPAttachmentStatusDelivered || attempt.Servers[1].Status != streams.MCPAttachmentStatusFiltered {
		t.Fatalf("reserved entry evidence: %+v", attempt.Servers)
	}
}
