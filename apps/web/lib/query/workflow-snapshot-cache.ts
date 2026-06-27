import type { QueryClient } from "@tanstack/react-query";
import type { WorkflowSnapshot } from "@/lib/types/http";

function isWorkflowSnapshotQueryKey(key: readonly unknown[]): boolean {
  return key[0] === "workflows" && typeof key[1] === "string" && key[2] === "snapshot";
}

function isWorkflowSnapshot(value: unknown): value is WorkflowSnapshot {
  return (
    typeof value === "object" &&
    value !== null &&
    "workflow" in value &&
    "tasks" in value &&
    Array.isArray((value as { tasks?: unknown }).tasks)
  );
}

export function updateWorkflowSnapshotQueries(
  queryClient: QueryClient,
  updater: (snapshot: WorkflowSnapshot) => WorkflowSnapshot,
): void {
  for (const query of queryClient.getQueryCache().findAll()) {
    if (!isWorkflowSnapshotQueryKey(query.queryKey)) continue;
    queryClient.setQueryData(query.queryKey, (current: unknown) =>
      isWorkflowSnapshot(current) ? updater(current) : current,
    );
  }
}

export function workflowSnapshotQueryDataForWorkflow(
  queryClient: QueryClient,
  workflowId: string,
): WorkflowSnapshot | undefined {
  const data = queryClient.getQueryData(["workflows", workflowId, "snapshot"]);
  return isWorkflowSnapshot(data) ? data : undefined;
}

export function updateWorkflowSnapshotQuery(
  queryClient: QueryClient,
  workflowId: string,
  updater: (snapshot: WorkflowSnapshot) => WorkflowSnapshot,
): void {
  queryClient.setQueryData(["workflows", workflowId, "snapshot"], (current: unknown) =>
    isWorkflowSnapshot(current) ? updater(current) : current,
  );
}

export function workflowSnapshotQueryData(queryClient: QueryClient): WorkflowSnapshot[] {
  const snapshots: WorkflowSnapshot[] = [];
  for (const query of queryClient.getQueryCache().findAll()) {
    if (!isWorkflowSnapshotQueryKey(query.queryKey)) continue;
    const data = queryClient.getQueryData(query.queryKey);
    if (isWorkflowSnapshot(data)) snapshots.push(data);
  }
  return snapshots;
}

export function removeTasksFromWorkflowSnapshotQueries(
  queryClient: QueryClient,
  ids: Set<string>,
): void {
  updateWorkflowSnapshotQueries(queryClient, (snapshot) => {
    if (!snapshot.tasks.some((task) => ids.has(task.id))) return snapshot;
    return {
      ...snapshot,
      tasks: snapshot.tasks.filter((task) => !ids.has(task.id)),
    };
  });
}
