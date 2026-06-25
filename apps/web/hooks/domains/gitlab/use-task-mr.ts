"use client";

import { useEffect } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { qk } from "@/lib/query/keys";
import {
  taskMrsQueryOptions,
  workspaceTaskMrsQueryOptions,
} from "@/lib/query/query-options/gitlab";
import type { TaskMR } from "@/lib/types/gitlab";
import { useGitLabStatus } from "./use-gitlab-status";

/**
 * Fetch all MR associations for a workspace and seed per-task MR query caches.
 */
export function useWorkspaceMRs(workspaceId: string | null) {
  const queryClient = useQueryClient();
  const query = useQuery({
    ...workspaceTaskMrsQueryOptions(workspaceId ?? ""),
    enabled: Boolean(workspaceId),
  });

  useEffect(() => {
    if (!workspaceId || !query.data) return;
    const mrsByTask = query.data.task_mrs ?? {};
    for (const [taskId, mrs] of Object.entries(mrsByTask)) {
      queryClient.setQueryData(qk.integrations.gitlab.taskMr(taskId), mrs);
    }
  }, [query.data, queryClient, workspaceId]);

  return query.data?.task_mrs ?? {};
}

const EMPTY_MRS: TaskMR[] = [];

/** Return MRs linked to a task. */
export function useTaskMRs(taskId: string | null): TaskMR[] {
  const query = useQuery(taskMrsQueryOptions(taskId ?? ""));
  return Array.isArray(query.data) ? query.data : EMPTY_MRS;
}

/**
 * Returns whether GitLab is configured enough to surface in the integrations
 * menu. Token-configured or authenticated counts as "available".
 */
export function useGitLabAvailable(): boolean {
  const { status } = useGitLabStatus();
  return Boolean(status?.authenticated || status?.token_configured);
}
