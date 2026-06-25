import { describe, expect, it } from "vitest";
import type { BackendMessageMap, BackendMessageType } from "@/lib/types/backend";
import type { BackendMessage } from "@/lib/types/backend-message";
import type { Task, WorkflowSnapshot } from "@/lib/types/http";
import type { WebSocketClient } from "@/lib/ws/client";
import { makeQueryClient } from "../client";
import { qk } from "../keys";
import { registerTaskBridge } from "./tasks";

type AnyBackendMessage = BackendMessage<string, Record<string, unknown>>;
type Handler = (message: AnyBackendMessage) => void;

const WORKSPACE_ID = "workspace-1";

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
    created_at: "2026-06-24T00:00:00Z",
    updated_at: "2026-06-24T00:00:00Z",
  } as unknown as Task;
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

describe("task query bridge task detail", () => {
  it("upserts and invalidates task detail when an update arrives before detail is cached", () => {
    const { ws, queryClient, cleanup } = setupBridge();

    ws.emit({
      type: "notification",
      action: "task.updated",
      payload: {
        task_id: "task-1",
        workflow_id: "wf-1",
        workflow_step_id: "step-1",
        title: "Renamed sender",
        description: "updated",
        state: "IN_PROGRESS",
        position: 2,
        is_ephemeral: false,
      },
    });

    expect(queryClient.getQueryData(qk.tasks.detail("task-1"))).toMatchObject({
      id: "task-1",
      task_id: "task-1",
      workflow_id: "wf-1",
      workflow_step_id: "step-1",
      title: "Renamed sender",
      description: "updated",
      state: "IN_PROGRESS",
      position: 2,
    });
    expect(queryClient.getQueryState(qk.tasks.detail("task-1"))?.isInvalidated).toBe(true);

    cleanup();
  });
});

describe("task query bridge workflow snapshots", () => {
  it("patches a cached workflow snapshot when a task is updated", () => {
    const { ws, queryClient, cleanup } = setupBridge();
    queryClient.setQueryData(
      qk.workflows.snapshot("wf-1"),
      [makeSnapshot("wf-1", "step-1", [makeTask("task-1", "wf-1", "step-1", "Old title")])][0],
    );

    ws.emit({
      type: "notification",
      action: "task.updated",
      payload: {
        task_id: "task-1",
        workflow_id: "wf-1",
        workflow_step_id: "step-1",
        title: "New title",
        description: "updated",
        state: "IN_PROGRESS",
        position: 4,
        primary_session_id: "session-1",
        is_ephemeral: false,
      },
    });

    const snapshot = queryClient.getQueryData<WorkflowSnapshot>(qk.workflows.snapshot("wf-1"));
    expect(snapshot?.tasks).toEqual([
      expect.objectContaining({
        id: "task-1",
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
      makeSnapshot("wf-old", "old-step", [makeTask("task-1", "wf-old", "old-step")]),
    );
    queryClient.setQueryData(
      qk.workflows.snapshot("wf-new"),
      makeSnapshot("wf-new", "new-step", []),
    );

    ws.emit({
      type: "notification",
      action: "task.updated",
      payload: {
        task_id: "task-1",
        workflow_id: "wf-new",
        old_workflow_id: "wf-old",
        workflow_step_id: "new-step",
        title: "Moved task",
        description: "",
        state: "TODO",
        position: 0,
        is_ephemeral: false,
      },
    });

    const oldSnapshot = queryClient.getQueryData<WorkflowSnapshot>(qk.workflows.snapshot("wf-old"));
    const newSnapshot = queryClient.getQueryData<WorkflowSnapshot>(qk.workflows.snapshot("wf-new"));
    expect(oldSnapshot?.tasks).toEqual([]);
    expect(newSnapshot?.tasks).toEqual([
      expect.objectContaining({
        id: "task-1",
        workflow_id: "wf-new",
        workflow_step_id: "new-step",
        title: "Moved task",
      }),
    ]);

    cleanup();
  });
});
