# Claude Code Integration

Technical reference for Kandev's Claude Code CLI integration. Covers the stream-json protocol, message lifecycle, control plane, tool normalization, session resume, and permission hooks.

## Protocol Overview

Kandev communicates with Claude Code via the **stream-json** protocol: newline-delimited JSON over stdin/stdout. The CLI runs as a subprocess managed by agentctl.

```
┌─────────────┐  stdin (JSON lines)  ┌──────────────────┐
│   agentctl  │ ──────────────────►  │  Claude Code CLI  │
│  (adapter)  │  ◄──────────────────  │  (subprocess)     │
└─────────────┘  stdout (JSON lines) └──────────────────┘
```

**CLI invocation:**
```
npx -y @anthropic-ai/claude-code@<version>
  -p                              # Print mode (non-interactive)
  --output-format=stream-json     # Streaming JSON output
  --input-format=stream-json      # Streaming JSON input
  --permission-prompt-tool=stdio  # Permission prompts via stdio control messages
  --include-partial-messages      # Stream in-progress assistant messages
  --replay-user-messages          # Replay user messages on session resume
  --verbose                       # Extended logging
  --model <model>                 # Model selection
  --resume <session-id>           # Resume existing session
```

---

## Architecture

```
pkg/claudecode/
├── types.go         # Protocol message types, constants, structs
└── client.go        # stdin/stdout client, read loop, control request handling

internal/agentctl/server/adapter/transport/streamjson/
├── adapter.go                # Main adapter: state, dispatch, sendUpdate
├── streamjson_init.go        # Initialize, hooks, MCP config
├── streamjson_session.go     # NewSession / LoadSession
├── streamjson_prompt.go      # Prompt sending, image attachments, Cancel
├── streamjson_messages.go    # System, assistant, user, rate-limit handlers
├── streamjson_result.go      # Result handler, deduplication, completion
├── streamjson_permissions.go # Permission requests, hook callbacks, timeout
├── streamjson_tracing.go     # OTel tracing helpers for control messages
└── normalize.go              # Tool data → NormalizedPayload conversion
```

---

## Message Types

Every line on stdout is a JSON object with a `type` field. The client dispatches based on this type:

| Type | Direction | Description |
|------|-----------|-------------|
| `system` | CLI → Adapter | Session initialization (session_id, status, slash commands) |
| `assistant` | CLI → Adapter | Assistant response with content blocks (text, thinking, tool_use) |
| `user` | CLI → Adapter | Tool results (tool_result blocks) or slash command output |
| `result` | CLI → Adapter | Turn completion with stats (cost, tokens, duration, errors) |
| `rate_limit` | CLI → Adapter | API rate limit notification |
| `control_request` | CLI → Adapter | Permission request or hook callback |
| `control_cancel_request` | CLI → Adapter | Cancels a pending permission/hook request |
| `control_response` | Adapter → CLI | Response to a control_request |

---

## Message Lifecycle

### Client Layer (`pkg/claudecode/client.go`)

The `Client` reads stdout via `bufio.Scanner` (10 MB buffer), parses each line into a `CLIMessage`, and dispatches:

```
stdout line
  │
  ├── type=control_request      → RequestHandler(requestID, *ControlRequest)
  ├── type=control_cancel_request → CancelHandler(requestID)
  ├── type=control_response     → matches pending request by ID, unblocks goroutine
  └── (all other types)         → MessageHandler(*CLIMessage)
```

Control messages are handled immediately. All other messages are forwarded to the adapter's `handleMessage()`.

### Adapter Layer (`streamjson/adapter.go`)

The adapter's `handleMessage()` marshals the message for debug logging/tracing, then dispatches:

```
handleMessage(msg)
  │
  ├── system    → handleSystemMessage     → session_status, available_commands
  ├── assistant → handleAssistantMessage  → message_chunk, reasoning, tool_call, context_window
  ├── user      → handleUserMessage       → tool_update (complete/error), message_chunk (slash cmds)
  ├── result    → handleResultMessage     → complete, error (+ auto-complete pending tools)
  └── rate_limit → handleRateLimitMessage → rate_limit
```

Every dispatched event goes through `sendUpdate()`, which:
1. Logs the normalized event (JSONL debug file)
2. Creates an OTel span (child of the active prompt span)
3. Sends to the `updatesCh` channel for WebSocket delivery

---

## Turn Anatomy

A single turn (prompt → result) produces this message sequence:

```
Adapter                              Claude Code CLI
  │                                        │
  │  ── user message (prompt) ──────────►  │
  │                                        │
  │  ◄── system (session_id, status) ────  │  (first prompt only)
  │  ◄── assistant (text blocks) ────────  │  (streaming chunks)
  │  ◄── assistant (thinking blocks) ────  │  (reasoning, if enabled)
  │  ◄── assistant (tool_use block) ─────  │  (tool invocation)
  │                                        │
  │  ◄── control_request (can_use_tool) ─  │  (if hooks require approval)
  │  ── control_response (allow/deny) ──►  │
  │                                        │
  │  ◄── user (tool_result block) ───────  │  (tool output)
  │  ◄── assistant (text blocks) ────────  │  (more streaming)
  │  ◄── result ─────────────────────────  │  (turn complete)
  │                                        │
```

### Text Deduplication

The `result` message may contain a `text` field that duplicates already-streamed assistant text. The adapter tracks `streamingTextSentThisTurn`:
- Set to `true` when any `message_chunk` event is sent from assistant text blocks
- If `true` when result arrives, `result.text` is skipped
- Reset to `false` after each result

### Message UUIDs

Each message has a `uuid` field used for session truncation on resume:
- **Assistant UUIDs** are held as `pendingAssistantUUID` until the result message commits them to `lastMessageUUID`
- **User UUIDs** are committed immediately (safe to resume from)
- `lastMessageUUID` is included in the `complete` event and persisted for `--resume-session-at`

---

## Content Blocks

Assistant messages contain an array of content blocks:

### Text Block
```json
{ "type": "text", "text": "Here's the implementation..." }
```
Emitted as `message_chunk` events for real-time streaming.

### Thinking Block
```json
{ "type": "thinking", "thinking": "Let me analyze the code structure..." }
```
Emitted as `reasoning` events. Only present when extended thinking is enabled.

### Tool Use Block
```json
{
  "type": "tool_use",
  "id": "toolu_abc123",
  "name": "Edit",
  "input": { "file_path": "/src/main.go", "old_string": "...", "new_string": "..." }
}
```
Normalized into a `NormalizedPayload` and emitted as a `tool_call` event with status `"running"`.

### Tool Result Block (in user messages)
```json
{
  "type": "tool_result",
  "tool_use_id": "toolu_abc123",
  "content": "File edited successfully",
  "is_error": false
}
```
Enriches the pending `NormalizedPayload` with output data and emits a `tool_update` event with status `"complete"` or `"error"`.

---

## Control Plane

### Initialize

Sent once after the subprocess starts. Registers hooks and retrieves available slash commands.

```
Adapter → CLI:
{
  "type": "control_request",
  "request_id": "<uuid>",
  "request": {
    "subtype": "initialize",
    "hooks": {
      "PreToolUse": [{ "matcher": "^Bash$", "hookCallbackIds": ["tool_approval"] }],
      "Stop": [{ "hookCallbackIds": ["stop_git_check"] }]
    }
  }
}

CLI → Adapter:
{
  "type": "control_response",
  "response": {
    "subtype": "success",
    "request_id": "<uuid>",
    "response": {
      "commands": [
        { "name": "commit", "description": "Create a git commit" },
        { "name": "review-pr", "description": "Review a pull request" }
      ]
    }
  }
}
```

### Permission Request (can_use_tool)

When the CLI needs approval for a tool call:

```
CLI → Adapter:
{
  "type": "control_request",
  "request_id": "req_123",
  "request": {
    "subtype": "can_use_tool",
    "tool_name": "Bash",
    "tool_use_id": "toolu_abc",
    "input": { "command": "rm -rf /tmp/test" },
    "blocked_paths": "/etc,/usr"
  }
}

Adapter → CLI:
{
  "type": "control_response",
  "request_id": "req_123",
  "response": {
    "subtype": "success",
    "result": {
      "behavior": "allow",
      "updatedPermissions": [],
      "message": ""
    }
  }
}
```

The `behavior` field is `"allow"` or `"deny"`. On deny, an optional `interrupt: true` stops the current operation.

### Hook Callbacks

Hooks are registered during initialize and fire before tool execution. The response format differs from permission responses:

**PreToolUse hook response:**
```json
{
  "type": "control_response",
  "request_id": "req_456",
  "response": {
    "subtype": "success",
    "result": {
      "hookSpecificOutput": {
        "hookEventName": "PreToolUse",
        "permissionDecision": "allow",
        "permissionDecisionReason": "Auto-approved by SDK"
      }
    }
  }
}
```

`permissionDecision` values:
- `"allow"` — approve the tool call
- `"deny"` — reject the tool call
- `"ask"` — trigger a separate `can_use_tool` permission request

**Stop hook response:**
```json
{
  "type": "control_response",
  "request_id": "req_789",
  "response": {
    "subtype": "success",
    "result": {
      "decision": "approve",
      "reason": ""
    }
  }
}
```

### Cancel Request

The CLI can cancel a pending permission dialog (e.g., when the tool call is superseded):

```
CLI → Adapter:
{
  "type": "control_cancel_request",
  "request_id": "req_123"
}
```

The adapter emits a `permission_cancelled` event so the frontend closes the dialog.

### Interrupt

Sent by the adapter to cancel the current operation:

```
Adapter → CLI:
{
  "type": "control_request",
  "request_id": "<uuid>",
  "request": { "subtype": "interrupt" }
}
```

---

## Permission Modes

The adapter supports three permission policies, configured via hooks at initialize:

### Autonomous (default)
No hooks registered. `--dangerously-skip-permissions` flag bypasses all checks. Agent runs freely.

### Supervised
Uses `--permission-mode=bypassPermissions` with hooks:
- **PreToolUse hook**: Matches all tools except safe read-only ones (`Glob`, `Grep`, `NotebookRead`, `Read`, `Task`, `TodoWrite`). Responds with `permissionDecision: "ask"` which triggers a `can_use_tool` flow, forwarding to the frontend for user approval.
- **Stop hook**: Checks for uncommitted changes before agent exit.
- Non-matching tools (read-only) are implicitly auto-approved by Claude Code.

### Plan
Uses `--permission-mode=bypassPermissions` with hooks:
- **PreToolUse hook (ExitPlanMode)**: Requires approval to exit plan mode.
- **PreToolUse hook (everything else)**: Auto-approved.
- **Stop hook**: Same as supervised.

---

## Result Message

The result message signals turn completion and contains aggregate statistics:

```json
{
  "type": "result",
  "subtype": "success",
  "cost_usd": 0.0234,
  "duration_ms": 15234,
  "duration_api_ms": 12100,
  "is_error": false,
  "num_turns": 3,
  "total_input_tokens": 45000,
  "total_output_tokens": 2300,
  "model_usage": {
    "claude-sonnet-4-5": { "context_window": 200000 }
  },
  "result": {
    "text": "I've implemented the changes...",
    "session_id": "sess_abc123"
  }
}
```

On result, the adapter:
1. Auto-completes any pending tool calls that never received a `tool_result`
2. Updates context window size from `model_usage` if available
3. Emits result text only if no streaming text was sent this turn
4. Commits the pending assistant UUID as `lastMessageUUID`
5. Emits `complete` event with cost/token/duration stats
6. Signals completion to unblock the `Prompt()` goroutine
7. Emits `error` event if `is_error: true`

---

## Session Resume

### Resume by Session ID

```
--resume <session-id>
```

Resumes the conversation from the last message. The session ID is stored in the `executors_running` table as the resume token.

### Resume at Message UUID

```
--resume-session-at <message-uuid>
```

Truncates the conversation to the specified message UUID and resumes from there. Used for retry — when a turn fails, resume from the last known-good message instead of replaying the failed context.

The `lastMessageUUID` is:
- Tracked from message `uuid` fields during the conversation
- Committed on result messages (assistant UUIDs are pending until result confirms success)
- Included in the `complete` event's `data.last_message_uuid` field
- Persisted in the `executors_running.last_message_uuid` column

---

## Tool Normalization

The normalizer converts Claude Code tool calls into protocol-agnostic `NormalizedPayload` structs. This allows the frontend to render tools uniformly regardless of which agent produced them.

### Tool → Kind Mapping

| Claude Code Tool | NormalizedPayload Kind | Frontend Renderer |
|-----------------|----------------------|-------------------|
| `Edit`, `Write`, `NotebookEdit` | `modify_file` | `tool_edit` |
| `Read` | `read_file` | `tool_read` |
| `Glob`, `Grep` | `code_search` | `tool_search` |
| `Bash` | `shell_exec` | `tool_execute` |
| `WebFetch`, `WebSearch` | `http_request` | `tool_call` |
| `Task` | `subagent_task` | `tool_call` |
| `TaskCreate` | `create_task` | `tool_call` |
| `TaskUpdate`, `TaskList`, `TodoWrite` | `manage_todos` | `todo` |
| (unknown) | `generic` | `tool_call` |

### Normalization Flow

```
tool_use block (name, id, input)
  │
  ├── NormalizeToolCall(name, input) → NormalizedPayload
  │     stored in pendingToolCalls[id]
  │
  ├── emit tool_call event (status: "running", payload attached)
  │
  ... (tool executes) ...
  │
  ├── tool_result block (tool_use_id, content, is_error)
  │
  ├── NormalizeToolResult(payload, content) → enriches payload with output
  │
  └── emit tool_update event (status: "complete" or "error", payload attached)
```

### NormalizedPayload Structure

A discriminated union — exactly one kind-specific field is populated:

```
NormalizedPayload
  ├── kind: ToolKind (read_file, modify_file, shell_exec, ...)
  ├── read_file:     { file_path, offset, limit, output: { content, line_count, truncated } }
  ├── modify_file:   { file_path, mutations: [{ type, content, old_content, new_content, diff }] }
  ├── shell_exec:    { command, work_dir, description, timeout, background, output: { exit_code, stdout, stderr } }
  ├── code_search:   { query, pattern, path, glob, output: { files, file_count, truncated } }
  ├── http_request:  { url, method, response, is_error }
  ├── subagent_task: { description, prompt, subagent_type }
  ├── create_task:   { title, description }
  ├── manage_todos:  { operation, items: [{ id, description, status }] }
  ├── generic:       { name, input, output }
  └── misc:          { label, details }
```

---

## Agent Events

Events emitted through the `updatesCh` channel to the frontend via WebSocket:

| Event Type | When | Key Fields |
|-----------|------|------------|
| `session_status` | First system message | `session_id`, `data.session_status` ("new"/"resumed") |
| `available_commands` | After initialize or system message | `available_commands[]` (name, description) |
| `message_chunk` | Assistant text or slash command output | `text` |
| `reasoning` | Thinking/chain-of-thought | `reasoning_text` |
| `tool_call` | Tool invocation starts | `tool_call_id`, `tool_name`, `tool_title`, `tool_status: "running"`, `normalized` |
| `tool_update` | Tool result received | `tool_call_id`, `tool_status: "complete"/"error"`, `normalized` |
| `context_window` | Token usage update | `context_window_size`, `context_window_used`, `context_window_remaining`, `context_efficiency` |
| `complete` | Turn finished | `data` (cost_usd, duration_ms, num_turns, input/output_tokens, last_message_uuid) |
| `error` | Turn failed | `error` message |
| `rate_limit` | API rate limited | `rate_limit_message` |
| `permission_cancelled` | Permission dialog cancelled | `pending_id` |

### Subagent Nesting

When Claude Code uses the `Task` tool, it spawns subagents. Messages from subagents include `parent_tool_use_id`, which maps to the parent `tool_call_id`. The frontend uses this for visual nesting.

---

## Context Window Tracking

Context usage is tracked from two sources:

1. **Assistant messages** — `usage` field provides per-message token counts:
   ```
   context_used = input_tokens + output_tokens + cache_creation_input_tokens + cache_read_input_tokens
   ```

2. **Result messages** — `model_usage` map provides the actual context window size per model:
   ```json
   { "claude-sonnet-4-5": { "context_window": 200000 } }
   ```

The adapter tracks `mainModelName` (first model seen, excluding subagent models) and updates `mainModelContextWindow` from result stats. The default fallback is 200,000 tokens.

---

## MCP Server Configuration

MCP servers are passed to Claude Code via the `--mcp-config` flag:

```
--mcp-config '{"mcpServers":{"my-server":{"command":"node","args":["server.js"]}}}'
```

Supported transports:
- **stdio**: `{ "command": "...", "args": [...] }`
- **SSE/HTTP**: `{ "url": "...", "type": "sse" }`

Configuration is built from `cfg.McpServers` in `PrepareCommandArgs()`.

---

## Image Attachments

Since stream-json doesn't support multimodal content blocks in prompts, image attachments are handled by:

1. Saving base64-decoded images to `.kandev/temp/images/` in the workspace
2. Prepending file references to the prompt text
3. Claude Code reads the images via its `Read` tool

Supported formats: PNG, JPEG, GIF, WebP.

---

## Debug & Tracing

Enabled via `KANDEV_DEBUG_AGENT_MESSAGES=true`.

### Debug Logging
Writes JSONL files in the working directory:
- `raw-streamjson-<agentId>.jsonl` — raw protocol messages as received
- `normalized-streamjson-<agentId>.jsonl` — normalized AgentEvents

### OTel Tracing
Requires `OTEL_EXPORTER_OTLP_ENDPOINT` in addition to the debug flag.

**Span hierarchy:**
```
streamjson.prompt (parent span, covers entire turn)
  ├── streamjson.system           (session init)
  ├── streamjson.message_chunk    (streaming text)
  ├── streamjson.reasoning        (thinking)
  ├── streamjson.tool_call        (tool invocation)
  ├── streamjson.tool_update      (tool result)
  ├── streamjson.context_window   (token usage)
  ├── streamjson.control.control_request.can_use_tool   (incoming permission request)
  ├── streamjson.control.permission_response            (outgoing permission response)
  ├── streamjson.control.hook_response.pre_tool_use     (outgoing hook response)
  └── streamjson.complete         (turn completion)
```

Each span carries `raw` and `normalized` events for side-by-side comparison in Jaeger/Tempo.

---

## Key Source Files

| File | Package | Purpose |
|------|---------|---------|
| `pkg/claudecode/types.go` | `claudecode` | Protocol message types and constants |
| `pkg/claudecode/client.go` | `claudecode` | stdin/stdout client, read loop, control requests |
| `streamjson/adapter.go` | `streamjson` | Main adapter state, message dispatch, sendUpdate |
| `streamjson/streamjson_init.go` | `streamjson` | Initialize, hook building, MCP config |
| `streamjson/streamjson_session.go` | `streamjson` | Session create/resume |
| `streamjson/streamjson_prompt.go` | `streamjson` | Prompt send, wait, cancel, image attachments |
| `streamjson/streamjson_messages.go` | `streamjson` | System, assistant, user, rate-limit handlers |
| `streamjson/streamjson_result.go` | `streamjson` | Result processing, text dedup, completion signaling |
| `streamjson/streamjson_permissions.go` | `streamjson` | Permissions, hooks, timeout, cancel |
| `streamjson/normalize.go` | `streamjson` | Tool call/result → NormalizedPayload |
| `streams/tool_payload.go` | `streams` | NormalizedPayload discriminated union |
| `streams/agent.go` | `streams` | AgentEvent types and structure |
| `agents/claude_code.go` | `agents` | Agent definition, CLI flags, models |
