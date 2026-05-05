import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  finalizeNoSessionSelect,
  prepareAndSwitchTask,
  type FinalizeNoSessionSelectDeps,
} from "./task-select-helpers";

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

import { launchSession } from "@/lib/services/session-launch-service";
import { releaseLayoutToDefault } from "@/lib/state/dockview-store";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";

const NEW_TASK_ID = "task-new";
const OLD_SESSION_ID = "old-session";

function makeDeps(overrides?: Partial<FinalizeNoSessionSelectDeps>): FinalizeNoSessionSelectDeps {
  return {
    setActiveTask: vi.fn(),
    releaseLayoutToDefault: vi.fn(),
    replaceTaskUrl: vi.fn(),
    ...overrides,
  };
}

describe("finalizeNoSessionSelect", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("clears the dockview to a default layout, releasing the outgoing session", () => {
    const deps = makeDeps();
    finalizeNoSessionSelect(NEW_TASK_ID, OLD_SESSION_ID, deps);
    expect(deps.releaseLayoutToDefault).toHaveBeenCalledWith(OLD_SESSION_ID);
  });

  it("releases the layout even when there is no prior session", () => {
    const deps = makeDeps();
    finalizeNoSessionSelect(NEW_TASK_ID, null, deps);
    // We still need to reset the dockview so the new task starts from a clean
    // default layout — passing null lets the store fall back to its current
    // layout session id internally.
    expect(deps.releaseLayoutToDefault).toHaveBeenCalledWith(null);
  });

  it("sets the new active task and replaces the URL", () => {
    const deps = makeDeps();
    finalizeNoSessionSelect(NEW_TASK_ID, OLD_SESSION_ID, deps);
    expect(deps.setActiveTask).toHaveBeenCalledWith(NEW_TASK_ID);
    expect(deps.replaceTaskUrl).toHaveBeenCalledWith(NEW_TASK_ID);
  });

  it("releases the layout BEFORE setting the new active task", () => {
    // Order matters — releasing the layout depends on the still-active session
    // for portal cleanup. If we cleared activeSessionId first the release
    // would target the wrong (already-cleared) session.
    const calls: string[] = [];
    const deps = makeDeps({
      releaseLayoutToDefault: vi.fn(() => calls.push("release")),
      setActiveTask: vi.fn(() => calls.push("setActiveTask")),
      replaceTaskUrl: vi.fn(() => calls.push("replaceTaskUrl")),
    });
    finalizeNoSessionSelect(NEW_TASK_ID, OLD_SESSION_ID, deps);
    expect(calls).toEqual(["release", "setActiveTask", "replaceTaskUrl"]);
  });
});

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
    let resolveLaunch: (v: { session_id: string }) => void = () => {};
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

    resolveLaunch({ session_id: "new-session" });
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
});
