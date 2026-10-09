import { describe, expect, it } from "vitest";
import { mergeTaskSession } from "./session-merge";
import type { TaskSession } from "@/lib/types/http";

function session(metadata: Record<string, unknown>, updatedAt: string): TaskSession {
  return {
    id: "session-1",
    task_id: "task-1",
    state: "WAITING_FOR_INPUT",
    started_at: "2026-09-28T08:00:00.000Z",
    updated_at: updatedAt,
    metadata,
  } as TaskSession;
}

describe("TaskSession delivery recovery hydration reconciliation", () => {
  it("rejects an older recovery revision with an equal timestamp and preserves unrelated errors", () => {
    const currentRecovery = {
      phase: "settled",
      revision: 4,
      session_id: "session-1",
      agent_execution_id: "execution-1",
      submission_id: "submission-1",
      stream_id: "stream-1",
      incarnation_id: "incarnation-1",
      harness_generation: 2,
      prompt_generation: 3,
    };
    const unrelatedError = { code: "NATIVE_RESTORE_FAILED", message: "Restore needs attention" };

    const merged = mergeTaskSession(
      session(
        {
          agent_delivery_recovery: currentRecovery,
          last_agent_error: unrelatedError,
        },
        "2026-09-28T08:01:00.000Z",
      ),
      session(
        {
          agent_delivery_recovery: { ...currentRecovery, phase: "uncertain", revision: 3 },
          last_agent_error: { code: "DURABLE_DELIVERY_UNCERTAIN", message: "Old notice" },
        },
        "2026-09-28T08:01:00.000Z",
      ),
    );

    expect(merged.metadata?.agent_delivery_recovery).toEqual(currentRecovery);
    expect(merged.metadata?.last_agent_error).toEqual(unrelatedError);
  });

  it("accepts a newer settled revision without clearing an unrelated error", () => {
    const currentRecovery = {
      phase: "uncertain",
      revision: 2,
      session_id: "session-1",
      agent_execution_id: "execution-1",
      submission_id: "submission-1",
      stream_id: "stream-1",
      incarnation_id: "incarnation-1",
      harness_generation: 2,
      prompt_generation: 3,
    };
    const settledRecovery = { ...currentRecovery, phase: "settled", revision: 3 };
    const unrelatedError = { code: "NATIVE_RESTORE_FAILED", message: "Restore needs attention" };

    const merged = mergeTaskSession(
      session(
        {
          agent_delivery_recovery: currentRecovery,
          last_agent_error: unrelatedError,
        },
        "2026-09-28T08:01:00.000Z",
      ),
      session({ agent_delivery_recovery: settledRecovery }, "2026-09-28T08:02:00.000Z"),
    );

    expect(merged.metadata?.agent_delivery_recovery).toEqual(settledRecovery);
    expect(merged.metadata?.last_agent_error).toEqual(unrelatedError);
  });
});
