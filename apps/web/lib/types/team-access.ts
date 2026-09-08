/**
 * Team access: workspace unit placement, membership, and the caller's
 * resolved scopes.
 *
 * Scope strings are compared with `===` against the values the backend
 * registry emits, so they must never be translated or reformatted.
 */

/** The caller's role in one workspace. Empty means unreachable. */
export type WorkspaceRole = "owner" | "collaborator" | "viewer" | "";

/** Roles that can be assigned through the member API. Owner comes from transfer. */
export const ASSIGNABLE_WORKSPACE_ROLES = ["collaborator", "viewer"] as const;
export type AssignableWorkspaceRole = (typeof ASSIGNABLE_WORKSPACE_ROLES)[number];

/**
 * Scope identifiers mirrored from `internal/authz`.
 * i18n-exempt: wire identifiers compared with ===, never displayed raw.
 */
export const SCOPE = {
  workspaceRead: "workspace.read",
  workspaceManage: "workspace.manage",
  taskWrite: "task.write",
  sessionPrompt: "session.prompt",
  sessionControl: "session.control",
  sessionExec: "session.exec",
  repositoryManage: "repository.manage",
  secretManage: "secret.manage",
  memberManage: "member.manage",
} as const;

export type Scope = (typeof SCOPE)[keyof typeof SCOPE];

export type WorkspaceMember = {
  user_id: string;
  display_name?: string;
  role: WorkspaceRole;
  added_by?: string;
};

export type ListWorkspaceMembersResponse = {
  members: WorkspaceMember[];
  total: number;
};

export type DirectoryUser = {
  id: string;
  display_name: string;
};

export type ListDirectoryUsersResponse = {
  users: DirectoryUser[];
  total: number;
};

/**
 * Whether the caller holds a scope on a workspace.
 *
 * The server is authoritative; this only decides whether to render a control.
 * Absent scopes (an older backend, or a payload that predates this field) read
 * as "not granted", so a stale client hides controls rather than showing ones
 * that would 403.
 */
export function hasScope(scopes: readonly string[] | undefined, scope: Scope): boolean {
  return Boolean(scopes?.includes(scope));
}
