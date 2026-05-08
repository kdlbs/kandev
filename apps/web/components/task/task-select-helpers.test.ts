import { describe, it, expect, vi, beforeEach } from "vitest";
import { prepareAndSwitchTask } from "./task-select-helpers";

vi.mock("@/lib/services/session-launch-service", () => ({
  launchSession: vi.fn(),
}));
vi.mock("@/lib/services/session-launch-helpers", () => ({
  buildPrepareRequest: vi.fn(() => ({ request: { taskId: "task-new" } })),
}));
vi.mock("@/lib/state/dockview-store", () => ({
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
import { releaseLayoutToDefault } from "@/lib/state/dockview-store";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";

const NEW_TASK_ID = "task-new";
const OLD_SESSION_ID = "old-session";

/**
 * Regression: switching from a task with env-scoped panels open (file-editor,
 * diff-viewer, commit-detail, browser, vscode, pr-detail) to a task that
 * needs an env prepared previously left those panels mounted in the dockview
 * for the entire `await launchSession(...)` round trip. The user saw stray
 * tabs (e.g. a diff panel) from the old task on the new task's page while the
 * env was still being prepared.
 *
 * Fix: release the outgoing env (drops env-scoped portals + falls back to a
 * default layout) BEFORE awaiting `launchSession`, so the user sees a clean
 * slate during preparation. The new env is adopted in the usual way once
 * its session id is known.
 */
describe("prepareAndSwitchTask — outgoing-env panel cleanup", () => {
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

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("releases the outgoing env's panels before awaiting launchSession", async () => {
    // Make launchSession return a deferred we control so we can observe
    // synchronous side effects that happen before the await resolves.
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

    // The outgoing env release must have already happened — without this the
    // diff/file-editor panels from the previous task stay visible until
    // launchSession resolves and the WS env-id mapping arrives.
    expect(releaseLayoutToDefault).toHaveBeenCalledTimes(1);
    expect(switchToSession).not.toHaveBeenCalled();

    resolveLaunch({
      success: true,
      task_id: NEW_TASK_ID,
      session_id: "new-session",
      state: "ready",
    });
    const result = await promise;

    // Happy-path coverage: switchToSession must run with the new session id and
    // a null oldSessionId (releaseLayoutToDefault already saved + released the
    // outgoing env; passing the real oldSessionId would trigger a second
    // saveOutgoingEnv that overwrites envA's correct layout with the default).
    expect(result).toBe(true);
    expect(switchToSession).toHaveBeenCalledTimes(1);
    expect(switchToSession).toHaveBeenCalledWith(NEW_TASK_ID, "new-session", null);
    expect(setPreparingTaskId).toHaveBeenLastCalledWith(null);
  });

  /**
   * Failure-path regression: launch errors / empty responses go through
   * selectTaskWithLayout's no-session fallback. Releasing the outgoing env
   * exactly once — here, before the await — keeps the originating task's
   * saved layout intact. A second release on the failure tail would overwrite
   * it with the default layout (the api state after the first release).
   */
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
