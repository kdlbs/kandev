/**
 * Permission request types mirrored from
 * `apps/backend/internal/agentctl/types/streams/permission.go`.
 *
 * Keep these unions in sync with the backend constants
 * (`PermissionActionType`, `PermissionOptionKind`). Stable wire-format strings
 * shared across agentctl, the backend orchestrator, and the frontend.
 */

/**
 * Categorises the kind of action requiring approval.
 *
 * Mirrors `streams.PermissionActionType` constants:
 *   - command:    shell command execution
 *   - file_write: file modification or creation
 *   - file_read:  file read (for sensitive files)
 *   - network:    network access
 *   - mcp_tool:   MCP tool invocation
 *   - other:      other / unknown action type
 */
export type PermissionActionType =
  | "command"
  | "file_write"
  | "file_read"
  | "network"
  | "mcp_tool"
  | "other";

/**
 * Identifies the semantics of a permission option presented to the user.
 *
 * Mirrors `streams.PermissionOptionKind` constants:
 *   - allow_once:    approves the current request only
 *   - allow_always:  approves and remembers the decision
 *   - reject_once:   rejects the current request only
 *   - reject_always: rejects and remembers the decision
 */
export type PermissionOptionKind = "allow_once" | "allow_always" | "reject_once" | "reject_always";
