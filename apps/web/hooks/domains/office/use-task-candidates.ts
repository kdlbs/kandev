"use client";

import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { searchTasks } from "@/lib/api/domains/office-extended-api";
import type { OfficeTask } from "@/lib/state/slices/office/types";

const EMPTY_TASKS: OfficeTask[] = [];
const CANDIDATE_LIMIT = 50;

/**
 * Candidate tasks for the parent / blockers pickers, read from TanStack
 * Query via the workspace task-search endpoint.
 *
 * Replaces the legacy `useAppStore(s => s.office.tasks.items)` mirror read
 * (with its lazy `searchTasks` fallback). The pickers don't need the full
 * paginated/filtered list — just a bounded set of candidates to choose
 * from — so this fetches its own search-backed cache, keyed separately from
 * the office tasks list so the two never collide.
 */
export function useTaskCandidates(): OfficeTask[] {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const { data } = useQuery({
    queryKey: ["office", workspaceId ?? "", "taskCandidates"] as const,
    queryFn: () => searchTasks(workspaceId ?? "", "", CANDIDATE_LIMIT).then((r) => r.tasks ?? []),
    enabled: !!workspaceId,
    staleTime: 30_000,
  });
  return data ?? EMPTY_TASKS;
}
