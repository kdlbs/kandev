import { describe, it, expect } from "vitest";
import { registerTasksHandlers } from "./tasks";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { KanbanState } from "@/lib/state/slices/kanban/types";

type KanbanTask = KanbanState["tasks"][number];

function makeStore(initialTasks: KanbanTask[] = []): StoreApi<AppState> {
  const state: Record<string, unknown> = {
    kanban: { workflowId: "wf-1", steps: [], tasks: initialTasks },
    kanbanMulti: {
      snapshots: {
        "wf-1": {
          workflowId: "wf-1",
          workflowName: "W",
          steps: [],
          tasks: [...initialTasks],
        },
      },
      isLoading: false,
    },
    tasks: { activeTaskId: null, activeSessionId: null },
    taskSessionsByTask: { itemsByTaskId: {} },
  };
  return {
    getState: () => state as unknown as AppState,
    setState: (updater: (s: AppState) => AppState) => {
      const next = updater(state as unknown as AppState);
      Object.assign(state, next);
    },
    subscribe: () => () => {},
    destroy: () => {},
    getInitialState: () => state as unknown as AppState,
  } as unknown as StoreApi<AppState>;
}

function getMultiTask(store: StoreApi<AppState>, id: string) {
  return store.getState().kanbanMulti.snapshots["wf-1"].tasks.find((t) => t.id === id);
}

const TASK_UPDATED = "task.updated";

function basePayload(overrides: Record<string, unknown> = {}) {
  return {
    task_id: "task-1",
    workflow_id: "wf-1",
    workflow_step_id: "step-1",
    title: "T",
    description: "D",
    position: 0,
    state: "TODO" as const,
    is_ephemeral: false,
    ...overrides,
  };
}

function taskUpdatedMessage(payload: Record<string, unknown>) {
  return { id: "m", type: "notification", action: TASK_UPDATED, payload };
}

describe("task.updated handler — isPRReview", () => {
  it("marks task as PR review when metadata carries review_watch_id", () => {
    const store = makeStore();
    const handler = registerTasksHandlers(store)[TASK_UPDATED]!;

    handler(
      taskUpdatedMessage(
        basePayload({
          metadata: { review_watch_id: "watch-xyz" },
        }),
      ),
    );

    const task = store.getState().kanban.tasks.find((t) => t.id === "task-1")!;
    expect(task.isPRReview).toBe(true);
    expect(getMultiTask(store, "task-1")?.isPRReview).toBe(true);
  });

  it("preserves existing isPRReview when orchestrator payload omits metadata", () => {
    const existing: KanbanTask = {
      id: "task-1",
      workflowStepId: "step-1",
      title: "T",
      position: 0,
      isPRReview: true,
    };
    const store = makeStore([existing]);
    const handler = registerTasksHandlers(store)[TASK_UPDATED]!;

    // Simulate orchestrator-sourced update: no metadata field in payload.
    handler(taskUpdatedMessage(basePayload({ title: "T2" })));

    const task = store.getState().kanban.tasks.find((t) => t.id === "task-1")!;
    expect(task.isPRReview).toBe(true);
    expect(task.title).toBe("T2");
    // Multi-kanban snapshot follows the same path and must also be preserved.
    expect(getMultiTask(store, "task-1")?.isPRReview).toBe(true);
  });

  it("defaults to false for a brand-new task without metadata", () => {
    const store = makeStore();
    const handler = registerTasksHandlers(store)[TASK_UPDATED]!;

    handler(taskUpdatedMessage(basePayload()));

    const task = store.getState().kanban.tasks.find((t) => t.id === "task-1")!;
    expect(task.isPRReview).toBe(false);
    expect(task.isIssueWatch).toBe(false);
  });

  it("derives issue watch fields from metadata", () => {
    const store = makeStore();
    const handler = registerTasksHandlers(store)[TASK_UPDATED]!;

    handler(
      taskUpdatedMessage(
        basePayload({
          metadata: {
            issue_watch_id: "watch-9",
            issue_url: "https://github.com/owner/repo/issues/42",
            issue_number: 42,
          },
        }),
      ),
    );

    const task = store.getState().kanban.tasks.find((t) => t.id === "task-1")!;
    expect(task.isIssueWatch).toBe(true);
    expect(task.issueUrl).toBe("https://github.com/owner/repo/issues/42");
    expect(task.issueNumber).toBe(42);
  });
});
