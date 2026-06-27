import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  prepareAndSwitchTask,
  buildSwitchToSession,
  selectTaskWithLayout,
} from "./task-select-helpers";

vi.mock("@/lib/services/session-launch-service", () => ({
  launchSession: vi.fn(),
}));
vi.mock("@/lib/services/session-launch-helpers", () => ({
  buildPrepareRequest: vi.fn(() => ({ request: { taskId: "task-new" } })),
}));
vi.mock("@/lib/state/dockview-store", () => ({
  performLayoutSwitch: vi.fn(),
  releaseLayoutToDefault: vi.fn(),
  useDockviewStore: { getState: () => ({ api: null, buildDefaultLayout: vi.fn() }) },
}));
vi.mock("@/lib/state/layout-manager", () => ({
  INTENT_PR_REVIEW: "pr-review",
}));
vi.mock("@/lib/links", () => ({
  replaceTaskUrl: vi.fn(),
}));
vi.mock("@/lib/api/domains/session-api", () => ({
  fetchTaskSession: vi.fn(async () => ({ session: null })),
}));

import { launchSession, type LaunchSessionResponse } from "@/lib/services/session-launch-service";
import { performLayoutSwitch, releaseLayoutToDefault } from "@/lib/state/dockview-store";
import { replaceTaskUrl } from "@/lib/links";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";
import { sessionId, taskId } from "@/lib/types/ids";

const NEW_TASK_ID = "task-new";
const OLD_SESSION_ID = "old-session";
const SELECT_TASK_ID = "task-A";
const PRIMARY_SESSION_ID = "sess-primary";
const OTHER_TASK_SESSION_ID = "sess-other-task";
const ENV_A = "env-A";
const ENV_B = "env-B";

function makeKanbanStore(args: {
  activeTaskId?: string | null;
  activeSessionId: string | null;
  envIds: Record<string, string>;
  lastSessionByTaskId?: Record<string, string>;
  sessionTaskIds?: Record<string, string>;
}): StoreApi<AppState> {
  const items: Record<string, Partial<TaskSession> & Pick<TaskSession, "id" | "task_id">> = {};
  for (const [sid, tid] of Object.entries(args.sessionTaskIds ?? {})) {
    items[sid] = {
      id: sessionId(sid),
      task_id: taskId(tid),
      task_environment_id: args.envIds[sid],
      is_passthrough: false,
    };
  }
  const state = {
    tasks: {
      activeTaskId: args.activeTaskId ?? null,
      activeSessionId: args.activeSessionId,
      lastSessionByTaskId: args.lastSessionByTaskId ?? {},
    },
    taskPRs: { byTaskId: {} as Record<string, unknown[]> },
    environmentIdBySessionId: args.envIds,
    taskSessions: { items },
    setTaskSession: vi.fn((session: TaskSession) => {
      items[session.id] = session;
    }),
  };
  return {
    getState: () => state as unknown as AppState,
    setState: vi.fn(),
    subscribe: vi.fn(),
  } as unknown as StoreApi<AppState>;
}

function runSelect(store: StoreApi<AppState>) {
  const switchToSession = vi.fn();
  selectTaskWithLayout({
    taskId: SELECT_TASK_ID,
    task: { primarySessionId: PRIMARY_SESSION_ID },
    store,
    switchToSession,
    loadTaskSessionsForTask: vi.fn(async () => []),
    setActiveTask: vi.fn(),
    setPreparingTaskId: vi.fn(),
  });
  return switchToSession;
}

function makeStore(activeSessionId: string | null): StoreApi<AppState> {
  const state = {
    tasks: { activeSessionId },
    taskPRs: { byTaskId: {} as Record<string, unknown[]> },
    environmentIdBySessionId: activeSessionId ? { [activeSessionId]: "env-old" } : {},
  };
  return {
    getState: () => state as unknown as AppState,
    setState: vi.fn(),
    subscribe: vi.fn(),
  } as unknown as StoreApi<AppState>;
}

function makeEnvStore(envIds: Record<string, string>): StoreApi<AppState> {
  return {
    getState: () => ({ environmentIdBySessionId: envIds }) as unknown as AppState,
  } as unknown as StoreApi<AppState>;
}

const ORIGINAL_TASK_ID = "task-original";
const PENDING_TASK_ID = "task-pending";
const ORIGINAL_SESSION_ID = "sess-original";
const PENDING_SESSION_ID = "sess-pending";
const ORIGINAL_ENV_ID = "env-original";

async function flushTaskSelection() {
  await Promise.resolve();
  await Promise.resolve();
  await new Promise((resolve) => setTimeout(resolve, 0));
}

function makeSelectionHarness(args: {
  activeTaskId: string;
  activeSessionId: string | null;
  envIds?: Record<string, string>;
  sessions?: Record<string, { id: string; task_id: string }>;
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
  };
  const snapshot = () =>
    ({
      ...state,
      tasks: { ...state.tasks },
    }) as unknown as AppState;
  const notify = (previousState: AppState) => {
    for (const listener of listeners) {
      listener(state as unknown as AppState, previousState);
    }
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
  const loadTaskSessionsForTask = vi.fn(
    () =>
      new Promise<TaskSession[]>((resolve) => {
        resolveLoad = resolve;
      }),
  );
  return {
    loadTaskSessionsForTask,
    resolveLoad: (sessions: TaskSession[]) => resolveLoad(sessions),
  };
}

describe("prepareAndSwitchTask — outgoing-env panel cleanup", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("releases the outgoing env's panels before awaiting launchSession", async () => {
    let resolveLaunch: (v: LaunchSessionResponse) => void = () => {};
    vi.mocked(launchSession).mockImplementation(
      () =>
        new Promise((res) => {
          resolveLaunch = res;
        }),
    );

    const store = makeStore(OLD_SESSION_ID);
    const switchToSession = vi.fn();
    const setPreparingTaskId = vi.fn();

    const promise = prepareAndSwitchTask(NEW_TASK_ID, store, switchToSession, setPreparingTaskId);

    expect(releaseLayoutToDefault).toHaveBeenCalledTimes(1);
    expect(switchToSession).not.toHaveBeenCalled();

    resolveLaunch({
      success: true,
      task_id: NEW_TASK_ID,
      session_id: "new-session",
      state: "ready",
    });
    const result = await promise;

    expect(result).toBe(true);
    expect(switchToSession).toHaveBeenCalledTimes(1);
    expect(switchToSession).toHaveBeenCalledWith(NEW_TASK_ID, "new-session", null);
    expect(setPreparingTaskId).toHaveBeenLastCalledWith(null);
  });

  it("returns false and does not call switchToSession when launchSession throws", async () => {
    vi.mocked(launchSession).mockRejectedValue(new Error("network"));
    const store = makeStore(OLD_SESSION_ID);
    const switchToSession = vi.fn();
    const setPreparingTaskId = vi.fn();

    const result = await prepareAndSwitchTask(
      NEW_TASK_ID,
      store,
      switchToSession,
      setPreparingTaskId,
    );

    expect(result).toBe(false);
    expect(releaseLayoutToDefault).toHaveBeenCalledTimes(1);
    expect(switchToSession).not.toHaveBeenCalled();
    expect(setPreparingTaskId).toHaveBeenLastCalledWith(null);
  });

  it("returns false and does not call switchToSession when session_id is absent", async () => {
    vi.mocked(launchSession).mockResolvedValue({} as never);
    const store = makeStore(OLD_SESSION_ID);
    const switchToSession = vi.fn();
    const setPreparingTaskId = vi.fn();

    const result = await prepareAndSwitchTask(
      NEW_TASK_ID,
      store,
      switchToSession,
      setPreparingTaskId,
    );

    expect(result).toBe(false);
    expect(releaseLayoutToDefault).toHaveBeenCalledTimes(1);
    expect(switchToSession).not.toHaveBeenCalled();
    expect(setPreparingTaskId).toHaveBeenLastCalledWith(null);
  });
});

describe("buildSwitchToSession", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("performs an env switch when the new session's environment is known", () => {
    const store = makeEnvStore({ "sess-old": "env-A", "sess-new": "env-B" });
    const setActiveSession = vi.fn();
    const switchToSession = buildSwitchToSession(store, setActiveSession);

    switchToSession("task-new", "sess-new", "sess-old");

    expect(setActiveSession).toHaveBeenCalledWith("task-new", "sess-new");
    expect(performLayoutSwitch).toHaveBeenCalledWith("env-A", "env-B", "sess-new");
    expect(releaseLayoutToDefault).not.toHaveBeenCalled();
  });

  it("releases the outgoing layout when the new env is not yet registered", () => {
    const store = makeEnvStore({ "sess-old": "env-A" });
    const setActiveSession = vi.fn();
    const switchToSession = buildSwitchToSession(store, setActiveSession);

    switchToSession("task-new", "sess-new", "sess-old");

    expect(setActiveSession).toHaveBeenCalledWith("task-new", "sess-new");
    expect(performLayoutSwitch).not.toHaveBeenCalled();
    expect(releaseLayoutToDefault).toHaveBeenCalledWith("env-A");
  });

  it("is a no-op for layout switching when the same session is reselected", () => {
    const store = makeEnvStore({});
    const setActiveSession = vi.fn();
    const switchToSession = buildSwitchToSession(store, setActiveSession);

    switchToSession("task-new", "sess-x", "sess-x");

    expect(setActiveSession).toHaveBeenCalledWith("task-new", "sess-x");
    expect(performLayoutSwitch).not.toHaveBeenCalled();
    expect(releaseLayoutToDefault).not.toHaveBeenCalled();
  });
});

/**
 * Regression for "switching tasks loses the user's last-selected session":
 *
 *   1. Task A has sessions [primary, gpt]; user clicks the gpt tab.
 *   2. User clicks Task B in the sidebar.
 *   3. User clicks Task A in the sidebar — expected the gpt tab still active.
 *
 * Before the fix, `selectTaskWithLayout` always switched to `primarySessionId`,
 * so step 3 set activeSessionId back to "primary". The dockview slow-path then
 * closed the gpt panel (it didn't match activeSessionId), and the surviving
 * sibling tab (Diff) auto-promoted to active.
 *
 * The fix tracks the user's last-selected session per task in
 * `tasks.lastSessionByTaskId` and prefers it over `primarySessionId` on
 * re-entry, as long as the session still has a known environment.
 */
describe("selectTaskWithLayout — last-selected session preference", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("prefers the user's last-selected session over primarySessionId on re-entry", () => {
    const LAST = "sess-gpt";
    const switchToSession = runSelect(
      makeKanbanStore({
        activeSessionId: OTHER_TASK_SESSION_ID,
        envIds: { [OTHER_TASK_SESSION_ID]: ENV_B, [PRIMARY_SESSION_ID]: ENV_A, [LAST]: ENV_A },
        lastSessionByTaskId: { [SELECT_TASK_ID]: LAST },
        sessionTaskIds: { [PRIMARY_SESSION_ID]: SELECT_TASK_ID, [LAST]: SELECT_TASK_ID },
      }),
    );

    expect(switchToSession).toHaveBeenCalledWith(SELECT_TASK_ID, LAST, OTHER_TASK_SESSION_ID);
  });

  it("falls back to primarySessionId when the remembered session has no env mapping", () => {
    const switchToSession = runSelect(
      makeKanbanStore({
        activeSessionId: null,
        envIds: { [PRIMARY_SESSION_ID]: ENV_A },
        lastSessionByTaskId: { [SELECT_TASK_ID]: "sess-stale" },
        sessionTaskIds: { [PRIMARY_SESSION_ID]: SELECT_TASK_ID },
      }),
    );

    expect(switchToSession).toHaveBeenCalledWith(SELECT_TASK_ID, PRIMARY_SESSION_ID, null);
  });

  it("uses primarySessionId when no last-selected session is recorded for the task", () => {
    const switchToSession = runSelect(
      makeKanbanStore({
        activeSessionId: null,
        envIds: { [PRIMARY_SESSION_ID]: ENV_A },
        lastSessionByTaskId: {},
        sessionTaskIds: { [PRIMARY_SESSION_ID]: SELECT_TASK_ID },
      }),
    );

    expect(switchToSession).toHaveBeenCalledWith(SELECT_TASK_ID, PRIMARY_SESSION_ID, null);
  });

  /**
   * Regression for a layout-leak observed when creating a new task: the
   * dockview tab-sync listener can fire `setActiveSession(newTaskId, oldSid)`
   * during a task switch (stale panel still live), which writes
   * `lastSessionByTaskId[newTaskId] = oldSid` even though `oldSid` belongs to
   * a different task. Without this guard, re-entering the new task would
   * resolve to that cross-task session, restoring the previous task's
   * env-scoped panels (files/changes) instead of the new task's primary.
   */
  it("falls back to primarySessionId when the remembered session belongs to a different task", () => {
    const POISONED = "sess-belongs-to-task-B";
    const switchToSession = runSelect(
      makeKanbanStore({
        activeSessionId: null,
        envIds: { [PRIMARY_SESSION_ID]: ENV_A, [POISONED]: ENV_B },
        lastSessionByTaskId: { [SELECT_TASK_ID]: POISONED },
        sessionTaskIds: { [POISONED]: "task-B", [PRIMARY_SESSION_ID]: SELECT_TASK_ID },
      }),
    );

    expect(switchToSession).toHaveBeenCalledWith(SELECT_TASK_ID, PRIMARY_SESSION_ID, null);
  });
});

describe("selectTaskWithLayout — pending selection races", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("does not switch back to a sessionless task when its prepare failure resolves after another task was selected", async () => {
    const SESSIONLESS = "task-sessionless";
    const OTHER = "task-other";
    const { state, store, setActiveTask } = makeSelectionHarness({
      activeTaskId: SESSIONLESS,
      activeSessionId: null,
      envIds: { "sess-other": "env-other" },
      sessions: { "sess-other": { id: "sess-other", task_id: OTHER } },
    });
    const { loadTaskSessionsForTask, resolveLoad } = makeDeferredSessionLoader();

    selectTaskWithLayout({
      taskId: SESSIONLESS,
      task: undefined,
      store,
      switchToSession: vi.fn(),
      loadTaskSessionsForTask,
      setActiveTask,
      setPreparingTaskId: vi.fn(),
    });

    state.tasks.activeTaskId = OTHER;
    state.tasks.activeSessionId = "sess-other";
    selectTaskWithLayout({
      taskId: OTHER,
      task: { primarySessionId: "sess-other" },
      store,
      switchToSession: vi.fn(),
      loadTaskSessionsForTask: vi.fn(async () => []),
      setActiveTask,
      setPreparingTaskId: vi.fn(),
    });
    resolveLoad([]);
    await flushTaskSelection();

    expect(setActiveTask).not.toHaveBeenCalledWith(SESSIONLESS);
    expect(releaseLayoutToDefault).not.toHaveBeenCalled();
    expect(replaceTaskUrl).not.toHaveBeenCalledWith(SESSIONLESS);
  });

  it("does not switch to an old pending selection after the user clicks back to the original task", async () => {
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

describe("selectTaskWithLayout — external active-task changes", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("does not apply a pending selection after an external active-task change", async () => {
    const OTHER = "task-other";
    const switchToSession = vi.fn();
    const { state, store, setActiveTask } = makeSelectionHarness({
      activeTaskId: ORIGINAL_TASK_ID,
      activeSessionId: ORIGINAL_SESSION_ID,
      envIds: { [ORIGINAL_SESSION_ID]: ORIGINAL_ENV_ID },
      sessions: {
        [PENDING_SESSION_ID]: { id: PENDING_SESSION_ID, task_id: PENDING_TASK_ID },
        [ORIGINAL_SESSION_ID]: { id: ORIGINAL_SESSION_ID, task_id: ORIGINAL_TASK_ID },
      },
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

    state.tasks.activeTaskId = OTHER;
    state.tasks.activeSessionId = null;
    resolveLoad([
      { id: PENDING_SESSION_ID, task_id: PENDING_TASK_ID, is_primary: true } as TaskSession,
    ]);
    await flushTaskSelection();

    expect(switchToSession).not.toHaveBeenCalled();
    expect(replaceTaskUrl).not.toHaveBeenCalledWith(PENDING_TASK_ID);
  });

  it("does not apply a pending selection after an external task switch returns to the original task", async () => {
    const OTHER = "task-other";
    const switchToSession = vi.fn();
    const { store, setActiveTask } = makeSelectionHarness({
      activeTaskId: ORIGINAL_TASK_ID,
      activeSessionId: ORIGINAL_SESSION_ID,
      envIds: { [ORIGINAL_SESSION_ID]: ORIGINAL_ENV_ID },
      sessions: {
        [PENDING_SESSION_ID]: { id: PENDING_SESSION_ID, task_id: PENDING_TASK_ID },
        [ORIGINAL_SESSION_ID]: { id: ORIGINAL_SESSION_ID, task_id: ORIGINAL_TASK_ID },
      },
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

    setActiveTask(OTHER);
    setActiveTask(ORIGINAL_TASK_ID);
    resolveLoad([
      { id: PENDING_SESSION_ID, task_id: PENDING_TASK_ID, is_primary: true } as TaskSession,
    ]);
    await flushTaskSelection();

    expect(switchToSession).not.toHaveBeenCalled();
    expect(replaceTaskUrl).not.toHaveBeenCalledWith(PENDING_TASK_ID);
  });
});

describe("selectTaskWithLayout — selection guard cleanup", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("disposes the selection guard when loading a primary task's sessions rejects", async () => {
    const { store, setActiveTask, getListenerCount } = makeSelectionHarness({
      activeTaskId: ORIGINAL_TASK_ID,
      activeSessionId: ORIGINAL_SESSION_ID,
      envIds: { [ORIGINAL_SESSION_ID]: ORIGINAL_ENV_ID },
    });

    selectTaskWithLayout({
      taskId: PENDING_TASK_ID,
      task: { primarySessionId: PENDING_SESSION_ID },
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

  it("disposes the selection guard when loading a sessionless task rejects", async () => {
    const { store, setActiveTask, getListenerCount } = makeSelectionHarness({
      activeTaskId: ORIGINAL_TASK_ID,
      activeSessionId: ORIGINAL_SESSION_ID,
      envIds: { [ORIGINAL_SESSION_ID]: ORIGINAL_ENV_ID },
    });

    selectTaskWithLayout({
      taskId: PENDING_TASK_ID,
      task: undefined,
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
});

describe("selectTaskWithLayout — pending old-session changes", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("keeps a pending selection alive when only the old task's session id changes", async () => {
    const OLD = "task-old";
    const SESSIONLESS = "task-sessionless";
    vi.mocked(launchSession).mockResolvedValue({} as never);
    const { state, store, setActiveTask } = makeSelectionHarness({
      activeTaskId: OLD,
      activeSessionId: "sess-old",
    });
    const { loadTaskSessionsForTask, resolveLoad } = makeDeferredSessionLoader();

    selectTaskWithLayout({
      taskId: SESSIONLESS,
      task: undefined,
      store,
      switchToSession: vi.fn(),
      loadTaskSessionsForTask,
      setActiveTask,
      setPreparingTaskId: vi.fn(),
    });

    state.tasks.activeSessionId = "sess-old-replaced";
    resolveLoad([]);
    await flushTaskSelection();

    expect(setActiveTask).toHaveBeenCalledWith(SESSIONLESS);
    expect(replaceTaskUrl).toHaveBeenCalledWith(SESSIONLESS);
  });
});
