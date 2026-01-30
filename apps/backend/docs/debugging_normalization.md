# Debugging Tool Normalization Issues

This guide explains how to debug and fix issues where tool calls from agents are not rendering correctly in the frontend (e.g., a shell command showing as generic `tool_call` instead of `tool_execute`).

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Tool Call Normalization Flow                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Agent CLI          Adapter              Lifecycle           Orchestrator   │
│  ┌────────┐        ┌──────────┐         ┌─────────┐         ┌────────────┐  │
│  │ Codex  │──JSON──│ Codex    │──Event──│ Manager │──Event──│ Event      │  │
│  │ Claude │        │ Adapter  │         │         │         │ Handlers   │  │
│  │ etc.   │        └────┬─────┘         └────┬────┘         └─────┬──────┘  │
│  └────────┘             │                    │                    │         │
│                         │                    │                    │         │
│                         ▼                    ▼                    ▼         │
│                   NormalizedPayload    AgentEvent          Message DB       │
│                   ┌─────────────┐      ┌──────────┐        ┌──────────┐    │
│                   │ kind: ...   │ ───▶ │Normalized│ ──────▶│type: ... │    │
│                   │ shell_exec  │      │Payload   │        │tool_exec │    │
│                   └─────────────┘      └──────────┘        └────┬─────┘    │
│                                                                  │          │
│                                                                  ▼          │
│                                                           WebSocket Event   │
│                                                           ┌──────────────┐  │
│                                                           │type:tool_exec│  │
│                                                           └──────┬───────┘  │
│                                                                  │          │
│                                                                  ▼          │
│                                                             Frontend        │
│                                                           ┌──────────────┐  │
│                                                           │ToolExecute   │  │
│                                                           │ Message.tsx  │  │
│                                                           └──────────────┘  │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Key Files

| Layer | File | Purpose |
|-------|------|---------|
| Types | `internal/agentctl/types/streams/tool_payload.go` | `NormalizedPayload` struct and factory functions |
| Codex Adapter | `internal/agentctl/server/adapter/transport/codex/adapter.go` | Converts Codex events to `AgentEvent` |
| Codex Normalizer | `internal/agentctl/server/adapter/transport/codex/normalize.go` | Creates `NormalizedPayload` for Codex tools |
| StreamJSON Adapter | `internal/agentctl/server/adapter/transport/streamjson/adapter.go` | Converts Claude Code events to `AgentEvent` |
| StreamJSON Normalizer | `internal/agentctl/server/adapter/transport/streamjson/normalize.go` | Creates `NormalizedPayload` for Claude tools |
| OpenCode Normalizer | `internal/agentctl/server/adapter/transport/opencode/normalize.go` | Creates `NormalizedPayload` for OpenCode tools |
| Lifecycle Events | `internal/agent/lifecycle/events.go` | Publishes `AgentStreamEvent` to event bus |
| Orchestrator Handlers | `internal/orchestrator/event_handlers.go` | Handles events and creates DB messages |
| Message Creator | `cmd/kandev/adapters.go` | `toolKindToMessageType()` maps kind to message type |
| Frontend Renderer | `apps/web/components/task/chat/message-renderer.tsx` | Routes message types to components |

## Tool Kind to Message Type Mapping

The `NormalizedPayload.Kind()` determines which frontend component renders the tool:

| Tool Kind | Message Type | Frontend Component |
|-----------|-------------|-------------------|
| `read_file` | `tool_read` | `ToolReadMessage` |
| `code_search` | `tool_search` | `ToolSearchMessage` |
| `modify_file` | `tool_edit` | `ToolEditMessage` |
| `shell_exec` | `tool_execute` | `ToolExecuteMessage` |
| `manage_todos` | `todo` | `TodoMessage` |
| `subagent_task` | `tool_call` | `ToolSubagentMessage` |
| `generic` | `tool_call` | `ToolCallMessage` |
| (nil/unknown) | `tool_call` | `ToolCallMessage` |

## Debugging Steps

### 1. Check Raw Events

Raw events are logged to `raw-<protocol>-<agent>.jsonl` files in the backend directory:

```bash
# View raw Codex events
tail -f raw-codex-codex.jsonl | jq .

# View raw Claude events
tail -f raw-streamjson-claude.jsonl | jq .
```

### 2. Check Normalized Events

Normalized events show what the adapter produced:

```bash
# View normalized events
tail -f normalized-codex-codex.jsonl | jq .
```

Look for the `normalized` field in `tool_call` events:
```json
{
  "event": {
    "type": "tool_call",
    "normalized": {
      "kind": "shell_exec",
      "shell_exec": {
        "command": "ls -la",
        "work_dir": "/path/to/dir"
      }
    }
  }
}
```

If `normalized` is missing or `kind` is wrong, the issue is in the adapter.

### 3. Check Database Message Type

Query the database to see what message type was stored:

```sql
SELECT id, type, metadata->>'normalized' as normalized
FROM task_session_messages
WHERE metadata->>'tool_call_id' = '<tool_call_id>'
ORDER BY created_at DESC LIMIT 1;
```

If the type is `tool_call` but should be `tool_execute`, the issue is in the message creation flow.

### 4. Check WebSocket Events

Use browser DevTools Network tab to inspect WebSocket messages. Look for `session.message.added` events and check the `type` field.

## Common Issues

### Issue 1: Missing NormalizedPayload in tool_update Events

**Symptom:** Tool calls render as generic `tool_call` instead of specialized type.

**Cause:** The adapter sends `tool_update` events (e.g., on completion) without the `NormalizedPayload`. When there's a race condition and the message doesn't exist yet, the fallback creation uses `tool_call` as default.

**Example (Codex):**
```go
// BAD - tool_update without normalized payload
update := AgentEvent{
    Type:       streams.EventTypeToolUpdate,
    ToolCallID: p.Item.ID,
    ToolStatus: "complete",
}

// GOOD - tool_update with normalized payload
update := AgentEvent{
    Type:              streams.EventTypeToolUpdate,
    ToolCallID:        p.Item.ID,
    ToolStatus:        "complete",
    NormalizedPayload: a.normalizer.NormalizeToolCall("commandExecution", args),
}
```

**Fix Location:** Adapter's notification handler (e.g., `NotifyItemCompleted` in Codex adapter).

### Issue 2: Wrong Tool Name Passed to Normalizer

**Symptom:** Tool renders as `tool_call` (generic) instead of specialized type.

**Cause:** The normalizer's switch statement doesn't match the tool name.

**Example:**
```go
// Normalizer expects "commandExecution" but receives "command_execution"
switch toolName {
case CodexItemCommandExecution:  // "commandExecution"
    return n.normalizeCommand(args)
default:
    return n.normalizeGeneric(toolName, args)  // Falls through here
}
```

**Fix:** Check the exact tool name string in the protocol and match it in the normalizer.

### Issue 3: Factory Function Not Setting Kind

**Symptom:** `normalized.Kind()` returns empty or wrong value.

**Cause:** Factory function doesn't set the `kind` field correctly.

**Example:**
```go
// BAD - missing kind
func NewShellExec(...) *NormalizedPayload {
    return &NormalizedPayload{
        shellExec: &ShellExecPayload{...},
    }
}

// GOOD - kind is set
func NewShellExec(...) *NormalizedPayload {
    return &NormalizedPayload{
        kind:      ToolKindShellExec,
        shellExec: &ShellExecPayload{...},
    }
}
```

**Fix Location:** `internal/agentctl/types/streams/tool_payload.go`

### Issue 4: toolKindToMessageType Not Handling New Kind

**Symptom:** New tool kind renders as generic `tool_call`.

**Cause:** The `toolKindToMessageType` function doesn't have a case for the new kind.

**Fix Location:** Two places need updating:
1. `internal/orchestrator/event_handlers.go` - `toolKindToMessageType()`
2. `cmd/kandev/adapters.go` - `toolKindToMessageType()`

### Issue 5: Missing UnmarshalJSON for NormalizedPayload

**Symptom:** All tool calls render as `tool_call` even though adapter logs show correct `kind`.

**Cause:** The `NormalizedPayload` struct has unexported fields. Without a custom `UnmarshalJSON` method, the standard library cannot populate these fields when deserializing events from WebSocket/JSON.

**Example:**
```go
// NormalizedPayload with unexported fields requires both methods:
func (p *NormalizedPayload) MarshalJSON() ([]byte, error) { ... }
func (p *NormalizedPayload) UnmarshalJSON(data []byte) error { ... }  // REQUIRED!
```

**Fix Location:** `internal/agentctl/types/streams/tool_payload.go`

**How to verify:** Run the round-trip test:
```bash
go test ./internal/agentctl/types/streams/... -v -run MarshalRoundTrip
```

## Adding a New Tool Kind

1. **Add kind constant** in `internal/agentctl/types/streams/tool_payload.go`:
   ```go
   const (
       ToolKindMyNewTool ToolKind = "my_new_tool"
   )
   ```

2. **Add payload struct** with fields:
   ```go
   type MyNewToolPayload struct {
       SomeField string `json:"some_field"`
   }
   ```

3. **Add unexported field** to `NormalizedPayload`:
   ```go
   type NormalizedPayload struct {
       kind       ToolKind
       myNewTool  *MyNewToolPayload
       // ...
   }
   ```

4. **Add getter method**:
   ```go
   func (p *NormalizedPayload) MyNewTool() *MyNewToolPayload { return p.myNewTool }
   ```

5. **Add to MarshalJSON**:
   ```go
   type jsonPayload struct {
       MyNewTool *MyNewToolPayload `json:"my_new_tool,omitempty"`
       // ...
   }
   ```

6. **Add factory function**:
   ```go
   func NewMyNewTool(someField string) *NormalizedPayload {
       return &NormalizedPayload{
           kind: ToolKindMyNewTool,
           myNewTool: &MyNewToolPayload{
               SomeField: someField,
           },
       }
   }
   ```

7. **Update normalizers** in each adapter to call the factory function.

8. **Update toolKindToMessageType** in both locations:
   ```go
   case streams.ToolKindMyNewTool:
       return "tool_my_new"
   ```

9. **Add frontend component** and update `message-renderer.tsx`:
   ```tsx
   {
       matches: (comment) => comment.type === 'tool_my_new',
       render: (comment) => <ToolMyNewMessage comment={comment} />,
   },
   ```

## Testing

### Unit Tests

Run normalizer tests:
```bash
go test ./internal/agentctl/server/adapter/transport/streamjson/... -v
go test ./internal/agentctl/server/adapter/transport/codex/... -v
```

### Integration Testing

1. Start a session with the agent
2. Trigger the tool you're testing
3. Check normalized events file
4. Check database message type
5. Verify frontend renders correct component

## Checklist for Fixing Normalization Issues

- [ ] Check raw events to confirm agent sends expected data
- [ ] Check normalized events to confirm adapter produces correct payload
- [ ] Verify `NormalizedPayload.Kind()` returns correct kind
- [ ] Verify `tool_call` event includes `NormalizedPayload`
- [ ] Verify `tool_update` event includes `NormalizedPayload` (for race condition handling)
- [ ] Check `toolKindToMessageType` handles the kind in both files
- [ ] Verify database stores correct message type
- [ ] Verify frontend `message-renderer.tsx` has matching adapter
