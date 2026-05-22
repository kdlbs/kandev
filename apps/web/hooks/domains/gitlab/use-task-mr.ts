"use client";

import { useEffect, useRef, useState } from "react";
import { fetchGitLabStatus, listWorkspaceTaskMRs } from "@/lib/api/domains/gitlab-api";
import { useAppStore } from "@/components/state-provider";
import type { TaskMR } from "@/lib/types/gitlab";

/**
 * Hydrate the gitlab task-MRs slice for a workspace. Fetches once per
 * workspaceId switch and clears the cache on null. Mirrors useWorkspacePRs
 * for GitHub but stays minimal (no WS subscription yet — that lands with
 * the poller in a follow-up phase).
 */
export function useWorkspaceMRs(workspaceId: string | null) {
  const setTaskMRs = useAppStore((state) => state.setTaskMRs);
  const resetTaskMRs = useAppStore((state) => state.resetTaskMRs);
  const fetchedRef = useRef<string | null>(null);
  const requestRef = useRef(0);

  useEffect(() => {
    if (!workspaceId) {
      // Invalidate any in-flight request and clear the cached MRs so a
      // workspace switch / sign-out doesn't leave the previous workspace's
      // MRs visible until the next fetch.
      requestRef.current += 1;
      fetchedRef.current = null;
      resetTaskMRs();
      return;
    }
    if (fetchedRef.current === workspaceId) return;
    const requestId = ++requestRef.current;
    fetchedRef.current = workspaceId;
    listWorkspaceTaskMRs(workspaceId, { cache: "no-store" })
      .then((response) => {
        if (requestRef.current !== requestId) return;
        setTaskMRs(response?.task_mrs ?? {});
      })
      .catch(() => {
        if (requestRef.current === requestId) {
          fetchedRef.current = null; // allow retry on failure
        }
      });
  }, [workspaceId, setTaskMRs, resetTaskMRs]);
}

// Stable empty array so the zustand selector output stays referentially
// equal across renders when a task has no MRs. Returning a fresh [] each
// call triggers an infinite re-render loop.
const EMPTY_MRS: TaskMR[] = [];

/** Return MRs linked to a task. Reads directly from the store. */
export function useTaskMRs(taskId: string | null): TaskMR[] {
  return useAppStore((state) =>
    taskId ? (state.taskMRs.byTaskId[taskId] ?? EMPTY_MRS) : EMPTY_MRS,
  );
}

/**
 * Returns whether GitLab is configured enough to surface in the integrations
 * menu. Token-configured or authenticated counts as "available" — same bar
 * as useGitHubStatus's `ready` flag. Probes /status on mount + after window
 * regains focus so settings changes propagate without a hard reload.
 */
export function useGitLabAvailable(): boolean {
  const [available, setAvailable] = useState(false);
  useEffect(() => {
    let cancelled = false;
    const probe = () => {
      // `cache` MUST be top-level — fetchJson reads options.cache directly
      // and overwrites init.cache with undefined. See lib/api/client.ts.
      fetchGitLabStatus({ cache: "no-store" })
        .then((s) => {
          if (!cancelled) setAvailable(Boolean(s?.authenticated || s?.token_configured));
        })
        .catch(() => {
          if (!cancelled) setAvailable(false);
        });
    };
    probe();
    window.addEventListener("focus", probe);
    return () => {
      cancelled = true;
      window.removeEventListener("focus", probe);
    };
  }, []);
  return available;
}
