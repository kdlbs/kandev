"use client";

import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";
import { resolveChangeRequestProviderTarget } from "@/lib/plugins/change-request-creation";
import { usePluginRegistry } from "@/lib/plugins/registry";
import type { Repository } from "@/lib/types/http";

const EMPTY_REPOSITORIES: readonly Repository[] = [];

export function useChangeRequestProviderTarget(sessionId: string | null, repositoryScope?: string) {
  const registry = usePluginRegistry();
  const registryVersion = registry.getVersion();
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const taskId = useAppStore((state) =>
    sessionId ? state.taskSessions.items[sessionId]?.task_id : state.tasks.activeTaskId,
  );
  const task = useAppStore((state) =>
    taskId
      ? (state.kanban.tasks.find((candidate) => candidate.id === taskId) ??
        findTaskInSnapshots(taskId, state.kanbanMulti.snapshots) ??
        undefined)
      : undefined,
  );
  const workspaceId = task?.workspaceId ?? activeWorkspaceId ?? undefined;
  const repositories = useAppStore((state) =>
    workspaceId
      ? (state.repositories.itemsByWorkspaceId[workspaceId] ?? EMPTY_REPOSITORIES)
      : EMPTY_REPOSITORIES,
  );
  const taskWithWorkspace = useMemo(
    () => (task && workspaceId ? { ...task, workspaceId } : task),
    [task, workspaceId],
  );
  return useMemo(
    () =>
      resolveChangeRequestProviderTarget({
        task: taskWithWorkspace,
        repositories,
        repositoryScope,
        getProvider: (providerId) => registry.getRepositoryProvider(providerId),
      }),
    [registry, registryVersion, repositories, repositoryScope, taskWithWorkspace],
  );
}
