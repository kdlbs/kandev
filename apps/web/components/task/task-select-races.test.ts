import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { TaskPendingAction, TaskSession } from "@/lib/types/http";
import { launchSession } from "@/lib/services/session-launch-service";
import { releaseLayoutToDefault } from "@/lib/state/dockview-store";
import { replaceTaskUrl } from "@/lib/links";
import { selectTaskWithLayout } from "./task-select-helpers";

vi.mock("@/lib/services/session-launch-service", () => ({ launchSession: vi.fn() }));
vi.mock("@/lib/services/session-launch-helpers", () => ({
  buildPrepareRequest: vi.fn(() => ({ request: { taskId: "task-new" } })),
}));
vi.mock("@/lib/state/dockview-store", () => ({
  performLayoutSwitch: vi.fn(),
  releaseLayoutToDefault: vi.fn(),
}));
vi.mock("@/lib/links", () => ({ replaceTaskUrl: vi.fn() }));

const ORIGINAL_TASK_ID = "task-original";
const PENDING_TASK_ID = "task-pending";
const ORIGINAL_SESSION_ID = "sess-original";
const PENDING_SESSION_ID = "sess-pending";
const ORIGINAL_ENV_ID = "env-original";
const SUMMARY_UPDATED_AT = "2026-08-15T15:00:00Z";
const NEWER_SUMMARY_UPDATED_AT = "2026-08-15T15:00:01Z";

function makeSelectionHarness(args: {
  activeTaskId: string;
  activeSessionId: string | null;
  envIds?: Record<string, string>;
  sessions?: Record<string, { id: string; task_id: string }>;
  taskRows?: Array<{
    id: string;
    primarySessionId?: string | null;
    taskPendingAction?: TaskPendingAction | null;
    statusSummary?: {
      revision: number;
      updated_at: string;
      pending_action?: TaskPendingAction | null;
    } | null;
  }>;
}) {
  const listeners: Array<(state: AppState, previousState: AppState) => void> = [];
  const state = {
    tasks: {
      activeTaskId: args.activeTaskId,
      activeSessionId: args.activeSessionId,
      lastSessionByTaskId: {},
    },
    taskPRs: { byTaskId: {} as Record<string, unknown[]> },
    environmentIdBySessionId: args.envIds ?? {},
    taskSessions: { items: (args.sessions ?? {}) as Record<string, TaskSession> },
    kanban: { tasks: args.taskRows ?? [] },
    kanbanMulti: { snapshots: {} },
  };
  const snapshot = () => ({ ...state, tasks: { ...state.tasks } }) as unknown as AppState;
  const notify = (previousState: AppState) => {
    for (const listener of listeners) listener(state as unknown as AppState, previousState);
  };
  const store = {
    getState: () => state as unknown as AppState,
    setState: vi.fn(),
    subscribe: vi.fn((listener: (state: AppState, previousState: AppState) => void) => {
      listeners.push(listener);
      return () => {
        const index = listeners.indexOf(listener);
        if (index >= 0) listeners.splice(index, 1);
      };
    }),
  } as unknown as StoreApi<AppState>;
  const setActiveTask = vi.fn((taskId: string) => {
    const previousState = snapshot();
    state.tasks.activeTaskId = taskId;
    state.tasks.activeSessionId = null;
    notify(previousState);
  });
  return { state, store, setActiveTask, getListenerCount: () => listeners.length };
}

function makeDeferredSessionLoader() {
  let resolveLoad: (sessions: TaskSession[]) => void = () => {};
  let rejectLoad: (error: Error) => void = () => {};
  const loadTaskSessionsForTask = vi.fn(
    () =>
      new Promise<TaskSession[]>((resolve, reject) => {
        resolveLoad = resolve;
        rejectLoad = reject;
      }),
  );
  return {
    loadTaskSessionsForTask,
    resolveLoad: (sessions: TaskSession[]) => resolveLoad(sessions),
    rejectLoad: (error: Error) => rejectLoad(error),
  };
}

async function flushTaskSelection() {
  await Promise.resolve();
  await Promise.resolve();
  await new Promise((resolve) => setTimeout(resolve, 0));
}

describe("selectTaskWithLayout pending selection races", () => {
  beforeEach(() => vi.clearAllMocks());

  it("ignores a sessionless selection after another task is selected", async () => {
    const sessionlessTaskId = "task-sessionless";
    const otherTaskId = "task-other";
    const { state, store, setActiveTask } = makeSelectionHarness({
      activeTaskId: sessionlessTaskId,
      activeSessionId: null,
      envIds: { "sess-other": "env-other" },
      sessions: { "sess-other": { id: "sess-other", task_id: otherTaskId } },
    });
    const { loadTaskSessionsForTask, resolveLoad } = makeDeferredSessionLoader();

    selectTaskWithLayout({
      taskId: sessionlessTaskId,
      task: undefined,
      store,
      switchToSession: vi.fn(),
      loadTaskSessionsForTask,
      setActiveTask,
      setPreparingTaskId: vi.fn(),
    });
    state.tasks.activeTaskId = otherTaskId;
    state.tasks.activeSessionId = "sess-other";
    selectTaskWithLayout({
      taskId: otherTaskId,
      task: { primarySessionId: "sess-other" },
      store,
      switchToSession: vi.fn(),
      loadTaskSessionsForTask: vi.fn(async () => []),
      setActiveTask,
      setPreparingTaskId: vi.fn(),
    });
    resolveLoad([]);
    await flushTaskSelection();

    expect(setActiveTask).not.toHaveBeenCalledWith(sessionlessTaskId);
    expect(releaseLayoutToDefault).not.toHaveBeenCalled();
    expect(replaceTaskUrl).not.toHaveBeenCalledWith(sessionlessTaskId);
  });

  it("ignores an old pending selection after returning to the original task", async () => {
    const { store, setActiveTask } = makeSelectionHarness({
      activeTaskId: ORIGINAL_TASK_ID,
      activeSessionId: ORIGINAL_SESSION_ID,
      envIds: { [ORIGINAL_SESSION_ID]: ORIGINAL_ENV_ID },
      sessions: {
        [ORIGINAL_SESSION_ID]: { id: ORIGINAL_SESSION_ID, task_id: ORIGINAL_TASK_ID },
      },
    });
    const { loadTaskSessionsForTask, resolveLoad } = makeDeferredSessionLoader();

    selectTaskWithLayout({
      taskId: PENDING_TASK_ID,
      task: undefined,
      store,
      switchToSession: vi.fn(),
      loadTaskSessionsForTask,
      setActiveTask,
      setPreparingTaskId: vi.fn(),
    });
    selectTaskWithLayout({
      taskId: ORIGINAL_TASK_ID,
      task: { primarySessionId: ORIGINAL_SESSION_ID },
      store,
      switchToSession: vi.fn(),
      loadTaskSessionsForTask: vi.fn(async () => []),
      setActiveTask,
      setPreparingTaskId: vi.fn(),
    });
    resolveLoad([]);
    await flushTaskSelection();

    expect(setActiveTask).not.toHaveBeenCalledWith(PENDING_TASK_ID);
    expect(replaceTaskUrl).not.toHaveBeenCalledWith(PENDING_TASK_ID);
  });
});

describe("selectTaskWithLayout pending summary races", () => {
  beforeEach(() => vi.clearAllMocks());

  it("falls back to the task route when the pending action changes", async () => {
    const initialTask = {
      id: PENDING_TASK_ID,
      primarySessionId: PENDING_SESSION_ID,
      statusSummary: {
        revision: 1,
        updated_at: SUMMARY_UPDATED_AT,
        pending_action: "clarification" as const,
      },
    };
    const { state, store, setActiveTask } = makeSelectionHarness({
      activeTaskId: ORIGINAL_TASK_ID,
      activeSessionId: ORIGINAL_SESSION_ID,
      taskRows: [initialTask],
    });
    const switchToSession = vi.fn();
    const { loadTaskSessionsForTask, resolveLoad } = makeDeferredSessionLoader();

    selectTaskWithLayout({
      taskId: PENDING_TASK_ID,
      task: initialTask,
      store,
      switchToSession,
      loadTaskSessionsForTask,
      setActiveTask,
      setPreparingTaskId: vi.fn(),
    });
    state.kanban.tasks[0] = {
      ...initialTask,
      statusSummary: {
        revision: 2,
        updated_at: NEWER_SUMMARY_UPDATED_AT,
        pending_action: "permission",
      },
    };
    resolveLoad([
      {
        id: PENDING_SESSION_ID,
        task_id: PENDING_TASK_ID,
        state: "WAITING_FOR_INPUT",
        pending_action: "clarification",
      } as TaskSession,
    ]);
    await vi.waitFor(() => expect(setActiveTask).toHaveBeenCalledWith(PENDING_TASK_ID));
    expect(loadTaskSessionsForTask).toHaveBeenCalledWith(PENDING_TASK_ID, { force: true });
    expect(switchToSession).not.toHaveBeenCalled();
    expect(replaceTaskUrl).toHaveBeenCalledWith(PENDING_TASK_ID);
  });
});

describe("selectTaskWithLayout same-action summary race", () => {
  beforeEach(() => vi.clearAllMocks());

  it("falls back to the task route when a same-action revision can have a new owner", async () => {
    const initialTask = {
      id: PENDING_TASK_ID,
      primarySessionId: PENDING_SESSION_ID,
      statusSummary: {
        revision: 1,
        updated_at: SUMMARY_UPDATED_AT,
        pending_action: "clarification" as const,
      },
    };
    const { state, store, setActiveTask } = makeSelectionHarness({
      activeTaskId: ORIGINAL_TASK_ID,
      activeSessionId: ORIGINAL_SESSION_ID,
      taskRows: [initialTask],
    });
    const switchToSession = vi.fn();
    const { loadTaskSessionsForTask, resolveLoad } = makeDeferredSessionLoader();

    selectTaskWithLayout({
      taskId: PENDING_TASK_ID,
      task: initialTask,
      store,
      switchToSession,
      loadTaskSessionsForTask,
      setActiveTask,
      setPreparingTaskId: vi.fn(),
    });
    state.kanban.tasks[0] = {
      ...initialTask,
      statusSummary: {
        revision: 2,
        updated_at: NEWER_SUMMARY_UPDATED_AT,
        pending_action: "clarification",
      },
    };
    resolveLoad([
      {
        id: PENDING_SESSION_ID,
        task_id: PENDING_TASK_ID,
        state: "WAITING_FOR_INPUT",
        pending_action: "clarification",
      } as TaskSession,
    ]);
    await vi.waitFor(() => expect(setActiveTask).toHaveBeenCalledWith(PENDING_TASK_ID));
    expect(switchToSession).not.toHaveBeenCalled();
    expect(replaceTaskUrl).toHaveBeenCalledWith(PENDING_TASK_ID);
  });
});

describe("selectTaskWithLayout deleted-task race", () => {
  beforeEach(() => vi.clearAllMocks());

  it("leaves selection inert when the pending task projection disappears", async () => {
    const initialTask = {
      id: PENDING_TASK_ID,
      primarySessionId: PENDING_SESSION_ID,
      statusSummary: {
        revision: 1,
        updated_at: SUMMARY_UPDATED_AT,
        pending_action: "clarification" as const,
      },
    };
    const { state, store, setActiveTask } = makeSelectionHarness({
      activeTaskId: ORIGINAL_TASK_ID,
      activeSessionId: ORIGINAL_SESSION_ID,
      taskRows: [initialTask],
    });
    const switchToSession = vi.fn();
    const { loadTaskSessionsForTask, resolveLoad } = makeDeferredSessionLoader();

    selectTaskWithLayout({
      taskId: PENDING_TASK_ID,
      task: initialTask,
      store,
      switchToSession,
      loadTaskSessionsForTask,
      setActiveTask,
      setPreparingTaskId: vi.fn(),
    });
    state.kanban.tasks = [];
    resolveLoad([
      {
        id: PENDING_SESSION_ID,
        task_id: PENDING_TASK_ID,
        state: "WAITING_FOR_INPUT",
        pending_action: "clarification",
      } as TaskSession,
    ]);
    await flushTaskSelection();

    expect(switchToSession).not.toHaveBeenCalled();
    expect(setActiveTask).not.toHaveBeenCalled();
    expect(replaceTaskUrl).not.toHaveBeenCalledWith(PENDING_TASK_ID);
  });
});

describe("selectTaskWithLayout rejected pending-load races", () => {
  beforeEach(() => vi.clearAllMocks());

  for (const primarySessionId of [PENDING_SESSION_ID, null]) {
    it(`falls back after a rejected ${primarySessionId ? "primary" : "sessionless"} load when the summary changes`, async () => {
      const initialTask = {
        id: PENDING_TASK_ID,
        primarySessionId,
        statusSummary: {
          revision: 1,
          updated_at: SUMMARY_UPDATED_AT,
          pending_action: "clarification" as const,
        },
      };
      const { state, store, setActiveTask } = makeSelectionHarness({
        activeTaskId: ORIGINAL_TASK_ID,
        activeSessionId: ORIGINAL_SESSION_ID,
        taskRows: [initialTask],
      });
      const switchToSession = vi.fn();
      const { loadTaskSessionsForTask, rejectLoad } = makeDeferredSessionLoader();

      selectTaskWithLayout({
        taskId: PENDING_TASK_ID,
        task: initialTask,
        store,
        switchToSession,
        loadTaskSessionsForTask,
        setActiveTask,
        setPreparingTaskId: vi.fn(),
      });
      state.kanban.tasks[0] = {
        ...initialTask,
        statusSummary: {
          revision: 2,
          updated_at: NEWER_SUMMARY_UPDATED_AT,
          pending_action: "permission",
        },
      };
      rejectLoad(new Error("load failed"));
      await vi.waitFor(() => expect(setActiveTask).toHaveBeenCalledWith(PENDING_TASK_ID));
      expect(switchToSession).not.toHaveBeenCalled();
      expect(replaceTaskUrl).toHaveBeenCalledWith(PENDING_TASK_ID);
    });
  }
});

describe("selectTaskWithLayout external active-task changes", () => {
  beforeEach(() => vi.clearAllMocks());

  function pendingHarness() {
    return makeSelectionHarness({
      activeTaskId: ORIGINAL_TASK_ID,
      activeSessionId: ORIGINAL_SESSION_ID,
      envIds: { [ORIGINAL_SESSION_ID]: ORIGINAL_ENV_ID },
      sessions: {
        [PENDING_SESSION_ID]: { id: PENDING_SESSION_ID, task_id: PENDING_TASK_ID },
        [ORIGINAL_SESSION_ID]: { id: ORIGINAL_SESSION_ID, task_id: ORIGINAL_TASK_ID },
      },
    });
  }

  it("ignores a pending selection after an external active-task change", async () => {
    const switchToSession = vi.fn();
    const { state, store, setActiveTask } = pendingHarness();
    const { loadTaskSessionsForTask, resolveLoad } = makeDeferredSessionLoader();
    selectTaskWithLayout({
      taskId: PENDING_TASK_ID,
      task: { primarySessionId: PENDING_SESSION_ID },
      store,
      switchToSession,
      loadTaskSessionsForTask,
      setActiveTask,
      setPreparingTaskId: vi.fn(),
    });

    state.tasks.activeTaskId = "task-other";
    state.tasks.activeSessionId = null;
    resolveLoad([
      { id: PENDING_SESSION_ID, task_id: PENDING_TASK_ID, is_primary: true } as TaskSession,
    ]);
    await flushTaskSelection();

    expect(switchToSession).not.toHaveBeenCalled();
    expect(replaceTaskUrl).not.toHaveBeenCalledWith(PENDING_TASK_ID);
  });

  it("remembers an external switch even when it returns to the original task", async () => {
    const switchToSession = vi.fn();
    const { store, setActiveTask } = pendingHarness();
    const { loadTaskSessionsForTask, resolveLoad } = makeDeferredSessionLoader();
    selectTaskWithLayout({
      taskId: PENDING_TASK_ID,
      task: { primarySessionId: PENDING_SESSION_ID },
      store,
      switchToSession,
      loadTaskSessionsForTask,
      setActiveTask,
      setPreparingTaskId: vi.fn(),
    });

    setActiveTask("task-other");
    setActiveTask(ORIGINAL_TASK_ID);
    resolveLoad([
      { id: PENDING_SESSION_ID, task_id: PENDING_TASK_ID, is_primary: true } as TaskSession,
    ]);
    await flushTaskSelection();

    expect(switchToSession).not.toHaveBeenCalled();
    expect(replaceTaskUrl).not.toHaveBeenCalledWith(PENDING_TASK_ID);
  });

  it("ignores an off-route pending selection after unrelated navigation", async () => {
    const switchToSession = vi.fn();
    const navigateToTask = vi.fn();
    const selectionController = new AbortController();
    const { store, setActiveTask } = pendingHarness();
    const { loadTaskSessionsForTask, resolveLoad } = makeDeferredSessionLoader();
    selectTaskWithLayout({
      taskId: PENDING_TASK_ID,
      task: {
        primarySessionId: PENDING_SESSION_ID,
        taskPendingAction: "clarification",
      },
      store,
      switchToSession,
      loadTaskSessionsForTask,
      setActiveTask,
      setPreparingTaskId: vi.fn(),
      navigateToTask,
      selectionSignal: selectionController.signal,
    });

    expect(loadTaskSessionsForTask).toHaveBeenCalledWith(PENDING_TASK_ID, {
      force: true,
      signal: selectionController.signal,
    });

    selectionController.abort();
    resolveLoad([
      {
        id: PENDING_SESSION_ID,
        task_id: PENDING_TASK_ID,
        state: "WAITING_FOR_INPUT",
        pending_action: "clarification",
      } as TaskSession,
    ]);
    await flushTaskSelection();

    expect(switchToSession).not.toHaveBeenCalled();
    expect(setActiveTask).not.toHaveBeenCalledWith(PENDING_TASK_ID);
    expect(navigateToTask).not.toHaveBeenCalled();
  });
});

describe("selectTaskWithLayout selection guard cleanup", () => {
  beforeEach(() => vi.clearAllMocks());

  for (const task of [
    { label: "primary", value: { primarySessionId: PENDING_SESSION_ID } },
    { label: "sessionless", value: undefined },
  ]) {
    it(`disposes the guard when loading a ${task.label} task rejects`, async () => {
      const { store, setActiveTask, getListenerCount } = makeSelectionHarness({
        activeTaskId: ORIGINAL_TASK_ID,
        activeSessionId: ORIGINAL_SESSION_ID,
        envIds: { [ORIGINAL_SESSION_ID]: ORIGINAL_ENV_ID },
      });
      selectTaskWithLayout({
        taskId: PENDING_TASK_ID,
        task: task.value,
        store,
        switchToSession: vi.fn(),
        loadTaskSessionsForTask: vi.fn(async () => {
          throw new Error("load failed");
        }),
        setActiveTask,
        setPreparingTaskId: vi.fn(),
      });

      expect(getListenerCount()).toBe(1);
      await flushTaskSelection();
      expect(getListenerCount()).toBe(0);
    });
  }
});

describe("selectTaskWithLayout old-session changes", () => {
  beforeEach(() => vi.clearAllMocks());

  it("keeps a pending selection alive when only the old session id changes", async () => {
    vi.mocked(launchSession).mockResolvedValue({} as never);
    const sessionlessTaskId = "task-sessionless";
    const { state, store, setActiveTask } = makeSelectionHarness({
      activeTaskId: "task-old",
      activeSessionId: "sess-old",
    });
    const { loadTaskSessionsForTask, resolveLoad } = makeDeferredSessionLoader();
    selectTaskWithLayout({
      taskId: sessionlessTaskId,
      task: undefined,
      store,
      switchToSession: vi.fn(),
      loadTaskSessionsForTask,
      setActiveTask,
      setPreparingTaskId: vi.fn(),
    });

    state.tasks.activeSessionId = "sess-old-replaced";
    resolveLoad([]);
    await vi.waitFor(() => expect(setActiveTask).toHaveBeenCalledWith(sessionlessTaskId));
    expect(replaceTaskUrl).toHaveBeenCalledWith(sessionlessTaskId);
  });

  it("uses the current old session after a primary-session load resolves", async () => {
    const replacementSessionId = "sess-old-replaced";
    const switchToSession = vi.fn();
    const { state, store, setActiveTask } = makeSelectionHarness({
      activeTaskId: ORIGINAL_TASK_ID,
      activeSessionId: ORIGINAL_SESSION_ID,
    });
    const { loadTaskSessionsForTask, resolveLoad } = makeDeferredSessionLoader();
    selectTaskWithLayout({
      taskId: PENDING_TASK_ID,
      task: { primarySessionId: PENDING_SESSION_ID },
      store,
      switchToSession,
      loadTaskSessionsForTask,
      setActiveTask,
      setPreparingTaskId: vi.fn(),
    });

    state.tasks.activeSessionId = replacementSessionId;
    resolveLoad([{ id: PENDING_SESSION_ID, task_id: PENDING_TASK_ID } as TaskSession]);
    await vi.waitFor(() =>
      expect(switchToSession).toHaveBeenCalledWith(
        PENDING_TASK_ID,
        PENDING_SESSION_ID,
        replacementSessionId,
      ),
    );
  });

  it("uses the current old session after a primary-session load rejects", async () => {
    const replacementSessionId = "sess-old-replaced";
    const switchToSession = vi.fn();
    const { state, store, setActiveTask } = makeSelectionHarness({
      activeTaskId: ORIGINAL_TASK_ID,
      activeSessionId: ORIGINAL_SESSION_ID,
    });
    const { loadTaskSessionsForTask, rejectLoad } = makeDeferredSessionLoader();
    selectTaskWithLayout({
      taskId: PENDING_TASK_ID,
      task: { primarySessionId: PENDING_SESSION_ID },
      store,
      switchToSession,
      loadTaskSessionsForTask,
      setActiveTask,
      setPreparingTaskId: vi.fn(),
    });

    state.tasks.activeSessionId = replacementSessionId;
    rejectLoad(new Error("load failed"));
    await vi.waitFor(() =>
      expect(switchToSession).toHaveBeenCalledWith(
        PENDING_TASK_ID,
        PENDING_SESSION_ID,
        replacementSessionId,
      ),
    );
  });
});
