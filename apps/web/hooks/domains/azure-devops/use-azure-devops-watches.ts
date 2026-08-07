"use client";

import { useCallback, useEffect, useState } from "react";
import {
  createAzureDevOpsPullRequestWatch,
  createAzureDevOpsWorkItemWatch,
  deleteAzureDevOpsPullRequestWatch,
  deleteAzureDevOpsWorkItemWatch,
  listAzureDevOpsPullRequestWatches,
  listAzureDevOpsWorkItemWatches,
  previewAzureDevOpsPullRequestWatchReset,
  previewAzureDevOpsWorkItemWatchReset,
  resetAzureDevOpsPullRequestWatch,
  resetAzureDevOpsWorkItemWatch,
  triggerAzureDevOpsPullRequestWatch,
  triggerAzureDevOpsWorkItemWatch,
  updateAzureDevOpsPullRequestWatch,
  updateAzureDevOpsWorkItemWatch,
} from "@/lib/api/domains/azure-devops-api";
import type {
  AzureDevOpsPullRequestWatch,
  AzureDevOpsPullRequestWatchInput,
  AzureDevOpsWatchResetResult,
  AzureDevOpsWorkItemWatch,
  AzureDevOpsWorkItemWatchInput,
} from "@/lib/types/azure-devops";

export function normalizeAzureWatchInput<
  T extends { pollIntervalSeconds: number; maxInflightTasks?: number },
>(input: T): T {
  const next = { ...input, pollIntervalSeconds: Math.max(60, input.pollIntervalSeconds || 300) };
  if (!input.maxInflightTasks) delete next.maxInflightTasks;
  return next;
}

type WatchState = {
  workItems: AzureDevOpsWorkItemWatch[];
  pullRequests: AzureDevOpsPullRequestWatch[];
  loading: boolean;
  error: string | null;
};

const WORKSPACE_REQUIRED = "workspace is required";

// eslint-disable-next-line max-lines-per-function -- this hook exposes the complete CRUD and reset API for both watch kinds.
export function useAzureDevOpsWatches(workspaceId?: string) {
  const [state, setState] = useState<WatchState>({
    workItems: [],
    pullRequests: [],
    loading: false,
    error: null,
  });
  const [revision, setRevision] = useState(0);
  const refresh = useCallback(() => setRevision((value) => value + 1), []);
  useEffect(() => {
    if (!workspaceId) {
      setState({ workItems: [], pullRequests: [], loading: false, error: null });
      return;
    }
    let cancelled = false;
    setState((previous) => ({ ...previous, loading: true, error: null }));
    Promise.all([
      listAzureDevOpsWorkItemWatches(workspaceId),
      listAzureDevOpsPullRequestWatches(workspaceId),
    ])
      .then(([workItems, pullRequests]) => {
        if (!cancelled)
          setState({
            workItems: workItems.watches ?? [],
            pullRequests: pullRequests.watches ?? [],
            loading: false,
            error: null,
          });
      })
      .catch((error) => {
        if (!cancelled)
          setState((previous) => ({ ...previous, loading: false, error: String(error) }));
      });
    return () => {
      cancelled = true;
    };
  }, [revision, workspaceId]);

  const mutate = useCallback(
    async <T>(operation: () => Promise<T>): Promise<T> => {
      const result = await operation();
      refresh();
      return result;
    },
    [refresh],
  );
  const createWorkItem = useCallback(
    (input: AzureDevOpsWorkItemWatchInput) => {
      if (!workspaceId) return Promise.reject(new Error(WORKSPACE_REQUIRED));
      return mutate(() =>
        createAzureDevOpsWorkItemWatch(workspaceId, normalizeAzureWatchInput(input)),
      );
    },
    [mutate, workspaceId],
  );
  const createPullRequest = useCallback(
    (input: AzureDevOpsPullRequestWatchInput) => {
      if (!workspaceId) return Promise.reject(new Error(WORKSPACE_REQUIRED));
      return mutate(() =>
        createAzureDevOpsPullRequestWatch(workspaceId, normalizeAzureWatchInput(input)),
      );
    },
    [mutate, workspaceId],
  );
  const updateWorkItem = useCallback(
    (id: string, input: Partial<AzureDevOpsWorkItemWatchInput> & { enabled?: boolean }) => {
      if (!workspaceId) return Promise.reject(new Error(WORKSPACE_REQUIRED));
      return mutate(() => updateAzureDevOpsWorkItemWatch(workspaceId, id, input));
    },
    [mutate, workspaceId],
  );
  const updatePullRequest = useCallback(
    (id: string, input: Partial<AzureDevOpsPullRequestWatchInput> & { enabled?: boolean }) => {
      if (!workspaceId) return Promise.reject(new Error(WORKSPACE_REQUIRED));
      return mutate(() => updateAzureDevOpsPullRequestWatch(workspaceId, id, input));
    },
    [mutate, workspaceId],
  );
  const remove = useCallback(
    (kind: "work-item" | "pull-request", id: string) => {
      if (!workspaceId) return Promise.reject(new Error(WORKSPACE_REQUIRED));
      return mutate(() =>
        kind === "work-item"
          ? deleteAzureDevOpsWorkItemWatch(workspaceId, id)
          : deleteAzureDevOpsPullRequestWatch(workspaceId, id),
      );
    },
    [mutate, workspaceId],
  );
  const trigger = useCallback(
    (kind: "work-item" | "pull-request", id: string) => {
      if (!workspaceId) return Promise.reject(new Error(WORKSPACE_REQUIRED));
      return kind === "work-item"
        ? triggerAzureDevOpsWorkItemWatch(workspaceId, id)
        : triggerAzureDevOpsPullRequestWatch(workspaceId, id);
    },
    [workspaceId],
  );
  const previewReset = useCallback(
    (kind: "work-item" | "pull-request", id: string): Promise<{ taskCount: number }> => {
      if (!workspaceId) return Promise.reject(new Error(WORKSPACE_REQUIRED));
      return kind === "work-item"
        ? previewAzureDevOpsWorkItemWatchReset(workspaceId, id)
        : previewAzureDevOpsPullRequestWatchReset(workspaceId, id);
    },
    [workspaceId],
  );
  const reset = useCallback(
    (kind: "work-item" | "pull-request", id: string): Promise<AzureDevOpsWatchResetResult> => {
      if (!workspaceId) return Promise.reject(new Error(WORKSPACE_REQUIRED));
      return mutate(() =>
        kind === "work-item"
          ? resetAzureDevOpsWorkItemWatch(workspaceId, id)
          : resetAzureDevOpsPullRequestWatch(workspaceId, id),
      );
    },
    [mutate, workspaceId],
  );
  return {
    ...state,
    refresh,
    createWorkItem,
    createPullRequest,
    updateWorkItem,
    updatePullRequest,
    remove,
    trigger,
    previewReset,
    reset,
  };
}
