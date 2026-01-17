// Package streams defines the protocol message types for agentctl WebSocket streams.
//
// The agentctl client produces messages to the following channels/streams:
//
// # Agent Events Stream (/api/v1/agent/events)
//
// Streams real-time events from the agent process including message chunks,
// reasoning/thinking content, tool invocations, plan updates, and completion
// or error notifications. This stream is protocol-agnostic and works with
// any agent backend (ACP, Codex, Claude Code, etc.).
//
// Message type: AgentEvent
//
// Event types (use EventType* constants):
//   - message_chunk: Streaming text content from the agent
//   - reasoning: Chain-of-thought or thinking content
//   - tool_call: A tool invocation has started
//   - tool_update: Tool status update (running, completed, error)
//   - plan: Agent plan/task list updates
//   - complete: The turn or operation has completed
//   - error: An error occurred
//
// # Permission Stream (/api/v1/acp/permissions/stream)
//
// Streams permission requests from the agent when it needs approval for
// actions like file writes, shell commands, or network access.
//
// Message type: PermissionNotification
//
// # Git Status Stream (/api/v1/workspace/git-status/stream)
//
// Streams git status updates when the workspace state changes.
//
// Message type: GitStatusUpdate
//
// # File Changes Stream (/api/v1/workspace/file-changes/stream)
//
// Streams file system change notifications when files are created,
// modified, deleted, or renamed in the workspace.
//
// Message type: FileChangeNotification
//
// # Shell Stream (/api/v1/shell/stream)
//
// Bidirectional WebSocket for interactive shell I/O.
//
// Message type: ShellMessage (input/output/ping/pong/exit)
//
// All streams use JSON-encoded messages over WebSocket connections.
package streams

