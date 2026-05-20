"use client";

import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { useWorkspacePRs } from "./use-task-pr";
import type { TaskPR } from "@/lib/types/github";

function prKey(owner: string, repo: string, prNumber: number): string {
  return `${owner}/${repo}#${prNumber}`;
}

export function usePRKeyToTasks(workspaceId: string | null): Map<string, TaskPR[]> {
  useWorkspacePRs(workspaceId);
  const byTaskId = useAppStore((state) => state.taskPRs.byTaskId);

  return useMemo(() => {
    const map = new Map<string, TaskPR[]>();
    for (const taskId of Object.keys(byTaskId)) {
      const prs = byTaskId[taskId];
      if (!Array.isArray(prs)) continue;
      for (const pr of prs) {
        const key = prKey(pr.owner, pr.repo, pr.pr_number);
        const existing = map.get(key) ?? [];
        existing.push(pr);
        map.set(key, existing);
      }
    }
    return map;
  }, [byTaskId]);
}

export { prKey };
