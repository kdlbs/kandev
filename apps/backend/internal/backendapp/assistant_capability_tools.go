package backendapp

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	settings "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	shared "github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func (a *assistantCapabilityReader) pluginCapabilities(_ context.Context, q shared.CapabilityQuery) ([]shared.Capability, error) {
	rows := []shared.Capability{}
	if a.plugins == nil {
		return rows, nil
	}
	snapshot, err := a.plugins.AgentToolCatalog()
	if err != nil {
		return nil, err
	}
	for _, tool := range snapshot.Tools {
		if !slices.Contains(tool.Surfaces, capabilityConversation) {
			continue
		}
		entry := catalogCapability("plugin", tool.ExposedName, tool.ExposedName, q.WorkspaceID, fmt.Sprintf("%s:%d", snapshot.Generation, snapshot.Revision))
		entry.Surfaces = slices.Clone(tool.Surfaces)
		entry.InputSchema, entry.SchemaPartial = capabilitySchema(tool.InputSchema)
		entry.Reason = "effect_unverified"
		rows = append(rows, entry)
	}
	return rows, nil
}

func (a *assistantCapabilityReader) sessionCapabilities(ctx context.Context, q shared.CapabilityQuery) ([]shared.Capability, error) {
	rows := []shared.Capability{}
	if q.SessionID == "" {
		return rows, nil
	}
	session, err := a.tasks.GetTaskSession(ctx, q.SessionID)
	if err != nil || session.TaskID != q.ConversationID {
		return nil, fmt.Errorf("session scope unavailable")
	}
	history, valid := agentruntime.LoadMCPAttachmentHistory(session.Metadata[taskmodels.SessionMetaKeyMCPAttachmentState])
	if !valid {
		return rows, nil
	}
	current := history.Current
	live := a.currentAttachment(ctx, session, current) && a.attachmentProfileCurrent(ctx, q.WorkspaceID, current.AgentProfileID)
	for _, server := range current.Servers {
		entry := mcpServerCapability(q, current, server, live)
		rows = append(rows, entry)
		for _, tool := range server.Tools {
			toolEntry := entry
			toolEntry.ID = fmt.Sprintf("mcp/%x", sha256.Sum256([]byte(q.SessionID+"/"+server.Name+"/"+tool.Name)))
			toolEntry.Name = safeCapabilityName(tool.Name)
			toolEntry.InputSchema, toolEntry.SchemaPartial = capabilitySchema(tool.InputSchema)
			rows = append(rows, toolEntry)
		}
	}
	return rows, nil
}

func mcpServerCapability(q shared.CapabilityQuery, attempt streams.MCPAttachmentAttempt, server streams.MCPServerAttachment, live bool) shared.Capability {
	id := fmt.Sprintf("%x", sha256.Sum256([]byte(q.SessionID+"/"+server.Name)))
	entry := catalogCapability("mcp", id, server.Name, q.WorkspaceID, attempt.AttemptID+":"+attempt.UpdatedAt.Format(time.RFC3339Nano))
	entry.SessionID, entry.ProfileID = q.SessionID, attempt.AgentProfileID
	entry.ResourceID = server.Name
	entry.Configured = server.ConfiguredAt != nil
	entry.Attached = live && server.DisconnectedAt == nil && (server.Status == streams.MCPAttachmentStatusActive || server.Status == streams.MCPAttachmentStatusConnected)
	switch {
	case !live:
		entry.Health, entry.Reason = capabilityUnavailable, "stale_session_attachment"
	case server.DisconnectedAt != nil:
		entry.Health, entry.Reason = capabilityDisconnected, "connection_closed"
	case entry.Attached:
		entry.Health, entry.Reason = capabilityReady, "session_observed"
	case server.Status == streams.MCPAttachmentStatusFailed:
		entry.Health, entry.Reason = capabilityUnavailable, "attachment_failed"
	case server.Status == streams.MCPAttachmentStatusUnavailable || server.Status == streams.MCPAttachmentStatusFiltered:
		entry.Health, entry.Reason = capabilityUnavailable, "attachment_unavailable"
	}
	return entry
}

// Only structural schema keywords leave the catalog. Descriptions, defaults,
// examples, extensions and references can contain provider configuration.
func capabilitySchema(raw json.RawMessage) (json.RawMessage, bool) {
	var source map[string]any
	if len(raw) > 65536 || json.Unmarshal(raw, &source) != nil {
		return json.RawMessage(`{}`), true
	}
	projected := structuralSchema(source, 0)
	data, _ := json.Marshal(projected)
	if len(data) > 8192 {
		return json.RawMessage(`{}`), true
	}
	original, _ := json.Marshal(source)
	return data, string(data) != string(original)
}

func structuralSchema(source map[string]any, depth int) map[string]any {
	result := map[string]any{}
	if depth > 6 {
		return result
	}
	if kind, ok := source[capabilitySchemaType].(string); ok && slices.Contains([]string{"object", "array", "string", "number", "integer", "boolean", "null"}, kind) {
		result[capabilitySchemaType] = kind
	}
	if additional, ok := source["additionalProperties"].(bool); ok {
		result["additionalProperties"] = additional
	}
	if properties, ok := source["properties"].(map[string]any); ok && len(properties) <= 64 {
		out := map[string]any{}
		for key, value := range properties {
			if child, ok := value.(map[string]any); ok && len(key) <= 128 {
				out[key] = structuralSchema(child, depth+1)
			}
		}
		result["properties"] = out
		result["required"] = schemaRequired(source["required"], out)
	}
	if child, ok := source["items"].(map[string]any); ok {
		result["items"] = structuralSchema(child, depth+1)
	}
	return result
}

func schemaRequired(raw any, properties map[string]any) []string {
	result := []string{}
	values, _ := raw.([]any)
	for _, value := range values {
		if name, ok := value.(string); ok && properties[name] != nil {
			result = append(result, name)
		}
	}
	return result
}

func (a *assistantCapabilityReader) attachmentProfileCurrent(ctx context.Context, workspace, id string) bool {
	if a.profiles == nil {
		return false
	}
	p, err := a.profiles.GetAgentProfile(ctx, id)
	return err == nil && p != nil && p.Enabled && p.DeletedAt == nil && p.Role == "" && (p.WorkspaceID == "" || p.WorkspaceID == workspace)
}

func (a *assistantCapabilityReader) configuredMCPCapabilities(ctx context.Context, q shared.CapabilityQuery, profile *settings.AgentProfile) ([]shared.Capability, error) {
	config, err := a.profiles.GetAgentProfileMcpConfig(ctx, profile.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return []shared.Capability{}, nil
	}
	if err != nil {
		return nil, err
	}
	rows := []shared.Capability{}
	if config == nil {
		return rows, nil
	}
	for name := range config.Servers {
		id := fmt.Sprintf("%x", sha256.Sum256([]byte("configured/"+profile.ID+"/"+name)))
		entry := catalogCapability("mcp", id, name, q.WorkspaceID, config.UpdatedAt.Format(time.RFC3339Nano))
		entry.ProfileID = profile.ID
		entry.ResourceID = name
		if !config.Enabled || !profile.Enabled {
			entry.Health, entry.Reason = capabilityUnavailable, "profile_mcp_disabled"
		}
		rows = append(rows, entry)
	}
	return rows, nil
}

func (a *assistantCapabilityReader) currentAttachment(ctx context.Context, session *taskmodels.TaskSession, current streams.MCPAttachmentAttempt) bool {
	if current.SessionID != session.ID || current.TaskID != session.TaskID || current.ExecutionID == "" || (session.State != taskmodels.TaskSessionStateRunning && session.State != taskmodels.TaskSessionStateWaitingForInput) {
		return false
	}
	running, err := a.tasks.GetExecutorRunningBySessionID(ctx, session.ID)
	return err == nil && running != nil && running.AgentExecutionID == current.ExecutionID && running.ExecutionProfileID == current.AgentProfileID && (running.Status == taskmodels.ExecutorRunningStatusRunning || running.Status == taskmodels.ExecutorRunningStatusReady)
}
