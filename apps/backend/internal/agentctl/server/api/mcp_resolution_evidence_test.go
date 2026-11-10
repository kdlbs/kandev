package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type resolutionCaptureAdapter struct{ mcpCaptureAdapter }

func (*resolutionCaptureAdapter) SupportsSessionResume() bool { return true }
func (*resolutionCaptureAdapter) SupportsSessionLoad() bool   { return true }
func (a *resolutionCaptureAdapter) ResumeSession(ctx context.Context, id string, servers []types.McpServer) error {
	return a.LoadSession(ctx, id, servers)
}
func (a *resolutionCaptureAdapter) ResetSession(ctx context.Context, servers []types.McpServer) (string, error) {
	return a.NewSession(ctx, servers)
}

func TestMCPResolutionExclusionIsEvidenceNotProviderConfiguration(t *testing.T) {
	for _, action := range []string{"new", "load", "resume", "reset"} {
		t.Run(action, func(t *testing.T) {
			log := newTestLogger()
			cfg := &config.InstanceConfig{WorkDir: t.TempDir(), InstanceID: "execution", TaskID: "task", SessionID: "session"}
			manager := process.NewManager(cfg, log)
			server := NewServer(cfg, manager, nil, nil, log)
			capture := &resolutionCaptureAdapter{}
			manager.SetAdapterForTest(capture)
			request, err := ws.NewRequest("request", "agent.session."+action, map[string]any{
				"session_id":  "native-session",
				"mcp_servers": []map[string]string{{"name": "healthy", "type": "http", "url": "https://example.test/mcp"}},
				"mcp_resolution": []map[string]any{
					{"runtime_name": "protected", "definition_id": "definition", "definition_revision": 2, "reason_code": "secret_resolution_failed", "summary": "must never trust raw secret=token", "origins": []map[string]string{{"scope": "repository", "workspace_id": "workspace", "owner_id": "repo"}}},
					{"runtime_name": "healthy", "definition_id": "healthy-definition", "definition_revision": 3},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			response := server.handleAgentStreamRequest(context.Background(), request)
			if response.Type != ws.MessageTypeResponse {
				t.Fatalf("response: %s", response.Payload)
			}
			servers := append([]types.McpServer(nil), capture.newSessionServers...)
			servers = append(servers, capture.loadSessionServers...)
			if len(servers) != 1 || servers[0].Name != "healthy" {
				t.Fatalf("provider servers: %+v", servers)
			}
			history := drainResolutionHistory(manager)
			var result struct {
				AttachmentAttemptID string `json:"attachment_attempt_id"`
			}
			if err := response.ParsePayload(&result); err != nil {
				t.Fatal(err)
			}
			if result.AttachmentAttemptID == "" || result.AttachmentAttemptID != history.Current.AttemptID {
				t.Fatalf("request identity: %+v", result)
			}
			found := false
			for _, row := range history.Current.Servers {
				if row.Name == "protected" && row.ReasonCode == "secret_resolution_failed" && row.Status == streams.MCPAttachmentStatusUnavailable && row.DefinitionID == "definition" && row.DefinitionRevision == 2 && len(row.Origins) == 1 {
					found = true
				}
				if row.Name == "healthy" && (row.DefinitionID != "healthy-definition" || row.DefinitionRevision != 3) {
					t.Fatalf("healthy identity: %+v", row)
				}
			}
			encoded, _ := json.Marshal(history)
			if !found || strings.Contains(string(encoded), "secret=token") {
				t.Fatalf("missing or unsafe exclusion: %s", encoded)
			}
		})
	}
}

func drainResolutionHistory(manager *process.Manager) streams.MCPAttachmentHistory {
	var history streams.MCPAttachmentHistory
	for {
		select {
		case event := <-manager.GetUpdates():
			if event.MCPAttachmentAttempt != nil {
				history.StartAttempt(*event.MCPAttachmentAttempt)
			}
			if event.MCPAttachment != nil {
				history.Apply(*event.MCPAttachment)
			}
		default:
			return history
		}
	}
}

func TestMCPResumeEnforcesManagedConversationPolicy(t *testing.T) {
	log := newTestLogger()
	policy := mcpprofile.New(mcpprofile.SurfaceManagedConversation, nil, nil)
	cfg := &config.InstanceConfig{WorkDir: t.TempDir(), McpProfile: &policy}
	manager := process.NewManager(cfg, log)
	capture := &resolutionCaptureAdapter{}
	manager.SetAdapterForTest(capture)
	server := NewServer(cfg, manager, nil, nil, log)
	request, _ := ws.NewRequest("request", "agent.session.resume", ResumeSessionRequest{SessionID: "native", McpServers: []types.McpServer{{Name: "external", Type: "http", URL: "https://example.test"}}})
	response := server.handleWSResumeSession(context.Background(), request)
	if response.Type != ws.MessageTypeError || len(capture.loadSessionServers) != 0 {
		t.Fatalf("resume bypassed managed policy: %s", response.Payload)
	}
	var payload ws.ErrorPayload
	if err := response.ParsePayload(&payload); err != nil || payload.Code != ws.ErrorCodeForbidden {
		t.Fatalf("policy response: %+v %v", payload, err)
	}
}

func TestMCPRemovedByAgentctlPolicyRemainsVisible(t *testing.T) {
	server := newTestServer(t)
	server.startMCPAttachmentAttempt(context.Background(), nil, []streams.MCPResolutionEntry{{RuntimeName: "external", DefinitionID: "definition"}})
	history := drainResolutionHistory(server.procMgr)
	if len(history.Current.Servers) != 1 || history.Current.Servers[0].Status != streams.MCPAttachmentStatusFiltered || history.Current.Servers[0].ReasonCode != "server_policy" {
		t.Fatalf("silent helper policy exclusion: %+v", history.Current.Servers)
	}
}
