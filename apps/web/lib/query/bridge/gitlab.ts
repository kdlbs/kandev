import type { QueryClient } from "@tanstack/react-query";
import type { WebSocketClient } from "@/lib/ws/client";
import type { TaskMR, TaskMRsResponse } from "@/lib/types/gitlab";

/**
 * Upsert a single MR into the by-task-id map.
 *
 * Keyed by (repository_id, project_path, mr_iid) — mirrors the upsert logic
 * in the old Zustand slice's setTaskMR action and the backend's UNIQUE
 * constraint on gitlab_task_mrs.
 */
function upsertMR(prev: TaskMRsResponse | undefined, taskId: string, mr: TaskMR): TaskMRsResponse {
  const existing: TaskMR[] = prev?.task_mrs[taskId] ?? [];
  const repoKey = mr.repository_id ?? "";
  const idx = existing.findIndex(
    (m) =>
      (m.repository_id ?? "") === repoKey &&
      m.project_path === mr.project_path &&
      m.mr_iid === mr.mr_iid,
  );
  const updated = [...existing];
  if (idx >= 0) {
    updated[idx] = mr;
  } else {
    updated.push(mr);
  }
  return {
    task_mrs: {
      ...(prev?.task_mrs ?? {}),
      [taskId]: updated,
    },
  };
}

/**
 * Registers WS handlers for GitLab domain events into the TanStack Query cache.
 *
 * There is no existing lib/ws/handlers/gitlab.ts — the WS side of GitLab was
 * handled server-push only via `gitlab.task_mr.updated`. This bridge replaces
 * the Zustand store writes with cache updates using immutable functional
 * updaters.
 *
 * Events handled:
 *   gitlab.task_mr.updated — upsert a TaskMR into the workspace MRs cache
 *
 * Returns a cleanup function that removes all registered handlers.
 */
export function registerGitlabBridge(ws: WebSocketClient, queryClient: QueryClient): () => void {
  const unsubMRUpdated = ws.on("gitlab.task_mr.updated", (message) => {
    const mr = message.payload;
    const taskId = mr.task_id;
    if (!taskId) return;

    // We need the workspace ID to know which cache key to update. The
    // TaskMR payload does not include a workspace_id, so we must update all
    // matching workspace MR caches that contain this task. In practice a task
    // belongs to one workspace, but we scan cached keys defensively.
    const cachedKeys = queryClient
      .getQueryCache()
      .findAll({ queryKey: ["gitlab"] })
      .filter((q) => q.queryKey[2] === "mrs");

    for (const query of cachedKeys) {
      queryClient.setQueryData<TaskMRsResponse>(
        query.queryKey,
        (prev): TaskMRsResponse => upsertMR(prev, taskId, mr),
      );
    }
  });

  return () => {
    unsubMRUpdated();
  };
}
