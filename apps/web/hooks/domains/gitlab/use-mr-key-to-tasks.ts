"use client";

import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import type { TaskMR } from "@/lib/types/gitlab";
import { gitLabMRKey } from "@/lib/gitlab-identity";
import { useWorkspaceMRs } from "./use-task-mr";

export function mrKey(host: string, projectPath: string, iid: number): string {
  return gitLabMRKey(host, projectPath, iid);
}

export function useMRKeyToTasks(workspaceId: string | null): Map<string, TaskMR[]> {
  useWorkspaceMRs(workspaceId);
  const byTaskId = useAppStore((state) =>
    workspaceId ? (state.taskMRs.byWorkspaceId[workspaceId] ?? EMPTY_TASK_MRS) : EMPTY_TASK_MRS,
  );

  return useMemo(() => {
    const result = new Map<string, TaskMR[]>();
    for (const associations of Object.values(byTaskId)) {
      for (const association of associations ?? []) {
        const key = mrKey(association.host, association.project_path, association.mr_iid);
        result.set(key, [...(result.get(key) ?? []), association]);
      }
    }
    return result;
  }, [byTaskId]);
}

const EMPTY_TASK_MRS: Record<string, TaskMR[]> = {};
