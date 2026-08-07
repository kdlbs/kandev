import type { TaskSession } from "@/lib/types/http";
import { getSessionWorkspacePath } from "@/lib/session-workspace-path";

const HOME_PATH_PATTERN = /^\/(?:Users|home)\/[^/]+\//;

export function getFileBrowserSessionWorkspacePath(
  session: Pick<TaskSession, "workspace_path" | "worktree_path"> | null | undefined,
): string | undefined {
  return getSessionWorkspacePath(session);
}

export type FileBrowserPathInput = {
  sessionWorktreePath?: string | null;
  repositoryLocalPath?: string | null;
  treePath?: string | null;
  treeLoaded: boolean;
};

export type FileBrowserPaths = {
  fullPath: string;
  displayPath: string;
};

export function resolveFileBrowserPaths({
  sessionWorktreePath,
  repositoryLocalPath,
  treePath,
  treeLoaded,
}: FileBrowserPathInput): FileBrowserPaths {
  const fullPath = sessionWorktreePath || repositoryLocalPath || treePath || "";
  if (fullPath) {
    return {
      fullPath,
      displayPath: fullPath.replace(HOME_PATH_PATTERN, "~/"),
    };
  }

  return {
    fullPath,
    displayPath: treeLoaded ? "Workspace root" : "",
  };
}
