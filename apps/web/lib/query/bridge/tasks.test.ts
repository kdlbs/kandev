import type { QueryClient } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import type { BackendMessageMap, BackendMessageType } from "@/lib/types/backend";
import type { BackendMessage } from "@/lib/types/backend-message";
import type { Task, WorkflowSnapshot } from "@/lib/types/http";
import type { WebSocketClient } from "@/lib/ws/client";
import { taskId } from "@/lib/types/ids";
import { makeQueryClient } from "../client";
import { qk } from "../keys";
import { registerTaskBridge } from "./tasks";

type AnyBackendMessage = BackendMessage<string, Record<string, unknown>>;
type Handler = (message: AnyBackendMessage) => void;
type TaskCreatedPayload = BackendMessageMap["task.created"]["payload"];
type TaskUpdatedPayload = BackendMessageMap["task.updated"]["payload"];

const WORKSPACE_ID = "workspace-1";
const TASK_ID = "task-1";
const PARENT_TASK_ID = taskId("parent-1");
const WF_ID = "wf-1";
const STEP_ID = "step-1";
const TASK_CREATED_ACTION = "task.created";
const TASK_UPDATED_ACTION = "task.updated";
const TEST_TIMESTAMP = "2026-06-24T00:00:00Z";
const RENAMED_CHILD_TITLE = "Renamed child";

class FakeWebSocketClient {
  private handlers = new Map<string, Set<Handler>>();

  on<T extends BackendMessageType>(type: T, handler: (message: BackendMessageMap[T]) => void) {
    const bucket = this.handlers.get(type) ?? new Set<Handler>();
    bucket.add(handler as Handler);
    this.handlers.set(type, bucket);
    return () => {
      bucket.delete(handler as Handler);
    };
  }

  emit(message: AnyBackendMessage) {
    this.handlers.get(message.action)?.forEach((handler) => handler(message));
  }
}

function makeTask(id: string, workflowId: string, stepId: string, title = "Task"): Task {
  return {
    id,
    workspace_id: WORKSPACE_ID,
    workflow_id: workflowId,
    workflow_step_id: stepId,
    position: 0,
    title,
    description: "",
    state: "TODO",
    priority: 0,
    repositories: [],
    created_at: TEST_TIMESTAMP,
    updated_at: TEST_TIMESTAMP,
  } as unknown as Task;
}

function makeTaskRepository(repositoryId: string): NonNullable<Task["repositories"]>[number] {
  return {
    id: `task-repo-${repositoryId}`,
    task_id: TASK_ID,
    repository_id: repositoryId,
    base_branch: "main",
    checkout_branch: "feature/rename",
    position: 0,
    created_at: TEST_TIMESTAMP,
    updated_at: TEST_TIMESTAMP,
  } as NonNullable<Task["repositories"]>[number];
}

function makeSnapshot(workflowId: string, stepId: string, tasks: Task[]): WorkflowSnapshot {
  return {
    workflow: {
      id: workflowId,
      workspace_id: WORKSPACE_ID,
      name: workflowId,
      sort_order: 0,
      hidden: false,
    },
    steps: [
      {
        id: stepId,
        workflow_id: workflowId,
        name: "Todo",
        position: 0,
        color: "bg-blue-500",
        allow_manual_move: true,
      },
    ],
    tasks,
  } as WorkflowSnapshot;
}

function setupBridge() {
  const ws = new FakeWebSocketClient();
  const queryClient = makeQueryClient();
  const registration = registerTaskBridge(ws as unknown as WebSocketClient, queryClient);
  return { ws, queryClient, cleanup: registration.cleanup };
}

function taskUpdatedPayload(overrides: Partial<TaskUpdatedPayload> = {}): TaskUpdatedPayload {
  return {
    task_id: TASK_ID,
    workflow_id: WF_ID,
    workflow_step_id: STEP_ID,
    title: "Updated task",
    description: "",
    state: "TODO",
    is_ephemeral: false,
    ...overrides,
  };
}

function emitTaskUpdated(ws: FakeWebSocketClient, overrides: Partial<TaskUpdatedPayload> = {}) {
  ws.emit({
    type: "notification",
    action: TASK_UPDATED_ACTION,
    payload: taskUpdatedPayload(overrides),
  });
}

function cacheTaskDetailWithRepository(queryClient: QueryClient, repositoryId = "repo-a") {
  const repo = makeTaskRepository(repositoryId);
  queryClient.setQueryData(qk.tasks.detail(TASK_ID), {
    ...makeTask(TASK_ID, WF_ID, STEP_ID, "Old title"),
    repositories: [repo],
  });
  return repo;
}

function cacheWorkflowSnapshotWithRepository(queryClient: QueryClient, repositoryId = "repo-a") {
  const repo = makeTaskRepository(repositoryId);
  const existingTask = {
    ...makeTask(TASK_ID, WF_ID, STEP_ID, "Old title"),
    repositories: [repo],
  };
  queryClient.setQueryData(
    qk.workflows.snapshot(WF_ID),
    makeSnapshot(WF_ID, STEP_ID, [existingTask]),
  );
  return repo;
}

describe("task query bridge task detail core", () => {
  it("upserts and invalidates task detail when an update arrives before detail is cached", () => {
    const { ws, queryClient, cleanup } = setupBridge();

    emitTaskUpdated(ws, {
      title: "Renamed sender",
      description: "updated",
      state: "IN_PROGRESS",
      position: 2,
    });

    expect(queryClient.getQueryData(qk.tasks.detail(TASK_ID))).toMatchObject({
      id: TASK_ID,
      task_id: TASK_ID,
      workflow_id: WF_ID,
      workflow_step_id: STEP_ID,
      title: "Renamed sender",
      description: "updated",
      state: "IN_PROGRESS",
      position: 2,
    });
    expect(queryClient.getQueryState(qk.tasks.detail(TASK_ID))?.isInvalidated).toBe(true);

    cleanup();
  });

  it("clears the cached archive timestamp when an unarchive update arrives", () => {
    const { ws, queryClient, cleanup } = setupBridge();
    queryClient.setQueryData(qk.tasks.detail(TASK_ID), {
      ...makeTask(TASK_ID, WF_ID, STEP_ID),
      archived_at: "2026-07-14T12:00:00Z",
    });

    emitTaskUpdated(ws, { archived_at: null });

    expect(queryClient.getQueryData(qk.tasks.detail(TASK_ID))).toMatchObject({
      archived_at: null,
    });

    cleanup();
  });
});

describe("task query bridge task detail repository metadata", () => {
  it("preserves cached metadata when a rename update omits repository fields", () => {
    const { ws, queryClient, cleanup } = setupBridge();
    const repo = cacheTaskDetailWithRepository(queryClient);

    emitTaskUpdated(ws, { title: "Renamed detail" });

    expect(queryClient.getQueryData(qk.tasks.detail(TASK_ID))).toMatchObject({
      title: "Renamed detail",
      repositories: [repo],
    });

    cleanup();
  });

  it("does not preserve stale cached repositories when the primary repository changes", () => {
    const { ws, queryClient, cleanup } = setupBridge();
    cacheTaskDetailWithRepository(queryClient);

    emitTaskUpdated(ws, {
      title: "Retargeted detail",
      repository_id: "repo-b",
    });

    expect(queryClient.getQueryData(qk.tasks.detail(TASK_ID))).toMatchObject({
      repositories: [expect.objectContaining({ repository_id: "repo-b" })],
    });

    cleanup();
  });

  it("clears cached repository metadata when an update explicitly sends an empty list", () => {
    const { ws, queryClient, cleanup } = setupBridge();
    cacheTaskDetailWithRepository(queryClient);

    emitTaskUpdated(ws, {
      title: "Repo-less detail",
      repositories: [],
    });

    expect(queryClient.getQueryData(qk.tasks.detail(TASK_ID))).toMatchObject({
      repositories: [],
    });

    cleanup();
  });
});

describe("task query bridge task detail parent metadata", () => {
  it("preserves parent links when an update omits parent_id", () => {
    const { ws, queryClient, cleanup } = setupBridge();
    queryClient.setQueryData(qk.tasks.detail(TASK_ID), {
      ...makeTask(TASK_ID, WF_ID, STEP_ID, "Old title"),
      parent_id: PARENT_TASK_ID,
    });

    emitTaskUpdated(ws, { title: RENAMED_CHILD_TITLE });

    expect(queryClient.getQueryData(qk.tasks.detail(TASK_ID))).toMatchObject({
      title: RENAMED_CHILD_TITLE,
      parent_id: PARENT_TASK_ID,
    });

    cleanup();
  });

  it("clears parent links when an update explicitly sends parent_id null", () => {
    const { ws, queryClient, cleanup } = setupBridge();
    queryClient.setQueryData(qk.tasks.detail(TASK_ID), {
      ...makeTask(TASK_ID, WF_ID, STEP_ID, "Old title"),
      parent_id: PARENT_TASK_ID,
    });

    emitTaskUpdated(ws, { title: "Promoted task", parent_id: null });

    expect(
      queryClient.getQueryData<Record<string, unknown>>(qk.tasks.detail(TASK_ID))?.parent_id,
    ).toBeUndefined();

    cleanup();
  });
});

describe("task query bridge workflow snapshots", () => {
  it("patches a cached workflow snapshot when a task is created", () => {
    const { ws, queryClient, cleanup } = setupBridge();
    queryClient.setQueryData(qk.workflows.snapshot(WF_ID), makeSnapshot(WF_ID, STEP_ID, []));

    ws.emit({
      type: "notification",
      action: TASK_CREATED_ACTION,
      payload: {
        task_id: "child-before-snapshot",
        workflow_id: WF_ID,
        workflow_step_id: STEP_ID,
        title: "Child before snapshot",
        state: "IN_PROGRESS",
        parent_id: PARENT_TASK_ID,
        is_ephemeral: false,
        updated_at: TEST_TIMESTAMP,
      } satisfies TaskCreatedPayload,
    });

    const snapshot = queryClient.getQueryData<WorkflowSnapshot>(qk.workflows.snapshot(WF_ID));
    expect(snapshot?.tasks).toEqual([
      expect.objectContaining({
        id: "child-before-snapshot",
        title: "Child before snapshot",
        state: "IN_PROGRESS",
        parent_id: PARENT_TASK_ID,
      }),
    ]);
    expect(queryClient.getQueryState(qk.workflows.snapshot(WF_ID))?.isInvalidated).toBe(true);

    cleanup();
  });

  it("patches a cached workflow snapshot when a task is updated", () => {
    const { ws, queryClient, cleanup } = setupBridge();
    queryClient.setQueryData(
      qk.workflows.snapshot(WF_ID),
      makeSnapshot(WF_ID, STEP_ID, [makeTask(TASK_ID, WF_ID, STEP_ID, "Old title")]),
    );

    emitTaskUpdated(ws, {
      title: "New title",
      description: "updated",
      state: "IN_PROGRESS",
      position: 4,
      primary_session_id: "session-1",
    });

    const snapshot = queryClient.getQueryData<WorkflowSnapshot>(qk.workflows.snapshot(WF_ID));
    expect(snapshot?.tasks).toEqual([
      expect.objectContaining({
        id: TASK_ID,
        title: "New title",
        description: "updated",
        state: "IN_PROGRESS",
        position: 4,
        primary_session_id: "session-1",
      }),
    ]);

    cleanup();
  });

  it("moves cached workflow snapshot tasks when a task changes workflows", () => {
    const { ws, queryClient, cleanup } = setupBridge();
    queryClient.setQueryData(
      qk.workflows.snapshot("wf-old"),
      makeSnapshot("wf-old", "old-step", [makeTask(TASK_ID, "wf-old", "old-step")]),
    );
    queryClient.setQueryData(
      qk.workflows.snapshot("wf-new"),
      makeSnapshot("wf-new", "new-step", []),
    );

    emitTaskUpdated(ws, {
      workflow_id: "wf-new",
      old_workflow_id: "wf-old",
      workflow_step_id: "new-step",
      title: "Moved task",
      position: 0,
    });

    const oldSnapshot = queryClient.getQueryData<WorkflowSnapshot>(qk.workflows.snapshot("wf-old"));
    const newSnapshot = queryClient.getQueryData<WorkflowSnapshot>(qk.workflows.snapshot("wf-new"));
    expect(oldSnapshot?.tasks).toEqual([]);
    expect(newSnapshot?.tasks).toEqual([
      expect.objectContaining({
        id: TASK_ID,
        workflow_id: "wf-new",
        workflow_step_id: "new-step",
        title: "Moved task",
      }),
    ]);

    cleanup();
  });
});

describe("task query bridge workflow snapshot archive state", () => {
  it("removes archived tasks", () => {
    const { ws, queryClient, cleanup } = setupBridge();
    queryClient.setQueryData(
      qk.workflows.snapshot(WF_ID),
      makeSnapshot(WF_ID, STEP_ID, [makeTask(TASK_ID, WF_ID, STEP_ID, "Old title")]),
    );

    emitTaskUpdated(ws, { archived_at: "2026-06-30T12:00:00Z" });

    const snapshot = queryClient.getQueryData<WorkflowSnapshot>(qk.workflows.snapshot(WF_ID));
    expect(snapshot?.tasks).toEqual([]);

    cleanup();
  });

  it("restores unarchived tasks", () => {
    const { ws, queryClient, cleanup } = setupBridge();
    queryClient.setQueryData(qk.workflows.snapshot(WF_ID), makeSnapshot(WF_ID, STEP_ID, []));

    emitTaskUpdated(ws, { archived_at: null, title: "Restored task" });

    const snapshot = queryClient.getQueryData<WorkflowSnapshot>(qk.workflows.snapshot(WF_ID));
    expect(snapshot?.tasks).toEqual([
      expect.objectContaining({ id: TASK_ID, title: "Restored task" }),
    ]);

    cleanup();
  });
});

describe("task query bridge workflow snapshot parent metadata", () => {
  it("preserves parent links when an update omits parent_id", () => {
    const { ws, queryClient, cleanup } = setupBridge();
    queryClient.setQueryData(
      qk.workflows.snapshot(WF_ID),
      makeSnapshot(WF_ID, STEP_ID, [
        { ...makeTask(TASK_ID, WF_ID, STEP_ID, "Old title"), parent_id: PARENT_TASK_ID } as Task,
      ]),
    );

    emitTaskUpdated(ws, { title: RENAMED_CHILD_TITLE });

    const snapshot = queryClient.getQueryData<WorkflowSnapshot>(qk.workflows.snapshot(WF_ID));
    expect(snapshot?.tasks[0]).toEqual(
      expect.objectContaining({ title: RENAMED_CHILD_TITLE, parent_id: PARENT_TASK_ID }),
    );

    cleanup();
  });

  it("applies explicit parent_id when adding a task from an event", () => {
    const { ws, queryClient, cleanup } = setupBridge();
    queryClient.setQueryData(qk.workflows.snapshot(WF_ID), makeSnapshot(WF_ID, STEP_ID, []));

    emitTaskUpdated(ws, { title: "New child", parent_id: PARENT_TASK_ID });

    const snapshot = queryClient.getQueryData<WorkflowSnapshot>(qk.workflows.snapshot(WF_ID));
    expect(snapshot?.tasks[0]).toEqual(
      expect.objectContaining({ title: "New child", parent_id: PARENT_TASK_ID }),
    );

    cleanup();
  });

  it("clears parent links when an update explicitly sends parent_id null", () => {
    const { ws, queryClient, cleanup } = setupBridge();
    queryClient.setQueryData(
      qk.workflows.snapshot(WF_ID),
      makeSnapshot(WF_ID, STEP_ID, [
        { ...makeTask(TASK_ID, WF_ID, STEP_ID, "Old title"), parent_id: PARENT_TASK_ID } as Task,
      ]),
    );

    emitTaskUpdated(ws, { title: "Promoted task", parent_id: null });

    const snapshot = queryClient.getQueryData<WorkflowSnapshot>(qk.workflows.snapshot(WF_ID));
    expect(snapshot?.tasks[0]?.parent_id).toBeUndefined();

    cleanup();
  });
});

describe("task query bridge workflow snapshot repository metadata", () => {
  it("preserves repository metadata when a rename update omits repository fields", () => {
    const { ws, queryClient, cleanup } = setupBridge();
    const repo = cacheWorkflowSnapshotWithRepository(queryClient);

    emitTaskUpdated(ws, { title: "Renamed task" });

    const snapshot = queryClient.getQueryData<WorkflowSnapshot>(qk.workflows.snapshot(WF_ID));
    expect(snapshot?.tasks[0]).toEqual(expect.objectContaining({ title: "Renamed task" }));
    expect(snapshot?.tasks[0]?.repositories).toEqual([repo]);

    cleanup();
  });

  it("does not preserve stale repository rows when the primary repository changes", () => {
    const { ws, queryClient, cleanup } = setupBridge();
    cacheWorkflowSnapshotWithRepository(queryClient);

    emitTaskUpdated(ws, {
      title: "Retargeted task",
      repository_id: "repo-b",
    });

    const snapshot = queryClient.getQueryData<WorkflowSnapshot>(qk.workflows.snapshot(WF_ID));
    expect(snapshot?.tasks[0]?.repositories).toEqual([
      expect.objectContaining({ repository_id: "repo-b" }),
    ]);

    cleanup();
  });

  it("clears repository metadata when an update explicitly sends an empty repository list", () => {
    const { ws, queryClient, cleanup } = setupBridge();
    cacheWorkflowSnapshotWithRepository(queryClient);

    emitTaskUpdated(ws, {
      title: "Repo-less task",
      repositories: [],
    });

    const snapshot = queryClient.getQueryData<WorkflowSnapshot>(qk.workflows.snapshot(WF_ID));
    expect(snapshot?.tasks[0]?.repositories).toEqual([]);

    cleanup();
  });
});
