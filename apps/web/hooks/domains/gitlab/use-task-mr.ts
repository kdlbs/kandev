"use client";

import { useQuery } from "@tanstack/react-query";
import { fetchGitLabStatus, listWorkspaceTaskMRs } from "@/lib/api/domains/gitlab-api";
import { useAppStore } from "@/components/state-provider";
import { qk } from "@/lib/query/keys";
import type { TaskMR } from "@/lib/types/gitlab";

// Stable empty array so referential equality is preserved across renders when
// a task has no MRs, preventing unnecessary re-renders.
const EMPTY_MRS: TaskMR[] = [];

/**
 * Hydrate the workspace's task-MR associations into the TanStack Query cache.
 *
 * Replaces the manual inFlight + fetchedRef pattern with useQuery dedup.
 * TanStack Query handles deduplication — mounting this hook multiple times for
 * the same workspaceId will issue only one request.
 */
export function useWorkspaceMRs(workspaceId: string | null) {
  useQuery({
    queryKey: qk.gitlab.mrs(workspaceId ?? ""),
    queryFn: () => listWorkspaceTaskMRs(workspaceId!, { cache: "no-store" }),
    enabled: !!workspaceId,
    staleTime: 30_000,
  });
}

/**
 * Return MRs linked to a task.
 *
 * Reads from the active workspace's MR cache so the hook signature stays
 * identical to the old Zustand-backed version (single taskId arg). The active
 * workspace ID is read from the Zustand UI store — that slice is retained as
 * UI state per the migration plan.
 */
export function useTaskMRs(taskId: string | null): TaskMR[] {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const { data } = useQuery({
    queryKey: qk.gitlab.mrs(workspaceId ?? ""),
    queryFn: () => listWorkspaceTaskMRs(workspaceId!, { cache: "no-store" }),
    enabled: !!workspaceId,
    staleTime: 30_000,
    select: (d) => (taskId ? (d?.task_mrs[taskId] ?? EMPTY_MRS) : EMPTY_MRS),
  });
  return data ?? EMPTY_MRS;
}

/**
 * Returns whether GitLab is configured enough to surface in the integrations
 * menu. Token-configured or authenticated counts as "available".
 *
 * Uses useQuery so the status is cached and deduped across consumers.
 * Replaces the manual mount + window focus listener pattern.
 */
export function useGitLabAvailable(): boolean {
  const { data } = useQuery({
    queryKey: ["gitlab", "status"] as const,
    queryFn: () => fetchGitLabStatus({ cache: "no-store" }),
    staleTime: 30_000,
    select: (s) => Boolean(s?.authenticated || s?.token_configured),
  });
  return data ?? false;
}
