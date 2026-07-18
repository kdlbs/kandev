"use client";

import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { listWorkspaceAzureDevOpsTaskPullRequests } from "@/lib/api/domains/azure-devops-api";
import type { AzureDevOpsTaskPullRequest } from "@/lib/types/azure-devops";

const pendingWorkspaces = new Map<string, Promise<void>>();
const workspaceSnapshots = new Map<string, Record<string, AzureDevOpsTaskPullRequest[]>>();

function loadWorkspace(
  workspaceId: string,
  setAll: (items: Record<string, AzureDevOpsTaskPullRequest[]>) => void,
) {
  const pending = pendingWorkspaces.get(workspaceId);
  if (pending) return pending;
  const request = listWorkspaceAzureDevOpsTaskPullRequests(workspaceId, { cache: "no-store" })
    .then((result) => {
      const snapshot = result.taskPrs ?? {};
      workspaceSnapshots.set(workspaceId, snapshot);
      setAll(snapshot);
    })
    .finally(() => pendingWorkspaces.delete(workspaceId));
  pendingWorkspaces.set(workspaceId, request);
  return request;
}

export function useAzureDevOpsTaskPullRequests(workspaceId: string | null, taskId: string | null) {
  const pullRequests = useAppStore((state) =>
    taskId ? (state.azureDevOpsTaskPullRequests.byTaskId[taskId] ?? []) : [],
  );
  const setAll = useAppStore((state) => state.setAzureDevOpsTaskPullRequests);

  useEffect(() => {
    if (!workspaceId) return;
    const snapshot = workspaceSnapshots.get(workspaceId);
    if (snapshot) {
      setAll(snapshot);
      return;
    }
    void loadWorkspace(workspaceId, setAll).catch(() => undefined);
  }, [setAll, workspaceId]);

  return pullRequests;
}
