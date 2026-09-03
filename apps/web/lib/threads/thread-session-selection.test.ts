import { describe, expect, it } from "vitest";
import type { TaskSession } from "@/lib/types/http";
import { sessionId, taskId } from "@/lib/types/ids";
import { selectThreadSessionId } from "./thread-session-selection";

const TASK_ID = taskId("task-1");

function session(id: string, overrides: Partial<TaskSession> = {}): TaskSession {
  return {
    id: sessionId(id),
    task_id: TASK_ID,
    state: "IDLE",
    started_at: `2026-08-27T10:0${id.length}:00Z`,
    updated_at: "2026-08-27T12:00:00Z",
    ...overrides,
  };
}

describe("selectThreadSessionId", () => {
  it("uses a valid URL session before every other fallback", () => {
    const sessions = [
      session("primary", { is_primary: true }),
      session("question", { pending_action: "clarification" }),
      session("requested", { state: "RUNNING" }),
    ];

    expect(
      selectThreadSessionId(sessions, {
        requestedSessionId: "requested",
        currentSessionId: null,
      }),
    ).toBe("requested");
  });

  it("preserves a valid local selection across sibling status changes", () => {
    const sessions = [
      session("selected", { state: "IDLE" }),
      session("permission", { pending_action: "permission", state: "WAITING_FOR_INPUT" }),
    ];

    expect(
      selectThreadSessionId(sessions, {
        currentSessionId: "selected",
      }),
    ).toBe("selected");
  });

  it("prefers permission over clarification when no selection exists", () => {
    const sessions = [
      session("clarification", { pending_action: "clarification" }),
      session("permission", { pending_action: "permission" }),
      session("primary", { is_primary: true }),
    ];

    expect(selectThreadSessionId(sessions)).toBe("permission");
  });

  it("uses the newest active session before the primary session", () => {
    const sessions = [
      session("primary", { is_primary: true, state: "IDLE" }),
      session("older-active", { state: "STARTING", started_at: "2026-08-27T08:00:00Z" }),
      session("newer-active", { state: "RUNNING", started_at: "2026-08-27T09:00:00Z" }),
    ];

    expect(selectThreadSessionId(sessions)).toBe("newer-active");
  });

  it("falls back to the primary session, then the newest remaining session", () => {
    const primary = session("primary", { is_primary: true, state: "IDLE" });
    const newest = session("newest", {
      started_at: "2026-08-27T11:00:00Z",
      state: "COMPLETED",
    });

    expect(selectThreadSessionId([newest, primary])).toBe("primary");
    expect(selectThreadSessionId([newest])).toBe("newest");
  });

  it("falls back when the selected session disappears", () => {
    const sessions = [
      session("primary", { is_primary: true, state: "IDLE" }),
      session("replacement", { state: "RUNNING" }),
    ];

    expect(selectThreadSessionId(sessions, { currentSessionId: "removed" })).toBe("replacement");
  });

  it("ignores a URL session that belongs to another task", () => {
    const sessions = [session("primary", { is_primary: true })];

    expect(
      selectThreadSessionId(sessions, {
        requestedSessionId: "other-task-session",
      }),
    ).toBe("primary");
  });
});
