import type { TaskSession } from "@/lib/types/http";

type SessionWorkspacePathInput = {
  workspace_path?: TaskSession["workspace_path"] | null;
  worktree_path?: TaskSession["worktree_path"] | null;
};

/** Returns the task-root path used by Files/chat, with legacy session fallback. */
export function getSessionWorkspacePath(
  session: SessionWorkspacePathInput | null | undefined,
): string | undefined {
  return session?.workspace_path || session?.worktree_path || undefined;
}
