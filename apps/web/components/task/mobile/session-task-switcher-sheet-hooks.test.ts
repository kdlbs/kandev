import { describe, expect, it, vi } from "vitest";
import { toSheetItem } from "./session-task-switcher-sheet-item";
import {
  handleTaskSheetOpenChange,
  selectPendingTaskFromSheet,
  selectTaskFromSheet,
} from "./session-task-switcher-sheet-selection";
import type { TaskSession } from "@/lib/types/http";

type SheetTask = Parameters<typeof toSheetItem>[0];
type SheetCtx = Parameters<typeof toSheetItem>[1];
const UPDATED_AT = "2026-07-22T00:00:00Z";
const ERROR_PREVIEW = "Agent failed";

function emptyCtx(): SheetCtx {
  return {
    repositoryPathsById: new Map(),
    workflowNameById: new Map(),
    stepTitleById: new Map(),
  };
}

function task(overrides: Partial<SheetTask> = {}): SheetTask {
  return {
    id: "t1",
    _workflowId: "wf1",
    title: "Task",
    state: "IN_PROGRESS",
    workflowStepId: "step-1",
    ...overrides,
  } as SheetTask;
}

describe("toSheetItem", () => {
  it("carries the autopilot marker onto the mobile sheet row", () => {
    expect(toSheetItem(task({ autopilot: true }), emptyCtx()).autopilot).toBe(true);
  });

  // The mobile task-switcher row must read the same task-level most-active-wins
  // aggregate the desktop sidebar and board card read, so a background-running
  // secondary session is caught on mobile too.
  it("carries the task-level foreground_activity aggregate onto the mobile sheet row", () => {
    const item = toSheetItem(task({ foregroundActivity: "background" }), emptyCtx());
    expect(item.foregroundActivity).toBe("background");
  });

  it("carries the generating aggregate through unchanged", () => {
    const item = toSheetItem(task({ foregroundActivity: "generating" }), emptyCtx());
    expect(item.foregroundActivity).toBe("generating");
  });

  it("passes an absent aggregate through as undefined (safe → not-background)", () => {
    const item = toSheetItem(task(), emptyCtx());
    expect(item.foregroundActivity).toBeUndefined();
  });

  it("preserves the archived marker for projected rows", () => {
    expect(toSheetItem(task({ isArchived: true }), emptyCtx()).isArchived).toBe(true);
  });

  it("reads pending permission from the task status summary", () => {
    const item = toSheetItem(
      task({
        statusSummary: {
          revision: 2,
          updated_at: UPDATED_AT,
          pending_action: "permission",
        },
      }),
      emptyCtx(),
    );
    expect(item.hasPendingPermission).toBe(true);
    expect(item.hasPendingClarification).toBe(false);
  });

  it("reads an active error from the task status summary", () => {
    const item = toSheetItem(
      task({
        statusSummary: {
          revision: 3,
          updated_at: UPDATED_AT,
          active_error: {
            session_id: "session-1",
            stamp: "error-3",
            occurred_at: UPDATED_AT,
            preview: ERROR_PREVIEW,
          },
        },
      }),
      emptyCtx(),
    );

    expect(item.agentErrorMessage).toBe(ERROR_PREVIEW);
  });

  it("does not resurrect legacy status when a summary explicitly clears it", () => {
    const item = toSheetItem(
      task({
        taskPendingAction: "permission",
        primarySessionState: "RUNNING",
        primarySessionId: "legacy-session",
        foregroundActivity: "background",
        updatedAt: "legacy-update",
        statusSummary: {
          revision: 4,
          updated_at: UPDATED_AT,
        },
      }),
      emptyCtx(),
    );

    expect(item.hasPendingPermission).toBe(false);
    expect(item.sessionState).toBeUndefined();
    expect(item.primarySessionId).toBeNull();
    expect(item.foregroundActivity).toBeUndefined();
    expect(item.updatedAt).toBe(UPDATED_AT);
  });

  it("hides only the acknowledged error stamp and shows a newer one", () => {
    const base = task({
      statusSummary: {
        revision: 5,
        updated_at: UPDATED_AT,
        active_error: {
          session_id: "session-1",
          stamp: "error-5",
          occurred_at: UPDATED_AT,
          preview: ERROR_PREVIEW,
        },
      },
    });

    expect(
      toSheetItem(base, {
        ...emptyCtx(),
        acknowledgedAgentErrors: { "session-1": "error-5" },
      }).agentErrorMessage,
    ).toBeNull();
    expect(
      toSheetItem(base, {
        ...emptyCtx(),
        dismissedAgentErrors: { "session-1": "older-error" },
      }).agentErrorMessage,
    ).toBe(ERROR_PREVIEW);
  });
});

describe("toSheetItem queued prompt count", () => {
  it("carries the queued prompt count from the task status summary", () => {
    const item = toSheetItem(
      task({ statusSummary: { revision: 3, updated_at: UPDATED_AT, queued_prompt_count: 2 } }),
      emptyCtx(),
    );
    expect(item.queuedCount).toBe(2);
  });

  it("leaves queuedCount undefined when nothing is queued", () => {
    const item = toSheetItem(
      task({ statusSummary: { revision: 4, updated_at: UPDATED_AT } }),
      emptyCtx(),
    );
    expect(item.queuedCount).toBeUndefined();
  });

  it("carries WIP queue status separately from queued prompts", () => {
    const wipQueue = { position: 1, total: 3, destinationTitle: "Review" };
    const item = toSheetItem(task(), {
      ...emptyCtx(),
      wipQueueByTaskId: new Map([["t1", wipQueue]]),
    });

    expect(item.wipQueue).toEqual(wipQueue);
    expect(item.queuedCount).toBeUndefined();
  });
});

describe("selectPendingTaskFromSheet", () => {
  it("waits for the owner session before navigating and closing", async () => {
    const order: string[] = [];
    const setActiveSession = vi.fn((_taskId: string, sessionId: string) => {
      order.push(`session:${sessionId}`);
    });
    await selectPendingTaskFromSheet({
      taskId: "task-1",
      preferredSessionId: "primary",
      taskPendingAction: "clarification",
      loadTaskSessionsForTask: vi.fn(
        async () =>
          [
            {
              id: "secondary",
              task_id: "task-1",
              state: "WAITING_FOR_INPUT",
              pending_action: "clarification",
              started_at: UPDATED_AT,
              updated_at: UPDATED_AT,
            },
            {
              id: "primary",
              task_id: "task-1",
              state: "WAITING_FOR_INPUT",
              is_primary: true,
              started_at: UPDATED_AT,
              updated_at: UPDATED_AT,
            },
          ] as TaskSession[],
      ),
      setActiveSession,
      setActiveTask: vi.fn(),
      navigate: () => order.push("navigate"),
      onOpenChange: () => order.push("close"),
    });

    expect(setActiveSession).toHaveBeenCalledWith("task-1", "secondary");
    expect(order).toEqual(["session:secondary", "navigate", "close"]);
  });

  it("falls back safely when session loading fails", async () => {
    const order: string[] = [];
    await selectPendingTaskFromSheet({
      taskId: "task-1",
      preferredSessionId: "primary",
      taskPendingAction: "permission",
      loadTaskSessionsForTask: vi.fn(async () => {
        throw new Error("offline");
      }),
      setActiveSession: (_taskId, sessionId) => order.push(`session:${sessionId}`),
      setActiveTask: () => order.push("task"),
      navigate: () => order.push("navigate"),
      onOpenChange: () => order.push("close"),
    });
    expect(order).toEqual(["task", "navigate", "close"]);
  });
});

describe("selectTaskFromSheet races", () => {
  it("ignores an older pending selection that resolves after a newer tap", async () => {
    let resolveTaskA: (sessions: TaskSession[]) => void = () => undefined;
    let resolveTaskB: (sessions: TaskSession[]) => void = () => undefined;
    const loadTaskSessionsForTask = vi.fn(
      (taskId: string) =>
        new Promise<TaskSession[]>((resolve) => {
          if (taskId === "task-a") resolveTaskA = resolve;
          else resolveTaskB = resolve;
        }),
    );
    const setActiveSession = vi.fn();
    const navigate = vi.fn();
    const onOpenChange = vi.fn();
    const shared = {
      state: {
        lastSessionByTaskId: {},
        environmentIdBySessionId: {},
        taskSessionsById: {},
      },
      loadTaskSessionsForTask,
      setActiveSession,
      setActiveTask: vi.fn(),
      navigate,
      onOpenChange,
    };

    selectTaskFromSheet({
      ...shared,
      taskId: "task-a",
      task: { primarySessionId: "primary-a", taskPendingAction: "clarification" },
    });
    selectTaskFromSheet({
      ...shared,
      taskId: "task-b",
      task: { primarySessionId: "primary-b", taskPendingAction: "clarification" },
    });

    resolveTaskB([
      {
        id: "owner-b",
        task_id: "task-b",
        state: "WAITING_FOR_INPUT",
        pending_action: "clarification",
        started_at: UPDATED_AT,
        updated_at: UPDATED_AT,
      } as TaskSession,
    ]);
    await Promise.resolve();
    resolveTaskA([
      {
        id: "owner-a",
        task_id: "task-a",
        state: "WAITING_FOR_INPUT",
        pending_action: "clarification",
        started_at: UPDATED_AT,
        updated_at: UPDATED_AT,
      } as TaskSession,
    ]);
    await Promise.resolve();

    expect(setActiveSession).toHaveBeenCalledTimes(1);
    expect(setActiveSession).toHaveBeenCalledWith("task-b", "owner-b");
    expect(navigate).toHaveBeenCalledTimes(1);
    expect(navigate).toHaveBeenCalledWith("task-b");
    expect(onOpenChange).toHaveBeenCalledTimes(1);
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});

describe("selectTaskFromSheet sheet lifecycle", () => {
  it("invalidates a pending selection when the sheet closes and reopens", async () => {
    let resolveLoad: (sessions: TaskSession[]) => void = () => undefined;
    const loadTaskSessionsForTask = vi.fn(
      () =>
        new Promise<TaskSession[]>((resolve) => {
          resolveLoad = resolve;
        }),
    );
    const setActiveSession = vi.fn();
    const navigate = vi.fn();
    const onOpenChange = vi.fn();

    selectTaskFromSheet({
      taskId: "task-stale",
      task: { primarySessionId: "primary", taskPendingAction: "clarification" },
      state: {
        lastSessionByTaskId: {},
        environmentIdBySessionId: {},
        taskSessionsById: {},
      },
      loadTaskSessionsForTask,
      setActiveSession,
      setActiveTask: vi.fn(),
      navigate,
      onOpenChange,
    });
    handleTaskSheetOpenChange(false, onOpenChange);
    handleTaskSheetOpenChange(true, onOpenChange);

    resolveLoad([
      {
        id: "owner-stale",
        task_id: "task-stale",
        state: "WAITING_FOR_INPUT",
        pending_action: "clarification",
        started_at: UPDATED_AT,
        updated_at: UPDATED_AT,
      } as TaskSession,
    ]);
    await Promise.resolve();

    expect(setActiveSession).not.toHaveBeenCalled();
    expect(navigate).not.toHaveBeenCalled();
    expect(onOpenChange.mock.calls).toEqual([[false], [true]]);
  });
});
