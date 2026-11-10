"use client";

import { githubResourceKey } from "@/lib/github/host";
import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { useWorkspacePRs } from "./use-task-pr";
import type { TaskPR } from "@/lib/types/github";

function prKey(owner: string, repo: string, prNumber: number, host = "github.com"): string {
  return githubResourceKey(owner, repo, prNumber, host);
}

export function usePRKeyToTasks(workspaceId: string | null): Map<string, TaskPR[]> {
  useWorkspacePRs(workspaceId);
  const taskPRs = useAppStore((state) => state.taskPRs);
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const workspaceContextGeneration = useAppStore((state) => state.workspaceContextGeneration);

  return useMemo(() => {
    const map = new Map<string, TaskPR[]>();
    if (
      workspaceId === null ||
      workspaceId !== activeWorkspaceId ||
      taskPRs.workspaceId !== workspaceId ||
      taskPRs.workspaceContextGeneration !== workspaceContextGeneration
    ) {
      return map;
    }
    const { byTaskId } = taskPRs;
    for (const taskId of Object.keys(byTaskId)) {
      const prs = byTaskId[taskId];
      if (!Array.isArray(prs)) continue;
      for (const pr of prs) {
        const key = prKey(pr.owner, pr.repo, pr.pr_number, pr.github_host);
        const existing = map.get(key) ?? [];
        existing.push(pr);
        map.set(key, existing);
      }
    }
    return map;
  }, [taskPRs, activeWorkspaceId, workspaceContextGeneration, workspaceId]);
}

export { prKey };
