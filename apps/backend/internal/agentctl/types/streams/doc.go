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
// # Unified Workspace Stream (/api/v1/workspace/stream)
//
// Bidirectional WebSocket that consolidates all workspace-related streams:
//   - Shell I/O (input, output, exit, resize)
//   - Git status updates
//   - File change notifications
//   - File list updates
//   - Ping/pong keepalive
//
// Message type: WorkspaceStreamMessage (defined in types/types.go)
//
// Message types (use WorkspaceMsg* constants):
//   - shell_output: Shell output data
//   - shell_input: Shell input data (client -> server)
//   - shell_exit: Shell process exited
//   - shell_resize: Terminal resize (client -> server)
//   - git_status: Git status update
//   - file_change: File change notification
//   - file_list: File list update
//   - ping/pong: Keepalive messages
//   - connected: Connection established
//   - error: Error occurred
//
// All streams use JSON-encoded messages over WebSocket connections.
package streams
