"use client";

import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import type { Repository } from "@/lib/types/http";

/**
 * Returns a map from `repository_name` to its task base_branch for the active
 * task. Multi-repo tasks store one base_branch per repo (e.g. front: `main`,
 * back: `release/24.x`); UI surfaces that show the merge target need this
 * resolution to avoid pretending one workspace-level branch covers every repo.
 *
 * Empty for single-repo tasks (callers fall back to the workspace-level
 * baseBranchDisplay) and for tasks not yet hydrated.
 */
export function useBaseBranchByRepo(activeTaskId: string | null): Record<string, string> {
  const tasks = useAppStore((s) => s.kanban.tasks);
  const reposByWorkspace = useAppStore((s) => s.repositories.itemsByWorkspaceId);
  return useMemo(() => {
    if (!activeTaskId) return {};
    const task = tasks.find((t) => t.id === activeTaskId);
    if (!task?.repositories?.length) return {};
    const allRepos = Object.values(reposByWorkspace).flat() as Repository[];
    const repoNameById = new Map(allRepos.map((r) => [r.id, r.name]));
    const out: Record<string, string> = {};
    for (const link of task.repositories) {
      const name = repoNameById.get(link.repository_id);
      if (name && link.base_branch) out[name] = link.base_branch;
    }
    return out;
  }, [activeTaskId, tasks, reposByWorkspace]);
}
