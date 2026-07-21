import { describe, expect, it } from "vitest";
import {
  isLiveAutoMRPanelIdentity,
  resolveAutoMRPanelAction,
  scheduleAfterTwoFrames,
} from "./dockview-auto-mr-panel";

describe("resolveAutoMRPanelAction", () => {
  it("adds a GitLab panel for a linked MR that has not been offered", () => {
    expect(
      resolveAutoMRPanelAction({
        hasMR: true,
        panelExists: false,
        restoring: false,
        maximized: false,
        offered: false,
      }),
    ).toBe("add");
  });

  it("removes only the auto panel when GitLab is no longer the primary review", () => {
    expect(
      resolveAutoMRPanelAction({
        hasMR: false,
        panelExists: true,
        restoring: false,
        maximized: false,
        offered: true,
      }),
    ).toBe("remove");
  });

  it("does not reopen a dismissed panel", () => {
    expect(
      resolveAutoMRPanelAction({
        hasMR: true,
        panelExists: false,
        restoring: false,
        maximized: false,
        offered: true,
      }),
    ).toBe("none");
  });
});

describe("scheduleAfterTwoFrames", () => {
  it("cancels the pending inner frame before it can mutate the next task layout", () => {
    const callbacks = new Map<number, FrameRequestCallback>();
    let nextID = 1;
    const schedule = (callback: FrameRequestCallback) => {
      const id = nextID++;
      callbacks.set(id, callback);
      return id;
    };
    const cancel = (id: number) => callbacks.delete(id);
    let calls = 0;

    const cleanup = scheduleAfterTwoFrames(() => calls++, schedule, cancel);
    const first = callbacks.get(1);
    callbacks.delete(1);
    first?.(0);
    cleanup();
    const second = callbacks.get(2);
    second?.(0);

    expect(calls).toBe(0);
    expect(callbacks.has(2)).toBe(false);
  });
});

describe("isLiveAutoMRPanelIdentity", () => {
  it("rejects a task switch between scheduled frames", () => {
    expect(
      isLiveAutoMRPanelIdentity(
        { taskId: "task-a", sessionId: "session-a", workspaceId: "workspace-a" },
        { taskId: "task-b", sessionId: "session-b", workspaceId: "workspace-a" },
      ),
    ).toBe(false);
  });
});
