import { describe, it, expect } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import type { StoreApi } from "zustand";
import {
  isTerminalSessionState,
  isStaleSessionStateEvent,
  pickReplacementSessionId,
  shouldAdoptNewSession,
} from "./agent-session";
import { qk } from "@/lib/query/keys";
import type { AppState } from "@/lib/state/store";
import type { TaskSession, TaskSessionState } from "@/lib/types/http";

type TasksSlice = {
  activeTaskId: string | null;
  activeSessionId: string | null;
  pinnedSessionId: string | null;
  lastSessionByTaskId: Record<string, string>;
};

function makeStore(tasks: Partial<TasksSlice>): StoreApi<AppState> {
  const state = {
    tasks: {
      activeTaskId: null,
      activeSessionId: null,
      pinnedSessionId: null,
      lastSessionByTaskId: {},
      ...tasks,
    },
  } as unknown as AppState;
  return { getState: () => state } as unknown as StoreApi<AppState>;
}

function seedById(qc: QueryClient, session: Record<string, unknown> & { id: string }): void {
  qc.setQueryData(qk.taskSession.byId(session.id), session as unknown as TaskSession);
}

function seedByTask(
  qc: QueryClient,
  taskId: string,
  sessions: Array<Record<string, unknown>>,
): void {
  qc.setQueryData(qk.taskSession.byTask(taskId), {
    sessions: sessions as unknown as TaskSession[],
    total: sessions.length,
  });
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
    const store = makeStore({ activeTaskId: "t-1", activeSessionId: null });
    const qc = new QueryClient();
    expect(shouldAdoptNewSession(store, qc, "t-1", "STARTING")).toBe(true);
  });

  it("adopts when active session belongs to a different task", () => {
    const store = makeStore({ activeTaskId: "t-1", activeSessionId: "s-other" });
    const qc = new QueryClient();
    seedById(qc, { id: "s-other", task_id: "t-2", state: "RUNNING" });
    expect(shouldAdoptNewSession(store, qc, "t-1", "STARTING")).toBe(true);
  });

  it("adopts when active session is already terminal", () => {
    const store = makeStore({ activeTaskId: "t-1", activeSessionId: "s-old" });
    const qc = new QueryClient();
    seedById(qc, { id: "s-old", task_id: "t-1", state: "COMPLETED" });
    expect(shouldAdoptNewSession(store, qc, "t-1", "STARTING")).toBe(true);
  });

  it("does NOT adopt while the current active session is still running", () => {
    const store = makeStore({ activeTaskId: "t-1", activeSessionId: "s-old" });
    const qc = new QueryClient();
    seedById(qc, { id: "s-old", task_id: "t-1", state: "RUNNING" });
    expect(shouldAdoptNewSession(store, qc, "t-1", "STARTING")).toBe(false);
  });

  it("does NOT adopt when the event is for a non-active task", () => {
    const store = makeStore({ activeTaskId: "t-1", activeSessionId: null });
    const qc = new QueryClient();
    expect(shouldAdoptNewSession(store, qc, "t-2", "STARTING")).toBe(false);
  });

  it("does NOT adopt terminal state events", () => {
    const store = makeStore({ activeTaskId: "t-1", activeSessionId: null });
    const qc = new QueryClient();
    expect(shouldAdoptNewSession(store, qc, "t-1", "COMPLETED")).toBe(false);
  });
});

describe("pickReplacementSessionId", () => {
  it("returns the newest non-terminal session in the per-task list", () => {
    const qc = new QueryClient();
    seedByTask(qc, "t-1", [
      { id: "s-1", task_id: "t-1", state: "COMPLETED", started_at: "", updated_at: "" },
      { id: "s-2", task_id: "t-1", state: "RUNNING", started_at: "", updated_at: "" },
      { id: "s-3", task_id: "t-1", state: "CANCELLED", started_at: "", updated_at: "" },
    ]);
    expect(pickReplacementSessionId(qc, "t-1")).toBe("s-2");
  });

  it("returns null when all sessions are terminal", () => {
    const qc = new QueryClient();
    seedByTask(qc, "t-1", [
      { id: "s-1", task_id: "t-1", state: "COMPLETED", started_at: "", updated_at: "" },
      { id: "s-2", task_id: "t-1", state: "FAILED", started_at: "", updated_at: "" },
    ]);
    expect(pickReplacementSessionId(qc, "t-1")).toBeNull();
  });

  it("returns null when the task has no sessions tracked", () => {
    expect(pickReplacementSessionId(new QueryClient(), "t-missing")).toBeNull();
  });
});
