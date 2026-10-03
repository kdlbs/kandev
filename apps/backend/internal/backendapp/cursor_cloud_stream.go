package backendapp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime"
	cursorcloudruntime "github.com/kandev/kandev/internal/agent/runtime/cursorcloud"
	provider "github.com/kandev/kandev/internal/cursorcloud"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	cursorCloudEventToolCall   = "tool_call"
	cursorCloudEventToolUpdate = "tool_update"
)

func projectCursorCloudStream(
	_ context.Context,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
	event provider.StreamEvent,
) (*cursorcloudruntime.StreamProjection, error) {
	if binding == nil || operation == nil {
		return nil, fmt.Errorf("cursor cloud stream identity is incomplete")
	}
	data, supported, err := decodeCursorCloudStreamData(event)
	if err != nil || !supported {
		return nil, err
	}
	switch event.Type {
	case "assistant":
		return projectCursorCloudAssistant(binding, operation, data), nil
	case cursorCloudEventToolCall, cursorCloudEventToolUpdate:
		return projectCursorCloudTool(binding, operation, event.Type, data), nil
	case "result":
		return projectCursorCloudResult(binding, operation, data), nil
	default:
		return nil, nil
	}

}

func decodeCursorCloudStreamData(event provider.StreamEvent) (map[string]any, bool, error) {
	switch event.Type {
	case "assistant", cursorCloudEventToolCall, cursorCloudEventToolUpdate, "result":
	default:
		return nil, false, nil
	}
	var data map[string]any
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return nil, true, fmt.Errorf("decode Cursor Cloud stream event")
	}
	return data, true, nil
}

func projectCursorCloudAssistant(
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
	data map[string]any,
) *cursorcloudruntime.StreamProjection {
	text, _ := data["text"].(string)
	if text == "" {
		return nil
	}
	message := cursorCloudAssistantMessage(binding, operation, text)
	payload := cursorCloudStreamPayload(binding, operation, "message_streaming")
	payload.Data.Text = text
	return &cursorcloudruntime.StreamProjection{Message: message, Payload: payload, AppendMessage: true}
}

func projectCursorCloudTool(
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
	eventType string,
	data map[string]any,
) *cursorcloudruntime.StreamProjection {
	callID, _ := data["callId"].(string)
	if strings.TrimSpace(callID) == "" {
		return nil
	}
	name, _ := data["name"].(string)
	status, _ := data["status"].(string)
	metadata := map[string]interface{}{"call_id": callID, "name": name, "status": status}
	if markers := cursorCloudTruncationMarkers(data["truncated"]); len(markers) > 0 {
		metadata["truncated"] = markers
	}
	message := &models.Message{
		ID:            "cursor-cloud-tool-" + operation.ID + "-" + callID,
		TaskSessionID: binding.SessionID, TaskID: binding.TaskID,
		TurnID: operation.RequestSnapshot.TurnID, AuthorType: models.MessageAuthorAgent,
		AuthorID: binding.ExecutionID, Content: name, Type: models.MessageTypeToolCall,
		Metadata: metadata,
	}
	payload := cursorCloudStreamPayload(binding, operation, eventType)
	payload.Data.ToolCallID = callID
	payload.Data.ToolName = name
	payload.Data.ToolStatus = status
	payload.Data.Data = metadata
	return &cursorcloudruntime.StreamProjection{Message: message, Payload: payload}
}

func cursorCloudTruncationMarkers(raw any) map[string]bool {
	truncation, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	markers := make(map[string]bool, len(truncation))
	for key, value := range truncation {
		if truncated, ok := value.(bool); ok && truncated {
			markers[key] = true
		}
	}
	return markers
}

func projectCursorCloudResult(
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
	data map[string]any,
) *cursorcloudruntime.StreamProjection {
	text, _ := data["text"].(string)
	if text == "" {
		text, _ = data["result"].(string)
	}
	if text == "" {
		return nil
	}
	message := cursorCloudAssistantMessage(binding, operation, text)
	payload := cursorCloudStreamPayload(binding, operation, "message_streaming")
	payload.Data.Text = text
	status, _ := data["status"].(string)
	return &cursorcloudruntime.StreamProjection{Message: message, Payload: payload, TerminalStatus: status}
}

func cursorCloudAssistantMessage(
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
	text string,
) *models.Message {
	return &models.Message{
		ID:            "cursor-cloud-assistant-" + operation.ID,
		TaskSessionID: binding.SessionID, TaskID: binding.TaskID,
		TurnID: operation.RequestSnapshot.TurnID, AuthorType: models.MessageAuthorAgent,
		AuthorID: binding.ExecutionID, Content: text, Type: models.MessageTypeMessage,
	}
}

func cursorCloudStreamPayload(
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
	eventType string,
) *runtime.AgentStreamEventPayload {
	return &runtime.AgentStreamEventPayload{
		Type: "agent/event", Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		AgentID: binding.ExecutionID, ExecutionID: binding.ExecutionID, TaskID: binding.TaskID,
		SessionID: binding.SessionID, OwnerKind: runtime.ExecutionOwnerTask,
		Data: &runtime.AgentStreamEventData{Type: eventType, TurnID: operation.RequestSnapshot.TurnID},
	}
}
