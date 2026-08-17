import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  prepareAndSwitchTask,
  buildSwitchToSession,
  resolveTaskSessionId,
  selectTaskWithLayout,
} from "./task-select-helpers";

const TASK_ID = "task-A";
const PRIMARY = "sess-primary";
const SECONDARY = "secondary";
const OTHER_SESSION_ID = "other-session";
const PRIMARY_ENV_ID = "env-primary";
const SECONDARY_ENV_ID = "env-secondary";
const NEWER_AT = "2026-08-14T12:02:00Z";
const OLDER_AT = "2026-08-14T12:01:00Z";

describe("resolveTaskSessionId", () => {
  const sessions = [
    {
      id: SECONDARY,
      task_id: TASK_ID,
      state: "WAITING_FOR_INPUT",
      pending_action: "clarification",
      updated_at: NEWER_AT,
    },
    {
      id: "primary",
      task_id: TASK_ID,
      state: "WAITING_FOR_INPUT",
      is_primary: true,
      updated_at: OLDER_AT,
    },
  ] as TaskSession[];

  it("selects the newest input-capable session that owns the task action", () => {
    expect(
      resolveTaskSessionId({
        sessions,
        preferredSessionId: "primary",
        taskPendingAction: "clarification",
      }),
    ).toBe(SECONDARY);
  });

  it("does not guess a fallback when the only matching owner is terminal", () => {
    expect(
      resolveTaskSessionId({
        sessions: [{ ...sessions[0], state: "COMPLETED" }, sessions[1]],
        preferredSessionId: "primary",
        taskPendingAction: "clarification",
      }),
    ).toBe("");
  });

  it("preserves remembered and primary fallback for a clean task", () => {
    expect(
      resolveTaskSessionId({
        sessions,
        preferredSessionId: SECONDARY,
        taskPendingAction: null,
      }),
    ).toBe(SECONDARY);
  });
});

const { dockviewState } = vi.hoisted(() => ({
  dockviewState: {
    api: null as unknown,
    buildDefaultLayout: vi.fn(),
  },
}));

vi.mock("@/lib/services/session-launch-service", () => ({
  launchSession: vi.fn(),
}));
vi.mock("@/lib/services/session-launch-helpers", () => ({
  buildPrepareRequest: vi.fn(() => ({ request: { taskId: "task-new" } })),
}));
vi.mock("@/lib/state/dockview-store", () => ({
  performLayoutSwitch: vi.fn(),
  releaseLayoutToDefault: vi.fn(),
  useDockviewStore: { getState: () => dockviewState },
}));
vi.mock("@/lib/links", () => ({
  replaceTaskUrl: vi.fn(),
}));

import { launchSession, type LaunchSessionResponse } from "@/lib/services/session-launch-service";
import { performLayoutSwitch, releaseLayoutToDefault } from "@/lib/state/dockview-store";
import { replaceTaskUrl } from "@/lib/links";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";

const NEW_TASK_ID = "task-new";
const OLD_SESSION_ID = "old-session";

function makeStore(activeSessionId: string | null, hasLinkedPR = false): StoreApi<AppState> {
  const state = {
    tasks: { activeSessionId },
    taskPRs: {
      byTaskId: hasLinkedPR ? { [NEW_TASK_ID]: [{}] } : ({} as Record<string, unknown[]>),
    },
    environmentIdBySessionId: activeSessionId ? { [activeSessionId]: "env-old" } : {},
  };
  return {
    getState: () => state as unknown as AppState,
    setState: vi.fn(),
    subscribe: vi.fn(),
  } as unknown as StoreApi<AppState>;
}

function makeEnvStore(
  envIds: Record<string, string>,
  taskSessionsByTask: Record<string, string[]> = {},
): StoreApi<AppState> {
  return {
    getState: () =>
      ({
        environmentIdBySessionId: envIds,
        taskSessionsByTask: {
          itemsByTaskId: Object.fromEntries(
            Object.entries(taskSessionsByTask).map(([taskId, sessionIds]) => [
              taskId,
              sessionIds.map((id) => ({ id })),
            ]),
          ),
        },
      }) as unknown as AppState,
  } as unknown as StoreApi<AppState>;
}

function makeKanbanStore(args: {
  activeTaskId?: string | null;
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
      activeTaskId: args.activeTaskId ?? null,
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

async function flushTaskSelection() {
  await Promise.resolve();
  await Promise.resolve();
  await new Promise((resolve) => setTimeout(resolve, 0));
}

describe("selectTaskWithLayout pending owner", () => {
  it("loads sessions before switching even when the primary environment is known", async () => {
    const store = makeKanbanStore({
      activeSessionId: OTHER_SESSION_ID,
      envIds: { [PRIMARY]: PRIMARY_ENV_ID, [SECONDARY]: SECONDARY_ENV_ID },
    });
    const switchToSession = vi.fn();
    const loadTaskSessionsForTask = vi.fn(
      async () =>
        [
          {
            id: SECONDARY,
            task_id: TASK_ID,
            state: "WAITING_FOR_INPUT",
            pending_action: "clarification",
            updated_at: NEWER_AT,
          },
          {
            id: PRIMARY,
            task_id: TASK_ID,
            state: "WAITING_FOR_INPUT",
            is_primary: true,
            updated_at: OLDER_AT,
          },
        ] as TaskSession[],
    );

    selectTaskWithLayout({
      taskId: TASK_ID,
      task: { primarySessionId: PRIMARY, taskPendingAction: "clarification" },
      store,
      switchToSession,
      loadTaskSessionsForTask,
      setActiveTask: vi.fn(),
      setPreparingTaskId: vi.fn(),
    });

    expect(switchToSession).not.toHaveBeenCalled();
    await flushTaskSelection();
    expect(loadTaskSessionsForTask).toHaveBeenCalledWith(TASK_ID, { force: true });
    expect(switchToSession).toHaveBeenCalledWith(TASK_ID, SECONDARY, OTHER_SESSION_ID);
  });

  it("uses the status-summary pending owner when the legacy field is empty", async () => {
    const store = makeKanbanStore({
      activeSessionId: OTHER_SESSION_ID,
      envIds: { [PRIMARY]: PRIMARY_ENV_ID, [SECONDARY]: SECONDARY_ENV_ID },
    });
    const switchToSession = vi.fn();
    const loadTaskSessionsForTask = vi.fn(
      async () =>
        [
          {
            id: SECONDARY,
            task_id: TASK_ID,
            state: "WAITING_FOR_INPUT",
            pending_action: "clarification",
            started_at: NEWER_AT,
            updated_at: NEWER_AT,
          },
          {
            id: PRIMARY,
            task_id: TASK_ID,
            state: "WAITING_FOR_INPUT",
            is_primary: true,
            started_at: OLDER_AT,
            updated_at: OLDER_AT,
          },
        ] as TaskSession[],
    );

    selectTaskWithLayout({
      taskId: TASK_ID,
      task: {
        primarySessionId: PRIMARY,
        statusSummary: { pending_action: "clarification" },
      },
      store,
      switchToSession,
      loadTaskSessionsForTask,
      setActiveTask: vi.fn(),
      setPreparingTaskId: vi.fn(),
    });

    expect(switchToSession).not.toHaveBeenCalled();
    await flushTaskSelection();
    expect(switchToSession).toHaveBeenCalledWith(TASK_ID, SECONDARY, OTHER_SESSION_ID);
  });
});

describe("selectTaskWithLayout pending summary authority", () => {
  it("lets an explicit summary clear override a stale legacy pending field", () => {
    const store = makeKanbanStore({
      activeSessionId: OTHER_SESSION_ID,
      envIds: { [PRIMARY]: PRIMARY_ENV_ID },
    });
    const switchToSession = vi.fn();

    selectTaskWithLayout({
      taskId: TASK_ID,
      task: {
        primarySessionId: PRIMARY,
        taskPendingAction: "clarification",
        statusSummary: {},
      },
      store,
      switchToSession,
      loadTaskSessionsForTask: vi.fn(async () => []),
      setActiveTask: vi.fn(),
      setPreparingTaskId: vi.fn(),
    });

    expect(switchToSession).toHaveBeenCalledWith(TASK_ID, PRIMARY, OTHER_SESSION_ID);
  });

  it("handles a rejected background refresh after an immediate selection", async () => {
    const store = makeKanbanStore({
      activeSessionId: OTHER_SESSION_ID,
      envIds: { [PRIMARY]: PRIMARY_ENV_ID },
    });
    const switchToSession = vi.fn();

    selectTaskWithLayout({
      taskId: TASK_ID,
      task: { primarySessionId: PRIMARY },
      store,
      switchToSession,
      loadTaskSessionsForTask: vi.fn(async () => {
        throw new Error("refresh rejected");
      }),
      setActiveTask: vi.fn(),
      setPreparingTaskId: vi.fn(),
    });

    await flushTaskSelection();
    expect(switchToSession).toHaveBeenCalledWith(TASK_ID, PRIMARY, OTHER_SESSION_ID);
  });

  it("does not choose a fallback session when pending-owner loading fails", async () => {
    const store = makeKanbanStore({
      activeSessionId: OTHER_SESSION_ID,
      envIds: { [PRIMARY]: PRIMARY_ENV_ID },
    });
    const switchToSession = vi.fn();
    const setActiveTask = vi.fn();

    selectTaskWithLayout({
      taskId: TASK_ID,
      task: { primarySessionId: PRIMARY, taskPendingAction: "clarification" },
      store,
      switchToSession,
      loadTaskSessionsForTask: vi.fn(async () => {
        throw new Error("offline");
      }),
      setActiveTask,
      setPreparingTaskId: vi.fn(),
    });

    await flushTaskSelection();
    expect(switchToSession).not.toHaveBeenCalled();
    expect(setActiveTask).toHaveBeenCalledWith(TASK_ID);
    expect(replaceTaskUrl).toHaveBeenCalledWith(TASK_ID);
  });

  it("uses the task-only fallback when pending-owner loading fails without a primary", async () => {
    const store = makeKanbanStore({ activeSessionId: OTHER_SESSION_ID, envIds: {} });
    const setActiveTask = vi.fn();

    selectTaskWithLayout({
      taskId: TASK_ID,
      task: { taskPendingAction: "clarification" },
      store,
      switchToSession: vi.fn(),
      loadTaskSessionsForTask: vi.fn(async () => {
        throw new Error("offline");
      }),
      setActiveTask,
      setPreparingTaskId: vi.fn(),
    });

    await flushTaskSelection();
    expect(setActiveTask).toHaveBeenCalledWith(TASK_ID);
    expect(replaceTaskUrl).toHaveBeenCalledWith(TASK_ID);
  });
});

describe("selectTaskWithLayout sessionless layout cleanup", () => {
  beforeEach(() => vi.clearAllMocks());

  it("navigates without launching when a pending task has no loaded owner", async () => {
    const outgoingEnvId = "env-outgoing";
    const store = makeKanbanStore({
      activeSessionId: OTHER_SESSION_ID,
      envIds: { [OTHER_SESSION_ID]: outgoingEnvId },
    });
    const setActiveTask = vi.fn();

    selectTaskWithLayout({
      taskId: TASK_ID,
      task: { taskPendingAction: "clarification" },
      store,
      switchToSession: vi.fn(),
      loadTaskSessionsForTask: vi.fn(async () => []),
      setActiveTask,
      setPreparingTaskId: vi.fn(),
    });

    await flushTaskSelection();
    expect(launchSession).not.toHaveBeenCalled();
    expect(releaseLayoutToDefault).toHaveBeenCalledWith(outgoingEnvId);
    expect(setActiveTask).toHaveBeenCalledWith(TASK_ID);
    expect(vi.mocked(releaseLayoutToDefault).mock.invocationCallOrder[0]).toBeLessThan(
      setActiveTask.mock.invocationCallOrder[0] ?? Number.POSITIVE_INFINITY,
    );
    expect(replaceTaskUrl).toHaveBeenCalledWith(TASK_ID);
  });

  it("uses the task-only fallback when sessionless loading fails", async () => {
    const store = makeKanbanStore({ activeSessionId: OTHER_SESSION_ID, envIds: {} });
    const setActiveTask = vi.fn();

    selectTaskWithLayout({
      taskId: TASK_ID,
      task: undefined,
      store,
      switchToSession: vi.fn(),
      loadTaskSessionsForTask: vi.fn(async () => {
        throw new Error("offline");
      }),
      setActiveTask,
      setPreparingTaskId: vi.fn(),
    });

    await flushTaskSelection();
    expect(launchSession).not.toHaveBeenCalled();
    expect(setActiveTask).toHaveBeenCalledWith(TASK_ID);
    expect(replaceTaskUrl).toHaveBeenCalledWith(TASK_ID);
  });
});

describe("prepareAndSwitchTask — outgoing-env panel cleanup", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    dockviewState.api = null;
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

  it("does not rebuild a user's layout when the task has a linked PR", async () => {
    vi.mocked(launchSession).mockResolvedValue({
      success: true,
      task_id: NEW_TASK_ID,
      session_id: "new-session",
      state: "ready",
    });
    dockviewState.api = {};

    const result = await prepareAndSwitchTask(
      NEW_TASK_ID,
      makeStore(OLD_SESSION_ID, true),
      vi.fn(),
      vi.fn(),
    );

    expect(result).toBe(true);
    expect(dockviewState.buildDefaultLayout).not.toHaveBeenCalled();
  });
});

describe("buildSwitchToSession", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("performs an env switch when the new session's environment is known", () => {
    const store = makeEnvStore(
      { "sess-old": "env-A", "sess-new": "env-B" },
      { "task-new": ["sess-new", "sess-sibling"] },
    );
    const setActiveSession = vi.fn();
    const switchToSession = buildSwitchToSession(store, setActiveSession);

    switchToSession("task-new", "sess-new", "sess-old");

    expect(setActiveSession).toHaveBeenCalledWith("task-new", "sess-new");
    expect(performLayoutSwitch).toHaveBeenCalledWith("env-A", "env-B", "sess-new", [
      "sess-new",
      "sess-sibling",
    ]);
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

describe("selectTaskWithLayout — archived tasks", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("navigates directly without loading or preparing a session", () => {
    const store = makeKanbanStore({
      activeSessionId: "sess-old",
      envIds: { "sess-old": "env-old" },
    });
    const loadTaskSessionsForTask = vi.fn(async () => []);
    const switchToSession = vi.fn();
    const setActiveTask = vi.fn();

    selectTaskWithLayout({
      taskId: "archived-task",
      task: { isArchived: true, primarySessionId: "archived-session" },
      store,
      switchToSession,
      loadTaskSessionsForTask,
      setActiveTask,
      setPreparingTaskId: vi.fn(),
    });

    expect(setActiveTask).toHaveBeenCalledWith("archived-task");
    expect(replaceTaskUrl).toHaveBeenCalledWith("archived-task");
    expect(loadTaskSessionsForTask).not.toHaveBeenCalled();
    expect(switchToSession).not.toHaveBeenCalled();
    expect(launchSession).not.toHaveBeenCalled();
  });
});
