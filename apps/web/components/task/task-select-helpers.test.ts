import { describe, it, expect, vi, beforeEach } from "vitest";
import { finalizeNoSessionSelect, type FinalizeNoSessionSelectDeps } from "./task-select-helpers";

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
