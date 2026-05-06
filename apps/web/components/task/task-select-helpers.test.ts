import { describe, it, expect, vi, beforeEach } from "vitest";
import { prepareAndSwitchTask, buildSwitchToSession } from "./task-select-helpers";

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
