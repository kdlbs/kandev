"use client";

import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import {
  listWorkspaceAzureDevOpsTaskPullRequests,
  syncAzureDevOpsTaskPullRequest,
} from "@/lib/api/domains/azure-devops-api";
import type { AzureDevOpsTaskPullRequest } from "@/lib/types/azure-devops";

type WorkspaceSnapshot = Record<string, AzureDevOpsTaskPullRequest[]>;

const TASK_PR_REFRESH_INTERVAL_MS = 90_000;
const EMPTY_TASK_PULL_REQUESTS: AzureDevOpsTaskPullRequest[] = [];
const pendingWorkspaces = new Map<string, Promise<void>>();
const pendingTaskPullRequests = new Map<string, Promise<AzureDevOpsTaskPullRequest>>();
const workspaceSnapshots = new Map<string, WorkspaceSnapshot>();
const workspaceUpdates = new Map<string, WorkspaceSnapshot>();

function withTaskPullRequest(
  snapshot: WorkspaceSnapshot,
  taskId: string,
  pullRequest: AzureDevOpsTaskPullRequest,
): WorkspaceSnapshot {
  const existing = snapshot[taskId] ?? [];
  const index = existing.findIndex((item) => item.id === pullRequest.id);
  const taskPullRequests = [...existing];
  if (index >= 0) taskPullRequests[index] = pullRequest;
  else taskPullRequests.push(pullRequest);
  return { ...snapshot, [taskId]: taskPullRequests };
}

function mergeWorkspaceUpdates(workspaceId: string, snapshot: WorkspaceSnapshot) {
  const updates = workspaceUpdates.get(workspaceId);
  if (!updates) return snapshot;
  let merged = snapshot;
  for (const [taskId, pullRequests] of Object.entries(updates)) {
    for (const pullRequest of pullRequests) {
      merged = withTaskPullRequest(merged, taskId, pullRequest);
    }
  }
  workspaceUpdates.delete(workspaceId);
  return merged;
}

// Keep successful associations in the module snapshot so remounting a task
// chip cannot overwrite the Zustand update with an older workspace response.
export function cacheAzureDevOpsTaskPullRequest(
  workspaceId: string,
  taskId: string,
  pullRequest: AzureDevOpsTaskPullRequest,
) {
  const snapshot = workspaceSnapshots.get(workspaceId);
  if (snapshot) {
    workspaceSnapshots.set(workspaceId, withTaskPullRequest(snapshot, taskId, pullRequest));
    return;
  }
  const updates = workspaceUpdates.get(workspaceId) ?? {};
  workspaceUpdates.set(workspaceId, withTaskPullRequest(updates, taskId, pullRequest));
}

function loadWorkspace(
  workspaceId: string,
  setAll: (items: Record<string, AzureDevOpsTaskPullRequest[]>) => void,
) {
  const pending = pendingWorkspaces.get(workspaceId);
  if (pending) return pending;
  const request = listWorkspaceAzureDevOpsTaskPullRequests(workspaceId, { cache: "no-store" })
    .then((result) => {
      const snapshot = mergeWorkspaceUpdates(workspaceId, result.taskPrs ?? {});
      workspaceSnapshots.set(workspaceId, snapshot);
      setAll(snapshot);
    })
    .finally(() => pendingWorkspaces.delete(workspaceId));
  pendingWorkspaces.set(workspaceId, request);
  return request;
}

function shouldRefresh(pullRequest: AzureDevOpsTaskPullRequest) {
  const lastSyncedAt = Date.parse(pullRequest.lastSyncedAt ?? "");
  return !Number.isFinite(lastSyncedAt) || Date.now() - lastSyncedAt >= TASK_PR_REFRESH_INTERVAL_MS;
}

function refreshTaskPullRequest(
  workspaceId: string,
  taskId: string,
  pullRequest: AzureDevOpsTaskPullRequest,
) {
  const key = `${workspaceId}:${pullRequest.id}`;
  const pending = pendingTaskPullRequests.get(key);
  if (pending) return pending;
  const request = syncAzureDevOpsTaskPullRequest(workspaceId, taskId, {
    repositoryId: pullRequest.repositoryId,
    pullRequestId: pullRequest.pullRequestId,
  }).finally(() => pendingTaskPullRequests.delete(key));
  pendingTaskPullRequests.set(key, request);
  return request;
}

export function useAzureDevOpsTaskPullRequests(workspaceId: string | null, taskId: string | null) {
  const pullRequests = useAppStore((state) =>
    taskId
      ? (state.azureDevOpsTaskPullRequests.byTaskId[taskId] ?? EMPTY_TASK_PULL_REQUESTS)
      : EMPTY_TASK_PULL_REQUESTS,
  );
  const setAll = useAppStore((state) => state.setAzureDevOpsTaskPullRequests);
  const setOne = useAppStore((state) => state.setAzureDevOpsTaskPullRequest);

  useEffect(() => {
    if (!workspaceId) return;
    const snapshot = workspaceSnapshots.get(workspaceId);
    if (snapshot) {
      setAll(snapshot);
      return;
    }
    void loadWorkspace(workspaceId, setAll).catch(() => undefined);
  }, [setAll, workspaceId]);

  useEffect(() => {
    if (!workspaceId || !taskId) return;
    for (const pullRequest of pullRequests) {
      if (!shouldRefresh(pullRequest)) continue;
      void refreshTaskPullRequest(workspaceId, taskId, pullRequest)
        .then((refreshed) => {
          cacheAzureDevOpsTaskPullRequest(workspaceId, taskId, refreshed);
          setOne(taskId, refreshed);
        })
        .catch(() => undefined);
    }
  }, [pullRequests, setOne, taskId, workspaceId]);

  return pullRequests;
}
