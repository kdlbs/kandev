import type { QueryClient } from "@tanstack/react-query";
import type { BackendMessageMap } from "@/lib/types/backend";
import { primaryTaskRepository, type Task, type WorkflowSnapshot } from "@/lib/types/http";
import type { WebSocketClient } from "@/lib/ws/client";
import { qk } from "../keys";
import { registerBridgeHandlers, type QueryBridgeRegistration } from "./registrar";

type TaskEventMessage =
  | BackendMessageMap["task.created"]
  | BackendMessageMap["task.deleted"]
  | BackendMessageMap["task.state_changed"]
  | BackendMessageMap["task.updated"];

type CachedTask = Record<string, unknown>;

export function registerTaskBridge(
  ws: WebSocketClient,
  queryClient: QueryClient,
): QueryBridgeRegistration {
  return registerBridgeHandlers(ws, queryClient, {
    "kanban.update": (message) => {
      queryClient.invalidateQueries({
        exact: true,
        queryKey: qk.workflows.snapshot(message.payload.workflowId),
      });
    },
    "task.created": (message) => {
      if (!message.payload.is_ephemeral) {
        upsertSnapshotTask(queryClient, message.payload.workflow_id, message.payload);
      }
      invalidateTaskPages(queryClient);
      invalidateWorkflowSnapshots(queryClient, message);
    },
    "task.deleted": (message) => {
      queryClient.removeQueries({
        exact: true,
        queryKey: qk.tasks.detail(message.payload.task_id),
      });
      removeSnapshotTask(queryClient, message.payload.workflow_id, message.payload.task_id);
      invalidateTaskPages(queryClient);
      invalidateWorkflowSnapshots(queryClient, message);
    },
    "task.state_changed": (message) => {
      patchTaskDetail(queryClient, message);
      if (!message.payload.is_ephemeral) {
        upsertSnapshotTask(queryClient, message.payload.workflow_id, message.payload);
      }
      invalidateTaskPages(queryClient);
      invalidateWorkflowSnapshots(queryClient, message);
    },
    "task.updated": (message) => {
      patchTaskDetail(queryClient, message);
      patchSnapshotTaskFromEvent(queryClient, message);
      invalidateTaskPages(queryClient);
      invalidateWorkflowSnapshots(queryClient, message);
    },
  });
}

function patchTaskDetail(queryClient: QueryClient, message: TaskEventMessage): void {
  const payload = message.payload;
  const queryKey = qk.tasks.detail(payload.task_id);
  queryClient.setQueryData(queryKey, (current: unknown) => {
    if (!isCachedTask(current)) return taskDetailFromPayload(payload);
    return taskDetailFromPayload(payload, current);
  });
  queryClient.invalidateQueries({ exact: true, queryKey });
}

function invalidateTaskPages(queryClient: QueryClient): void {
  queryClient.invalidateQueries({ queryKey: ["tasks", "page"] });
  queryClient.invalidateQueries({ queryKey: ["tasks", "infinite"] });
}

function invalidateWorkflowSnapshots(queryClient: QueryClient, message: TaskEventMessage): void {
  const payload = message.payload;
  queryClient.invalidateQueries({
    exact: true,
    queryKey: qk.workflows.snapshot(payload.workflow_id),
  });
  if (payload.old_workflow_id && payload.old_workflow_id !== payload.workflow_id) {
    queryClient.invalidateQueries({
      exact: true,
      queryKey: qk.workflows.snapshot(payload.old_workflow_id),
    });
  }
}

function patchSnapshotTaskFromEvent(queryClient: QueryClient, message: TaskEventMessage): void {
  const payload = message.payload;
  if (payload.old_workflow_id && payload.old_workflow_id !== payload.workflow_id) {
    removeSnapshotTask(queryClient, payload.old_workflow_id, payload.task_id);
  }
  if (payload.archived_at) {
    removeSnapshotTask(queryClient, payload.workflow_id, payload.task_id);
    return;
  }
  if (payload.is_ephemeral) return;
  upsertSnapshotTask(queryClient, payload.workflow_id, payload);
}

function upsertSnapshotTask(
  queryClient: QueryClient,
  workflowId: string,
  payload: TaskEventMessage["payload"],
): void {
  queryClient.setQueryData(qk.workflows.snapshot(workflowId), (current: unknown) => {
    if (!isWorkflowSnapshot(current)) return current;
    const existing = current.tasks.find((task) => task.id === payload.task_id);
    const nextTask = taskFromPayload(payload, current, existing);
    const tasks = existing
      ? current.tasks.map((task) => (task.id === payload.task_id ? nextTask : task))
      : [...current.tasks, nextTask];
    return { ...current, tasks };
  });
}

function removeSnapshotTask(queryClient: QueryClient, workflowId: string, taskId: string): void {
  queryClient.setQueryData(qk.workflows.snapshot(workflowId), (current: unknown) => {
    if (!isWorkflowSnapshot(current)) return current;
    if (!current.tasks.some((task) => task.id === taskId)) return current;
    return {
      ...current,
      tasks: current.tasks.filter((task) => task.id !== taskId),
    };
  });
}

function taskFromPayload(
  payload: TaskEventMessage["payload"],
  snapshot: WorkflowSnapshot,
  existing: Task | undefined,
): Task {
  return {
    ...(existing ?? {}),
    id: payload.task_id,
    workspace_id: workspaceIdFromSnapshot(snapshot, existing),
    workflow_id: payload.workflow_id,
    workflow_step_id: payload.workflow_step_id,
    parent_id: parentIdFromPayload(payload, existing),
    position: positionFromPayload(payload, existing),
    title: payload.title,
    description: descriptionFromPayload(payload, existing),
    state: stateFromPayload(payload, existing),
    priority: priorityFromPayload(payload, existing),
    repositories: repositoriesFromPayload(payload, existing?.repositories),
    primary_session_id: primarySessionIdFromPayload(payload, existing),
    session_count: sessionCountFromPayload(payload, existing),
    review_status: reviewStatusFromPayload(payload, existing),
    created_at: createdAtFromPayload(payload, existing),
    updated_at: updatedAtFromPayload(payload, existing),
  } as Task;
}

function workspaceIdFromSnapshot(snapshot: WorkflowSnapshot, existing: Task | undefined) {
  return existing?.workspace_id ?? snapshot.workflow.workspace_id;
}

function positionFromPayload(payload: TaskEventMessage["payload"], existing: Task | undefined) {
  return payload.position ?? existing?.position ?? 0;
}

function parentIdFromPayload(payload: TaskEventMessage["payload"], existing: Task | undefined) {
  if (Object.prototype.hasOwnProperty.call(payload, "parent_id")) {
    return payload.parent_id ?? undefined;
  }
  return existing?.parent_id;
}

function descriptionFromPayload(payload: TaskEventMessage["payload"], existing: Task | undefined) {
  return payload.description ?? existing?.description ?? "";
}

function stateFromPayload(payload: TaskEventMessage["payload"], existing: Task | undefined) {
  return payload.state ?? existing?.state ?? "TODO";
}

function priorityFromPayload(payload: TaskEventMessage["payload"], existing: Task | undefined) {
  return payload.priority ?? existing?.priority ?? 0;
}

function sessionCountFromPayload(payload: TaskEventMessage["payload"], existing: Task | undefined) {
  return payload.session_count ?? existing?.session_count;
}

function reviewStatusFromPayload(payload: TaskEventMessage["payload"], existing: Task | undefined) {
  return payload.review_status ?? existing?.review_status;
}

function createdAtFromPayload(payload: TaskEventMessage["payload"], existing: Task | undefined) {
  return existing?.created_at ?? payload.updated_at ?? "";
}

function updatedAtFromPayload(payload: TaskEventMessage["payload"], existing: Task | undefined) {
  return payload.updated_at ?? existing?.updated_at ?? "";
}

function repositoriesFromPayload(
  payload: TaskEventMessage["payload"],
  existingRepositories: Task["repositories"] | undefined,
): Task["repositories"] {
  if (payload.repositories !== undefined) {
    return payload.repositories as Task["repositories"];
  }
  const payloadRepositoryId = payload.repository_id;
  const existingRepositoryId = primaryRepositoryId(existingRepositories);
  if (payloadRepositoryId && payloadRepositoryId !== existingRepositoryId) {
    return [fallbackTaskRepository(payload, payloadRepositoryId)];
  }
  if (existingRepositories) return existingRepositories;
  if (!payloadRepositoryId) return [];
  return [fallbackTaskRepository(payload, payloadRepositoryId)];
}

function primaryRepositoryId(repositories: Task["repositories"] | undefined): string | undefined {
  return primaryTaskRepository(repositories)?.repository_id;
}

function fallbackTaskRepository(
  payload: TaskEventMessage["payload"],
  repositoryId: string,
): NonNullable<Task["repositories"]>[number] {
  return {
    id: "",
    task_id: payload.task_id,
    repository_id: repositoryId,
    base_branch: "",
    position: 0,
    created_at: payload.updated_at ?? "",
    updated_at: payload.updated_at ?? "",
  } as NonNullable<Task["repositories"]>[number];
}

function taskDetailFromPayload(
  payload: TaskEventMessage["payload"],
  existing: CachedTask = {},
): CachedTask {
  return {
    ...existing,
    ...payload,
    repositories: repositoriesFromPayload(payload, cachedRepositories(existing)),
    ...taskDetailCoreFields(payload, existing),
    ...taskDetailSessionFields(payload, existing),
    ...taskDetailTimestampFields(payload, existing),
  };
}

function cachedRepositories(existing: CachedTask): Task["repositories"] | undefined {
  return Array.isArray(existing.repositories)
    ? (existing.repositories as Task["repositories"])
    : undefined;
}

function taskDetailCoreFields(
  payload: TaskEventMessage["payload"],
  existing: CachedTask,
): CachedTask {
  return {
    id: existing.id ?? payload.task_id,
    description: payload.description ?? existing.description ?? "",
    state: payload.state ?? existing.state ?? "TODO",
    priority: payload.priority ?? existing.priority ?? 0,
    position: payload.position ?? existing.position ?? 0,
    parent_id: cachedParentIdFromPayload(payload, existing),
  };
}

function cachedParentIdFromPayload(
  payload: TaskEventMessage["payload"],
  existing: CachedTask,
): unknown {
  if (Object.prototype.hasOwnProperty.call(payload, "parent_id")) {
    return payload.parent_id ?? undefined;
  }
  return existing.parent_id;
}

function taskDetailSessionFields(
  payload: TaskEventMessage["payload"],
  existing: CachedTask,
): CachedTask {
  return {
    primary_session_id: cachedPrimarySessionIdFromPayload(payload, existing),
    session_count: payload.session_count ?? existing.session_count,
    review_status: payload.review_status ?? existing.review_status,
    archived_at: cachedArchivedAtFromPayload(payload, existing),
  };
}

function cachedArchivedAtFromPayload(
  payload: TaskEventMessage["payload"],
  existing: CachedTask,
): unknown {
  if (payload.archived_at !== undefined) return payload.archived_at;
  return existing.archived_at;
}

function taskDetailTimestampFields(
  payload: TaskEventMessage["payload"],
  existing: CachedTask,
): CachedTask {
  return {
    updated_at: payload.updated_at ?? existing.updated_at ?? "",
    created_at: existing.created_at ?? payload.updated_at ?? "",
  };
}

function cachedPrimarySessionIdFromPayload(
  payload: TaskEventMessage["payload"],
  existing: CachedTask,
): unknown {
  if (payload.primary_session_id === undefined) return existing.primary_session_id;
  return payload.primary_session_id;
}

function primarySessionIdFromPayload(
  payload: TaskEventMessage["payload"],
  existing: Task | undefined,
): Task["primary_session_id"] {
  if (payload.primary_session_id === undefined) return existing?.primary_session_id;
  return payload.primary_session_id as Task["primary_session_id"];
}

function isCachedTask(value: unknown): value is CachedTask {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isWorkflowSnapshot(value: unknown): value is WorkflowSnapshot {
  return (
    isCachedTask(value) &&
    isCachedTask(value.workflow) &&
    Array.isArray(value.steps) &&
    Array.isArray(value.tasks)
  );
}
