"use client";

import { useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { listWorkspaceAzureDevOpsTaskWorkItems } from "@/lib/api/domains/azure-devops-api";
import type { AzureDevOpsTaskWorkItem } from "@/lib/types/azure-devops";

type WorkspaceSnapshot = Record<string, AzureDevOpsTaskWorkItem[]>;

const EMPTY_TASK_WORK_ITEMS: AzureDevOpsTaskWorkItem[] = [];
const pendingWorkspaces = new Map<string, Promise<WorkspaceSnapshot>>();
const workspaceSnapshots = new Map<string, WorkspaceSnapshot>();
const workspaceUpdates = new Map<string, WorkspaceSnapshot>();

function withTaskWorkItem(
  snapshot: WorkspaceSnapshot,
  taskId: string,
  workItem: AzureDevOpsTaskWorkItem,
): WorkspaceSnapshot {
  const existing = snapshot[taskId] ?? [];
  const index = existing.findIndex((item) => item.id === workItem.id);
  const taskWorkItems = [...existing];
  if (index >= 0) taskWorkItems[index] = workItem;
  else taskWorkItems.push(workItem);
  return { ...snapshot, [taskId]: taskWorkItems };
}

function mergeWorkspaceUpdates(workspaceId: string, snapshot: WorkspaceSnapshot) {
  const updates = workspaceUpdates.get(workspaceId);
  if (!updates) return snapshot;
  let merged = snapshot;
  for (const [taskId, workItems] of Object.entries(updates)) {
    for (const workItem of workItems) merged = withTaskWorkItem(merged, taskId, workItem);
  }
  workspaceUpdates.delete(workspaceId);
  return merged;
}

export function cacheAzureDevOpsTaskWorkItem(
  workspaceId: string,
  taskId: string,
  workItem: AzureDevOpsTaskWorkItem,
) {
  const snapshot = workspaceSnapshots.get(workspaceId);
  if (snapshot) {
    workspaceSnapshots.set(workspaceId, withTaskWorkItem(snapshot, taskId, workItem));
    return;
  }
  const updates = workspaceUpdates.get(workspaceId) ?? {};
  workspaceUpdates.set(workspaceId, withTaskWorkItem(updates, taskId, workItem));
}

function loadWorkspace(workspaceId: string) {
  const pending = pendingWorkspaces.get(workspaceId);
  if (pending) return pending;
  const request = listWorkspaceAzureDevOpsTaskWorkItems(workspaceId, { cache: "no-store" })
    .then((result) => {
      const snapshot = mergeWorkspaceUpdates(workspaceId, result.taskWorkItems ?? {});
      workspaceSnapshots.set(workspaceId, snapshot);
      return snapshot;
    })
    .finally(() => pendingWorkspaces.delete(workspaceId));
  pendingWorkspaces.set(workspaceId, request);
  return request;
}

export function useAzureDevOpsTaskWorkItems(workspaceId: string | null, taskId: string | null) {
  const generation = useRef({ scope: workspaceId, value: 0 });
  if (generation.current.scope !== workspaceId) {
    generation.current = { scope: workspaceId, value: generation.current.value + 1 };
  }
  const workItems = useAppStore((state) =>
    taskId
      ? (state.azureDevOpsTaskWorkItems.byTaskId[taskId] ?? EMPTY_TASK_WORK_ITEMS)
      : EMPTY_TASK_WORK_ITEMS,
  );
  const setAll = useAppStore((state) => state.setAzureDevOpsTaskWorkItems);

  useEffect(() => {
    if (!workspaceId) return;
    const current = generation.current.value;
    const applySnapshot = (snapshot: WorkspaceSnapshot) => {
      if (current === generation.current.value) setAll(snapshot);
    };
    const snapshot = workspaceSnapshots.get(workspaceId);
    if (snapshot) {
      applySnapshot(snapshot);
      return;
    }
    void loadWorkspace(workspaceId)
      .then(applySnapshot)
      .catch(() => undefined);
  }, [generation, setAll, workspaceId]);

  return workItems;
}
