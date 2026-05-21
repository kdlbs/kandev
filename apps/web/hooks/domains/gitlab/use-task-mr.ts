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
  const fetchedRef = useRef<string | null>(null);
  const requestRef = useRef(0);

  useEffect(() => {
    if (!workspaceId) {
      fetchedRef.current = null;
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
  }, [workspaceId, setTaskMRs]);
}

/** Return MRs linked to a task. Reads directly from the store. */
export function useTaskMRs(taskId: string | null): TaskMR[] {
  return useAppStore((state) => (taskId ? (state.taskMRs.byTaskId[taskId] ?? []) : []));
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
      fetchGitLabStatus({ init: { cache: "no-store" } })
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
