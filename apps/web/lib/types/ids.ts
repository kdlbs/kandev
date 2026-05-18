/**
 * Branded ID types
 *
 * Branded primitives let TypeScript catch argument-order mixups at compile
 * time (e.g. passing a `sessionId` where a `taskId` is expected). They erase
 * to `string` at runtime — the brand is a phantom property that only exists
 * in the type system.
 *
 * Usage:
 *
 *   function loadTask(id: TaskId) { ... }
 *
 *   // at a trust boundary (HTTP parse, URL params, WS payload, SSR hydration):
 *   loadTask(taskId(json.id));
 *
 *   // values read from already-branded fields propagate the brand:
 *   loadTask(task.id); // task.id is already TaskId
 *
 * Comparison with string literals still works because branded types extend
 * string. Cross-brand comparison is a compile error — which is the goal.
 */

export type TaskId = string & { readonly __brand: "TaskId" };
export type SessionId = string & { readonly __brand: "SessionId" };
export type WorkspaceId = string & { readonly __brand: "WorkspaceId" };
export type WorkflowId = string & { readonly __brand: "WorkflowId" };
export type RepositoryId = string & { readonly __brand: "RepositoryId" };
export type AgentProfileId = string & { readonly __brand: "AgentProfileId" };

// Escape-hatch constructors. Use these only at trust boundaries:
//   - HTTP/WS payload parsing
//   - URL/route param decoding
//   - SSR hydration adapters
//   - test fixtures
// Inside the app, prefer reading from already-branded fields (task.id, etc).
export const taskId = (s: string): TaskId => s as TaskId;
export const sessionId = (s: string): SessionId => s as SessionId;
export const workspaceId = (s: string): WorkspaceId => s as WorkspaceId;
export const workflowId = (s: string): WorkflowId => s as WorkflowId;
export const repositoryId = (s: string): RepositoryId => s as RepositoryId;
export const agentProfileId = (s: string): AgentProfileId => s as AgentProfileId;
