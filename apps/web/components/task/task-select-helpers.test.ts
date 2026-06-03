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

import { launchSession, type LaunchSessionResponse } from "@/lib/services/session-launch-service";
import { performLayoutSwitch, releaseLayoutToDefault } from "@/lib/state/dockview-store";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";

const NEW_TASK_ID = "task-new";
const OLD_SESSION_ID = "old-session";

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
  const TASK_ID = "task-A";
  const PRIMARY = "sess-primary";

  beforeEach(() => {
    vi.clearAllMocks();
  });

  function makeKanbanStore(args: {
    activeSessionId: string | null;
    envIds: Record<string, string>;
    lastSessionByTaskId?: Record<string, string>;
    sessionTaskIds?: Record<string, string>;
  }): StoreApi<AppState> {
    const items: Record<string, { id: string; task_id: string }> = {};
    for (const [sid, tid] of Object.entries(args.sessionTaskIds ?? {})) {
      items[sid] = { id: sid, task_id: tid };
    }
    const state = {
      tasks: {
        activeSessionId: args.activeSessionId,
        lastSessionByTaskId: args.lastSessionByTaskId ?? {},
      },
      taskPRs: { byTaskId: {} as Record<string, unknown[]> },
      environmentIdBySessionId: args.envIds,
      taskSessions: { items },
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
      taskId: TASK_ID,
      task: { primarySessionId: PRIMARY },
      store,
      switchToSession,
      loadTaskSessionsForTask: vi.fn(async () => []),
      setActiveTask: vi.fn(),
      setPreparingTaskId: vi.fn(),
    });
    return switchToSession;
  }

  it("prefers the user's last-selected session over primarySessionId on re-entry", () => {
    const LAST = "sess-gpt";
    const switchToSession = runSelect(
      makeKanbanStore({
        activeSessionId: "sess-other-task",
        envIds: { "sess-other-task": "env-B", [PRIMARY]: "env-A", [LAST]: "env-A" },
        lastSessionByTaskId: { [TASK_ID]: LAST },
      }),
    );

    expect(switchToSession).toHaveBeenCalledWith(TASK_ID, LAST, "sess-other-task");
  });

  it("falls back to primarySessionId when the remembered session has no env mapping", () => {
    const switchToSession = runSelect(
      makeKanbanStore({
        activeSessionId: null,
        envIds: { [PRIMARY]: "env-A" },
        lastSessionByTaskId: { [TASK_ID]: "sess-stale" },
      }),
    );

    expect(switchToSession).toHaveBeenCalledWith(TASK_ID, PRIMARY, null);
  });

  it("uses primarySessionId when no last-selected session is recorded for the task", () => {
    const switchToSession = runSelect(
      makeKanbanStore({
        activeSessionId: null,
        envIds: { [PRIMARY]: "env-A" },
        lastSessionByTaskId: {},
      }),
    );

    expect(switchToSession).toHaveBeenCalledWith(TASK_ID, PRIMARY, null);
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
        envIds: { [PRIMARY]: "env-A", [POISONED]: "env-B" },
        lastSessionByTaskId: { [TASK_ID]: POISONED },
        sessionTaskIds: { [POISONED]: "task-B", [PRIMARY]: TASK_ID },
      }),
    );

    expect(switchToSession).toHaveBeenCalledWith(TASK_ID, PRIMARY, null);
  });
});
