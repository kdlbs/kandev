import { describe, expect, it } from "vitest";
import type { TaskSession } from "@/lib/types/http";
import { sessionId, taskId } from "@/lib/types/ids";
import { resolveThreadColumnStatus, resolveThreadSessionStatus } from "./thread-session-status";

function session(overrides: Partial<TaskSession> = {}): TaskSession {
  return {
    id: sessionId("session-1"),
    task_id: taskId("task-1"),
    state: "IDLE",
    started_at: "2026-08-27T10:00:00Z",
    updated_at: "2026-08-27T12:00:00Z",
    ...overrides,
  };
}

describe("resolveThreadSessionStatus", () => {
  it("gives permission the highest precedence", () => {
    expect(
      resolveThreadSessionStatus(session({ state: "RUNNING", pending_action: "permission" })),
    ).toMatchObject({ kind: "permission", labelKey: "threads:statusPermissionNeeded" });
  });

  it("uses a question status only for an explicit clarification", () => {
    expect(
      resolveThreadSessionStatus(
        session({ state: "WAITING_FOR_INPUT", pending_action: "clarification" }),
      ),
    ).toMatchObject({ kind: "clarification", labelKey: "threads:statusQuestionFromAgent" });
  });

  it("does not turn plain WAITING_FOR_INPUT into a question", () => {
    expect(resolveThreadSessionStatus(session({ state: "WAITING_FOR_INPUT" }))).toMatchObject({
      kind: "waiting",
      hasAttention: false,
    });
  });

  it("distinguishes starting, foreground work, and a finished turn", () => {
    expect(resolveThreadSessionStatus(session({ state: "STARTING" })).kind).toBe("starting");
    expect(resolveThreadSessionStatus(session({ state: "RUNNING" })).kind).toBe("working");
    expect(resolveThreadSessionStatus(session({ state: "IDLE" })).kind).toBe("finished");
  });

  it("lets a foreground activity keep an idle session working", () => {
    expect(
      resolveThreadSessionStatus(session({ state: "IDLE", foreground_activity: "background" }))
        .kind,
    ).toBe("working");
  });

  it("keeps terminal errors distinct", () => {
    expect(resolveThreadSessionStatus(session({ state: "FAILED" })).kind).toBe("failed");
    expect(resolveThreadSessionStatus(session({ state: "CANCELLED" })).kind).toBe("cancelled");
  });
});

describe("resolveThreadColumnStatus", () => {
  it("shows review readiness only when no session needs explicit action", () => {
    expect(
      resolveThreadColumnStatus({
        taskState: "REVIEW",
        session: session({ state: "COMPLETED" }),
      }),
    ).toMatchObject({ kind: "review-ready", labelKey: "threads:statusReadyForReview" });
  });

  it("keeps session attention ahead of review readiness", () => {
    expect(
      resolveThreadColumnStatus({
        taskState: "REVIEW",
        session: session({ state: "WAITING_FOR_INPUT", pending_action: "clarification" }),
      }).kind,
    ).toBe("clarification");
  });

  it("keeps task-wide attention separate from the selected session action", () => {
    expect(
      resolveThreadColumnStatus({
        taskPendingAction: "permission",
        session: session({ state: "WAITING_FOR_INPUT", pending_action: null }),
      }),
    ).toMatchObject({ kind: "needs-you", hasAttention: true });
  });
});
