"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { useWorkspacePRs } from "@/hooks/domains/github/use-task-pr";
import { listWorkspaceTaskPRs } from "@/lib/api/domains/github-api";
import type { TaskPR } from "@/lib/types/github";

const EMPTY_PRS: ReadonlyMap<string, TaskPR[]> = new Map();

/**
 * Reads the shared `taskPRs` store slice (`state.taskPRs.byTaskId`), kept
 * current by the existing PR WebSocket handlers (docs/specs/coordinator/
 * system-design/needs-you.md#inputs). Like `workflows.items`, this is a
 * single non-partitioned cache holding one workspace's data at a time
 * (`setTaskPRs` wholesale-replaces `byTaskId`), so only `useWorkspacePRs`
 * fetches into it, and only when `workspaceId` is the workspace it is
 * currently scoped to; the read below matches on `taskPRs.workspaceId` alone
 * (not `workspaceContextGeneration`, which tracks the globally *active*
 * workspace and is irrelevant to a route that may not be it).
 */
function useCoordinatorPRsFromActiveCache(
  workspaceId: string | null,
): ReadonlyMap<string, TaskPR[]> {
  useWorkspacePRs(workspaceId);
  const byTaskId = useAppStore((state) =>
    workspaceId && state.taskPRs.workspaceId === workspaceId ? state.taskPRs.byTaskId : null,
  );
  return useMemo(() => (byTaskId ? new Map(Object.entries(byTaskId)) : EMPTY_PRS), [byTaskId]);
}

/**
 * Self-contained fetch for a workspace that is NOT the globally active one.
 * Fetching into the shared `taskPRs` slice for a non-active workspace would
 * wholesale-overwrite whatever the active workspace's own PR cache holds
 * (the same class of bug fixed for tasks/workflow snapshots in
 * `use-coordinator-tasks.ts`), so this reads directly into local hook state.
 * Trade-off: no live WebSocket updates on this path, only mount — acceptable
 * because it is only reached for the non-active-workspace edge case.
 */
function useCoordinatorPRsDirect(workspaceId: string | null): ReadonlyMap<string, TaskPR[]> {
  const [prsByTaskId, setPrsByTaskId] = useState<ReadonlyMap<string, TaskPR[]>>(EMPTY_PRS);
  const requestRef = useRef(0);

  useEffect(() => {
    // Reset before fetching, not just when `workspaceId` goes null: this
    // effect's own instance can persist across a route change from one
    // non-active workspace straight to another (no `key` remounts
    // `CoordinatorRoute`), so a stale prior workspace's PR data must never
    // survive into the next one, even transiently (mirrors
    // `use-coordinator-list.ts`'s reset-before-fetch pattern).
    setPrsByTaskId(EMPTY_PRS);
    if (!workspaceId) return;
    const requestId = ++requestRef.current;
    listWorkspaceTaskPRs(workspaceId, { cache: "no-store" })
      .then((response) => {
        if (requestRef.current !== requestId) return;
        setPrsByTaskId(new Map(Object.entries(response?.task_prs ?? {})));
      })
      .catch(() => {
        // No banner surfaces PR-read failures (Adoption decision: the row's
        // own fallback text covers a missing PR); leave the prior map as-is.
      });
  }, [workspaceId]);

  return prsByTaskId;
}

/**
 * PR detail input for the Queue screen's In review / Ready to merge rows, for
 * the route's own `workspaceId` regardless of which workspace is globally
 * active (docs/specs/coordinator/system-design/needs-you.md#inputs).
 */
export function useCoordinatorPRs(workspaceId: string | null): ReadonlyMap<string, TaskPR[]> {
  const activeId = useAppStore((state) => state.workspaces.activeId);
  const isActiveWorkspace = workspaceId !== null && workspaceId === activeId;

  const liveMap = useCoordinatorPRsFromActiveCache(isActiveWorkspace ? workspaceId : null);
  const directMap = useCoordinatorPRsDirect(isActiveWorkspace ? null : workspaceId);

  return isActiveWorkspace ? liveMap : directMap;
}
