import { describe, it, expect } from "vitest";
import {
  isTerminalSessionState,
  isStaleSessionStateEvent,
  pickReplacementSessionId,
  shouldAdoptNewSession,
  shouldPreservePinnedSessionForTask,
} from "./agent-session";
import type { AppState } from "@/lib/state/store";
import type { TaskSessionState } from "@/lib/types/http";

function makeAppState(partial: Partial<AppState>): AppState {
  return {
    tasks: {
      activeTaskId: null,
      activeSessionId: null,
      pinnedSessionId: null,
      lastSessionByTaskId: {},
    },
    taskSessions: { items: {} },
    taskSessionsByTask: { itemsByTaskId: {} },
    ...partial,
  } as unknown as AppState;
}

describe("isTerminalSessionState", () => {
  it.each<[TaskSessionState | undefined, boolean]>([
    ["COMPLETED", true],
    ["FAILED", true],
    ["CANCELLED", true],
    ["RUNNING", false],
    ["STARTING", false],
    ["CREATED", false],
    ["WAITING_FOR_INPUT", false],
    [undefined, false],
  ])("returns %o → %s", (input, expected) => {
    expect(isTerminalSessionState(input)).toBe(expected);
  });
});

describe("isStaleSessionStateEvent", () => {
  it("returns false when payload has no updated_at", () => {
    expect(isStaleSessionStateEvent({ updated_at: "2026-01-02T00:00:00.000Z" }, undefined)).toBe(
      false,
    );
  });

  it("returns false when existing session has no updated_at", () => {
    expect(isStaleSessionStateEvent({}, "2026-01-01T00:00:00.000Z")).toBe(false);
  });

  it("returns true when payload updated_at is older than store", () => {
    expect(
      isStaleSessionStateEvent(
        { updated_at: "2026-01-02T00:00:00.000Z" },
        "2026-01-01T00:00:00.000Z",
      ),
    ).toBe(true);
  });

  it("returns false when payload updated_at is newer than store", () => {
    expect(
      isStaleSessionStateEvent(
        { updated_at: "2026-01-01T00:00:00.000Z" },
        "2026-01-02T00:00:00.000Z",
      ),
    ).toBe(false);
  });
});

describe("shouldAdoptNewSession", () => {
  it("adopts when there is no active session for the task", () => {
    const state = makeAppState({
      tasks: {
        activeTaskId: "t-1",
        activeSessionId: null,
        pinnedSessionId: null,
        lastSessionByTaskId: {},
      },
    });
    expect(shouldAdoptNewSession(state, "t-1", "STARTING")).toBe(true);
  });

  it("adopts when active session belongs to a different task", () => {
    const state = makeAppState({
      tasks: {
        activeTaskId: "t-1",
        activeSessionId: "s-other",
        pinnedSessionId: null,
        lastSessionByTaskId: {},
      },
      taskSessions: {
        items: { "s-other": { id: "s-other", task_id: "t-2", state: "RUNNING" } },
      } as unknown as AppState["taskSessions"],
    });
    expect(shouldAdoptNewSession(state, "t-1", "STARTING")).toBe(true);
  });

  it("adopts when active session is already terminal", () => {
    const state = makeAppState({
      tasks: {
        activeTaskId: "t-1",
        activeSessionId: "s-old",
        pinnedSessionId: null,
        lastSessionByTaskId: {},
      },
      taskSessions: {
        items: { "s-old": { id: "s-old", task_id: "t-1", state: "COMPLETED" } },
      } as unknown as AppState["taskSessions"],
    });
    expect(shouldAdoptNewSession(state, "t-1", "STARTING")).toBe(true);
  });

  it("adopts when the active session is parked idle", () => {
    const state = makeAppState({
      tasks: {
        activeTaskId: "t-1",
        activeSessionId: "s-old",
        pinnedSessionId: null,
        lastSessionByTaskId: {},
      },
      taskSessions: {
        items: { "s-old": { id: "s-old", task_id: "t-1", state: "IDLE" } },
      } as unknown as AppState["taskSessions"],
    });
    expect(shouldAdoptNewSession(state, "t-1", "STARTING")).toBe(true);
  });

  it("does NOT adopt while the current active session is still running", () => {
    const state = makeAppState({
      tasks: {
        activeTaskId: "t-1",
        activeSessionId: "s-old",
        pinnedSessionId: null,
        lastSessionByTaskId: {},
      },
      taskSessions: {
        items: { "s-old": { id: "s-old", task_id: "t-1", state: "RUNNING" } },
      } as unknown as AppState["taskSessions"],
    });
    expect(shouldAdoptNewSession(state, "t-1", "STARTING")).toBe(false);
  });

  it("does NOT adopt when the event is for a non-active task", () => {
    const state = makeAppState({
      tasks: {
        activeTaskId: "t-1",
        activeSessionId: null,
        pinnedSessionId: null,
        lastSessionByTaskId: {},
      },
    });
    expect(shouldAdoptNewSession(state, "t-2", "STARTING")).toBe(false);
  });

  it("does NOT adopt terminal state events", () => {
    const state = makeAppState({
      tasks: {
        activeTaskId: "t-1",
        activeSessionId: null,
        pinnedSessionId: null,
        lastSessionByTaskId: {},
      },
    });
    expect(shouldAdoptNewSession(state, "t-1", "COMPLETED")).toBe(false);
  });
});

describe("pickReplacementSessionId", () => {
  it("returns the newest non-terminal session in the per-task list", () => {
    const state = makeAppState({
      taskSessionsByTask: {
        itemsByTaskId: {
          "t-1": [
            { id: "s-1", task_id: "t-1", state: "COMPLETED", started_at: "", updated_at: "" },
            { id: "s-2", task_id: "t-1", state: "RUNNING", started_at: "", updated_at: "" },
            { id: "s-3", task_id: "t-1", state: "CANCELLED", started_at: "", updated_at: "" },
          ],
        },
      } as unknown as AppState["taskSessionsByTask"],
    });
    expect(pickReplacementSessionId(state, "t-1")).toBe("s-2");
  });

  it("returns null when all sessions are terminal", () => {
    const state = makeAppState({
      taskSessionsByTask: {
        itemsByTaskId: {
          "t-1": [
            { id: "s-1", task_id: "t-1", state: "COMPLETED", started_at: "", updated_at: "" },
            { id: "s-2", task_id: "t-1", state: "FAILED", started_at: "", updated_at: "" },
          ],
        },
      } as unknown as AppState["taskSessionsByTask"],
    });
    expect(pickReplacementSessionId(state, "t-1")).toBeNull();
  });

  it("returns null when the task has no sessions tracked", () => {
    expect(pickReplacementSessionId(makeAppState({}), "t-missing")).toBeNull();
  });
});

describe("shouldPreservePinnedSessionForTask", () => {
  it("preserves a missing pinned session while the per-task list is hydrating", () => {
    const state = makeAppState({
      tasks: {
        activeTaskId: "t-1",
        activeSessionId: "s-drifted",
        pinnedSessionId: "s-pinned",
        lastSessionByTaskId: {},
      },
      taskSessionsByTask: {
        itemsByTaskId: { "t-1": [] },
        loadedByTaskId: {},
        loadingByTaskId: { "t-1": true },
      } as unknown as AppState["taskSessionsByTask"],
    });

    expect(shouldPreservePinnedSessionForTask(state, "t-1")).toBe(true);
  });

  it("does not preserve a missing pinned session once the per-task list is loaded", () => {
    const state = makeAppState({
      tasks: {
        activeTaskId: "t-1",
        activeSessionId: "s-drifted",
        pinnedSessionId: "s-pinned",
        lastSessionByTaskId: {},
      },
      taskSessionsByTask: {
        itemsByTaskId: { "t-1": [] },
        loadedByTaskId: { "t-1": true },
        loadingByTaskId: {},
      } as unknown as AppState["taskSessionsByTask"],
    });

    expect(shouldPreservePinnedSessionForTask(state, "t-1")).toBe(false);
  });

  it("does not preserve a stale by-id row when a loaded per-task list omits the pin", () => {
    const state = makeAppState({
      tasks: {
        activeTaskId: "t-1",
        activeSessionId: "s-drifted",
        pinnedSessionId: "s-pinned",
        lastSessionByTaskId: {},
      },
      taskSessions: {
        items: {
          "s-pinned": { id: "s-pinned", task_id: "t-1", state: "RUNNING" },
        },
      } as unknown as AppState["taskSessions"],
      taskSessionsByTask: {
        itemsByTaskId: { "t-1": [] },
        loadedByTaskId: { "t-1": true },
        loadingByTaskId: {},
      } as unknown as AppState["taskSessionsByTask"],
    });

    expect(shouldPreservePinnedSessionForTask(state, "t-1")).toBe(false);
  });
});
