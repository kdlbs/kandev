import type { TaskSession } from "@/lib/types/http";

type SessionWorkspacePathInput = Pick<TaskSession, "workspace_path" | "worktree_path">;

/** Returns the task-root path used by Files/chat, with legacy session fallback. */
export function getSessionWorkspacePath(
  session: SessionWorkspacePathInput | null | undefined,
): string | undefined {
  return session?.workspace_path || session?.worktree_path || undefined;
}
