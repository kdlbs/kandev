import { describe, expect, it } from "vitest";
import {
  sessionId,
  taskId,
  type ForegroundActivity,
  type TaskSession,
  type TaskSessionState,
} from "@/lib/types/http";
import { deriveSessionInputMode, resolvesSteeringAffordance } from "./session-input-mode";

function session(
  state: TaskSessionState,
  foregroundActivity?: ForegroundActivity | null,
  supportsSteering?: boolean,
): TaskSession {
  return {
    id: sessionId("selected-session"),
    task_id: taskId("task-1"),
    state,
    foreground_activity: foregroundActivity,
    supports_steering: supportsSteering,
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

  it("delivers directly for a generating session that supports steering", () => {
    const selected = session("RUNNING", "generating", true);
    expect(deriveSessionInputMode(selected)).toBe("direct");
  });

  it("queues a steer-capable session when a message is already queued", () => {
    const selected = session("RUNNING", "generating", true);
    expect(deriveSessionInputMode(selected, 1)).toBe("queue");
  });

  it("still queues a generating session when steering is not supported", () => {
    const selected = session("RUNNING", "generating", false);
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

describe("resolvesSteeringAffordance", () => {
  it("shows the steer affordance when steering is supported and nothing is queued", () => {
    expect(resolvesSteeringAffordance(true, 0)).toBe(true);
  });

  it("falls back to the queue affordance once anything is queued, even if steering is supported", () => {
    // SteerTask never jumps an already-queued message — it silently joins the
    // queue instead (see steer.go's order rule). The composer must not
    // promise delivery into the running turn when the next send would
    // actually be queued behind an existing entry.
    expect(resolvesSteeringAffordance(true, 1)).toBe(false);
    expect(resolvesSteeringAffordance(true, 2)).toBe(false);
  });

  it("stays false when steering is not supported, regardless of queue length", () => {
    expect(resolvesSteeringAffordance(false, 0)).toBe(false);
    expect(resolvesSteeringAffordance(false, 1)).toBe(false);
  });
});
