import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { toSheetItem } from "./session-task-switcher-sheet-item";
import {
  createTaskSheetSelectionController,
  handleTaskSheetOpenChange,
  selectPendingTaskFromSheet,
  selectTaskFromSheet,
} from "./session-task-switcher-sheet-selection";
import type { TaskPendingAction, TaskSession } from "@/lib/types/http";
import { useSheetArchiveActions } from "./session-task-switcher-sheet-hooks";

type SheetTask = Parameters<typeof toSheetItem>[0];
type SheetCtx = Parameters<typeof toSheetItem>[1];
const UPDATED_AT = "2026-07-22T00:00:00Z";
const ERROR_PREVIEW = "Agent failed";

async function flushSelection(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  await new Promise((resolve) => setTimeout(resolve, 0));
}

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
});

describe("toSheetItem status", () => {
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

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.10
describe("toSheetItem automation indicators", () => {
  it("carries bounded automation indicators onto the mobile sheet row", () => {
    const item = toSheetItem(
      task({
        statusSummary: {
          revision: 1,
          updated_at: UPDATED_AT,
          pull_request: {
            number: 42,
            state: "open",
            auto_fix_enabled: true,
            auto_merge_enabled: true,
          },
        },
      }),
      emptyCtx(),
    );

    expect(item.prInfo).toEqual({
      number: 42,
      state: "Open",
      autoFixEnabled: true,
      autoMergeEnabled: true,
    });
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
    const loadTaskSessionsForTask = vi.fn(
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
    );
    const setActiveSession = vi.fn((_taskId: string, sessionId: string) => {
      order.push(`session:${sessionId}`);
    });
    await selectPendingTaskFromSheet({
      taskId: "task-1",
      preferredSessionId: "primary",
      taskPendingAction: "clarification",
      loadTaskSessionsForTask,
      setActiveSession,
      setActiveTask: vi.fn(),
      navigate: () => order.push("navigate"),
      onOpenChange: () => order.push("close"),
    });

    expect(setActiveSession).toHaveBeenCalledWith("task-1", "secondary");
    expect(loadTaskSessionsForTask).toHaveBeenCalledWith("task-1", { force: true });
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

  it("opens the task when the same action has a newer summary revision", async () => {
    const taskId = "task-revision";
    const setActiveSession = vi.fn();
    const setActiveTask = vi.fn();
    const navigate = vi.fn();
    const onOpenChange = vi.fn();
    await selectPendingTaskFromSheet({
      taskId,
      preferredSessionId: "primary",
      taskPendingAction: "clarification",
      pendingSnapshot: { revision: 1, pendingAction: "clarification" },
      getTaskPendingSnapshot: () => ({ revision: 2, pendingAction: "clarification" }),
      loadTaskSessionsForTask: vi.fn(async () => [
        {
          id: "owner",
          task_id: taskId,
          state: "WAITING_FOR_INPUT",
          pending_action: "clarification",
        } as TaskSession,
      ]),
      setActiveSession,
      setActiveTask,
      navigate,
      onOpenChange,
    });

    expect(setActiveSession).not.toHaveBeenCalled();
    expect(setActiveTask).toHaveBeenCalledWith(taskId);
    expect(navigate).toHaveBeenCalledWith(taskId);
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});

describe("selectPendingTaskFromSheet aborts", () => {
  it("leaves selection inert when a forced session load is superseded", async () => {
    const order: string[] = [];
    const abortError = new Error("superseded");
    abortError.name = "AbortError";

    await selectPendingTaskFromSheet({
      taskId: "task-1",
      preferredSessionId: "primary",
      taskPendingAction: "permission",
      loadTaskSessionsForTask: vi.fn(async () => {
        throw abortError;
      }),
      setActiveSession: (_taskId, sessionId) => order.push(`session:${sessionId}`),
      setActiveTask: () => order.push("task"),
      navigate: () => order.push("navigate"),
      onOpenChange: () => order.push("close"),
    });

    expect(order).toEqual([]);
  });
});

describe("selectTaskFromSheet races", () => {
  it("keeps simultaneous sheet selection controllers independent", () => {
    const first = createTaskSheetSelectionController();
    const second = createTaskSheetSelectionController();
    const firstToken = first.beginSelection();
    const secondToken = second.beginSelection();

    second.invalidate();

    expect(first.isCurrent(firstToken)).toBe(true);
    expect(second.isCurrent(secondToken)).toBe(false);
  });

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
      selectionController: createTaskSheetSelectionController(),
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
    await flushSelection();
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
    await flushSelection();

    expect(setActiveSession).toHaveBeenCalledTimes(1);
    expect(setActiveSession).toHaveBeenCalledWith("task-b", "owner-b");
    expect(navigate).toHaveBeenCalledTimes(1);
    expect(navigate).toHaveBeenCalledWith("task-b");
    expect(onOpenChange).toHaveBeenCalledTimes(1);
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});

describe("selectTaskFromSheet aborted loads", () => {
  it("leaves a sessionless selection inert when its session load is aborted", async () => {
    const abortError = new Error("task removed");
    abortError.name = "AbortError";
    const setActiveSession = vi.fn();
    const setActiveTask = vi.fn();
    const navigate = vi.fn();
    const onOpenChange = vi.fn();

    selectTaskFromSheet({
      selectionController: createTaskSheetSelectionController(),
      taskId: "task-aborted",
      task: { primarySessionId: null },
      state: {
        lastSessionByTaskId: {},
        environmentIdBySessionId: {},
        taskSessionsById: {},
      },
      loadTaskSessionsForTask: vi.fn(async () => {
        throw abortError;
      }),
      setActiveSession,
      setActiveTask,
      navigate,
      onOpenChange,
    });
    await flushSelection();

    expect(setActiveSession).not.toHaveBeenCalled();
    expect(setActiveTask).not.toHaveBeenCalled();
    expect(navigate).not.toHaveBeenCalled();
    expect(onOpenChange).not.toHaveBeenCalled();
  });
});

describe("selectTaskFromSheet summary races", () => {
  it("opens the task and closes the sheet when the pending action changes", async () => {
    const taskId = "task-summary-race";
    let resolveLoad: (sessions: TaskSession[]) => void = () => undefined;
    const loadTaskSessionsForTask = vi.fn(
      () =>
        new Promise<TaskSession[]>((resolve) => {
          resolveLoad = resolve;
        }),
    );
    let currentSnapshot: {
      revision: number;
      pendingAction: TaskPendingAction;
    } = { revision: 1, pendingAction: "clarification" };
    const setActiveSession = vi.fn();
    const setActiveTask = vi.fn();
    const navigate = vi.fn();
    const onOpenChange = vi.fn();

    selectTaskFromSheet({
      selectionController: createTaskSheetSelectionController(),
      taskId,
      task: {
        primarySessionId: "primary",
        statusSummary: {
          revision: 1,
          updated_at: UPDATED_AT,
          pending_action: "clarification",
        },
      },
      state: {
        lastSessionByTaskId: {},
        environmentIdBySessionId: {},
        taskSessionsById: {},
      },
      loadTaskSessionsForTask,
      setActiveSession,
      setActiveTask,
      navigate,
      onOpenChange,
      getTaskPendingSnapshot: () => currentSnapshot,
    });
    currentSnapshot = { revision: 2, pendingAction: "permission" };
    resolveLoad([
      {
        id: "old-owner",
        task_id: taskId,
        state: "WAITING_FOR_INPUT",
        pending_action: "clarification",
      } as TaskSession,
    ]);
    await flushSelection();

    expect(loadTaskSessionsForTask).toHaveBeenCalledWith(taskId, { force: true });
    expect(setActiveSession).not.toHaveBeenCalled();
    expect(setActiveTask).toHaveBeenCalledWith(taskId);
    expect(navigate).toHaveBeenCalledWith(taskId);
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});

describe("selectTaskFromSheet deleted-task race", () => {
  it("leaves the sheet open when the pending task projection disappears", async () => {
    const taskId = "task-deleted-race";
    let resolveLoad: (sessions: TaskSession[]) => void = () => undefined;
    const loadTaskSessionsForTask = vi.fn(
      () =>
        new Promise<TaskSession[]>((resolve) => {
          resolveLoad = resolve;
        }),
    );
    const setActiveSession = vi.fn();
    const setActiveTask = vi.fn();
    const navigate = vi.fn();
    const onOpenChange = vi.fn();

    selectTaskFromSheet({
      selectionController: createTaskSheetSelectionController(),
      taskId,
      task: {
        primarySessionId: "primary",
        statusSummary: {
          revision: 1,
          updated_at: UPDATED_AT,
          pending_action: "clarification",
        },
      },
      state: {
        lastSessionByTaskId: {},
        environmentIdBySessionId: {},
        taskSessionsById: {},
      },
      loadTaskSessionsForTask,
      setActiveSession,
      setActiveTask,
      navigate,
      onOpenChange,
      getTaskPendingSnapshot: () => undefined,
    });
    resolveLoad([
      {
        id: "deleted-owner",
        task_id: taskId,
        state: "WAITING_FOR_INPUT",
        pending_action: "clarification",
      } as TaskSession,
    ]);
    await flushSelection();

    expect(setActiveSession).not.toHaveBeenCalled();
    expect(setActiveTask).not.toHaveBeenCalled();
    expect(navigate).not.toHaveBeenCalled();
    expect(onOpenChange).not.toHaveBeenCalled();
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
    const selectionController = createTaskSheetSelectionController();

    selectTaskFromSheet({
      selectionController,
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
    handleTaskSheetOpenChange(selectionController, false, onOpenChange);
    handleTaskSheetOpenChange(selectionController, true, onOpenChange);

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
    await flushSelection();

    expect(setActiveSession).not.toHaveBeenCalled();
    expect(navigate).not.toHaveBeenCalled();
    expect(onOpenChange.mock.calls).toEqual([[false], [true]]);
  });
});

describe("useSheetArchiveActions", () => {
  it("tracks the directly archived row while its request is pending", async () => {
    let resolveArchive: () => void = () => undefined;
    const archiveAndSwitch = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          resolveArchive = resolve;
        }),
    );
    const store = {
      getState: () => ({}),
    } as Parameters<typeof useSheetArchiveActions>[0];
    const { result } = renderHook(() =>
      useSheetArchiveActions(
        store,
        archiveAndSwitch as Parameters<typeof useSheetArchiveActions>[1],
      ),
    );

    act(() => result.current.handleArchiveTask("task-1", { cascade: false }));

    await waitFor(() => expect(result.current.archivingTaskId).toBe("task-1"));
    expect(result.current.isArchiving).toBe(true);

    await act(async () => resolveArchive());

    await waitFor(() => expect(result.current.archivingTaskId).toBeNull());
    expect(result.current.isArchiving).toBe(false);
  });
});
