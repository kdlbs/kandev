import { describe, expect, it } from "vitest";
import {
  sessionId,
  taskId,
  type ForegroundActivity,
  type TaskSession,
  type TaskSessionState,
} from "@/lib/types/http";
import { deriveSessionInputMode } from "./session-input-mode";

function session(
  state: TaskSessionState,
  foregroundActivity?: ForegroundActivity | null,
): TaskSession {
  return {
    id: sessionId("selected-session"),
    task_id: taskId("task-1"),
    state,
    foreground_activity: foregroundActivity,
    started_at: "2026-07-22T00:00:00Z",
    updated_at: "2026-07-22T00:00:00Z",
  };
}

describe("deriveSessionInputMode", () => {
  it.each([
    ["CREATED", undefined, "direct"],
    ["STARTING", undefined, "queue"],
    ["RUNNING", "generating", "queue"],
    ["RUNNING", undefined, "queue"],
    ["RUNNING", null, "queue"],
    ["RUNNING", "background", "direct"],
    ["IDLE", undefined, "direct"],
    ["WAITING_FOR_INPUT", undefined, "direct"],
    ["COMPLETED", undefined, "unavailable"],
    ["FAILED", undefined, "unavailable"],
    ["CANCELLED", undefined, "unavailable"],
  ] as const)("returns %s + %s as %s", (state, activity, expected) => {
    expect(deriveSessionInputMode(session(state, activity))).toBe(expected);
  });

  it("treats an unknown RUNNING activity conservatively as queue", () => {
    const selected = session("RUNNING", "unknown" as ForegroundActivity);
    expect(deriveSessionInputMode(selected)).toBe("queue");
  });

  it("returns unavailable when the selected session is missing", () => {
    expect(deriveSessionInputMode(null)).toBe("unavailable");
    expect(deriveSessionInputMode(undefined)).toBe("unavailable");
  });

  it("depends only on the selected session, not another session's activity", () => {
    const selected = session("RUNNING", "background");
    const another = session("RUNNING", "generating");

    expect(deriveSessionInputMode(selected)).toBe("direct");
    expect(deriveSessionInputMode(another)).toBe("queue");
  });
});
