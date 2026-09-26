"use client";

import { useCallback, useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { deleteTaskMR, listWorkspaceTaskMRs } from "@/lib/api/domains/gitlab-api";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import type { TaskMR } from "@/lib/types/gitlab";
import { useGitLabStatus } from "./use-gitlab-status";

type WorkspaceMRResponse = Awaited<ReturnType<typeof listWorkspaceTaskMRs>>;
const workspaceMRRequests = new Map<string, Promise<WorkspaceMRResponse>>();

function acquireWorkspaceMRRequest(workspaceId: string) {
  const activeRequest = workspaceMRRequests.get(workspaceId);
  if (activeRequest) return activeRequest;

  const request = listWorkspaceTaskMRs(workspaceId, { cache: "no-store" });
  workspaceMRRequests.set(workspaceId, request);
  const release = () => {
    if (workspaceMRRequests.get(workspaceId) === request) {
      workspaceMRRequests.delete(workspaceId);
    }
  };
  void request.then(release, release);
  return request;
}

/**
 * Hydrate the GitLab task-MRs slice for a workspace. Co-mounted consumers
 * share one in-flight request. A later mount can start a fresh request. The
 * hook clears the cache when the workspace becomes null or a refresh fails.
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
    const request = acquireWorkspaceMRRequest(workspaceId);
    // Keep the current data visible until the shared request succeeds. A
    // failure clears that workspace below, so stale data cannot remain.
    request
      .then((response) => {
        if (requestRef.current !== requestId) return;
        setTaskMRs(workspaceId, response?.task_mrs ?? {});
      })
      .catch(() => {
        if (requestRef.current === requestId) {
          resetTaskMRs(workspaceId);
          fetchedRef.current = null; // allow retry on failure
        }
      });
    return () => {
      if (requestRef.current === requestId) requestRef.current += 1;
      if (fetchedRef.current === workspaceId) fetchedRef.current = null;
    };
  }, [workspaceId, setTaskMRs, resetTaskMRs]);
}

// Stable empty array so the zustand selector output stays referentially
// equal across renders when a task has no MRs. Returning a fresh [] each
// call triggers an infinite re-render loop.
const EMPTY_MRS: TaskMR[] = [];

/** Return MRs linked to a task. Reads directly from the store. */
export function useTaskMRs(taskId: string | null, requestedWorkspaceId?: string | null): TaskMR[] {
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const workspaceId = requestedWorkspaceId === undefined ? activeWorkspaceId : requestedWorkspaceId;
  return useAppStore((state) => {
    if (!taskId || !workspaceId) return EMPTY_MRS;
    return state.taskMRs.byWorkspaceId[workspaceId]?.[taskId] ?? EMPTY_MRS;
  });
}

/**
 * Returns whether GitLab is configured enough to surface in the integrations
 * menu. Token-configured or authenticated counts as "available" — same bar
 * as useGitHubStatus's `ready` flag. Backed by the store-cached
 * useGitLabStatus hook, so multiple consumers share a single fetch and the
 * status doesn't re-probe on every window focus.
 */
export function useGitLabAvailable(): boolean {
  const { status } = useGitLabStatus();
  return Boolean(status?.authenticated || status?.token_configured);
}

/**
 * Unlink closure shared by MRTopbarButton and MRStatusChip so the two
 * surfaces can never disagree on unlink behaviour (extracted rather than
 * duplicated — apps/web/eslint.config.mjs forbids identical functions and
 * 4+ duplicated strings, so a second copy would be a lint failure).
 * `workspaceId` may be null across the hook's unconditional call in a
 * component that only invokes the returned closure once a workspace is
 * known, mirroring how MRTopbarButton already calls its hooks before its
 * own `!workspaceId` early return.
 */
export function useUnlinkTaskMR(
  workspaceId: string | null,
): (associationId: string) => Promise<void> {
  const { t } = useTranslation();
  const removeTaskMR = useAppStore((state) => state.removeTaskMR);
  const { toast } = useToast();

  return useCallback(
    async (associationId: string) => {
      if (!workspaceId) return;
      try {
        await deleteTaskMR(associationId, workspaceId);
        removeTaskMR(workspaceId, associationId);
      } catch (error) {
        toast({
          title: t("gitlab:failedToUnlinkMergeRequest"),
          description:
            error instanceof Error ? error.message : t("gitlab:theMergeRequestIsStillLinked"),
          variant: "error",
        });
      }
    },
    [workspaceId, removeTaskMR, toast, t],
  );
}
