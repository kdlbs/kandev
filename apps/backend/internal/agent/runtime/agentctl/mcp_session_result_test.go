package client

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestMCPSessionResultCarriesExactRequestMetadataAndIdentity(t *testing.T) {
	for _, action := range []string{"new", "reset", "load", "resume"} {
		t.Run(action, func(t *testing.T) {
			var received []streams.MCPResolutionEntry
			client, server := newTestClientWithStream(t, func(message ws.Message) *ws.Message {
				var payload struct {
					Resolution []streams.MCPResolutionEntry `json:"mcp_resolution"`
				}
				if err := json.Unmarshal(message.Payload, &payload); err != nil {
					t.Error(err)
				}
				received = payload.Resolution
				response, _ := ws.NewResponse(message.ID, message.Action, map[string]any{"success": true, "session_id": "native", "attachment_attempt_id": "actual-" + action})
				return response
			})
			defer server.Close()
			defer client.Close()
			ctx := streams.WithMCPResolution(context.Background(), []streams.MCPResolutionEntry{{RuntimeName: "protected", DefinitionID: "definition", ReasonCode: "secret_resolution_failed"}})
			var id string
			var err error
			switch action {
			case "new":
				var result NewSessionResponse
				result, err = client.NewSessionWithResult(ctx, "/workspace", nil)
				id = result.AttachmentAttemptID
			case "reset":
				var result NewSessionResponse
				result, err = client.ResetSessionWithResult(ctx, "/workspace", nil)
				id = result.AttachmentAttemptID
			case "load":
				id, err = client.LoadSessionWithResult(ctx, "native", nil, streams.SessionSettingsPolicyStrict)
			case "resume":
				id, err = client.ResumeSessionWithResult(ctx, "native", "/workspace", nil)
			}
			client.setLastAttachmentAttemptID("unrelated-later-request")
			if err != nil || id != "actual-"+action || len(received) != 1 || received[0].DefinitionID != "definition" {
				t.Fatalf("request result: %s %v %+v", id, err, received)
			}
		})
	}
}
