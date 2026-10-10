import { describe, expect, it } from "vitest";
import { mergeTaskSession } from "./session-merge";
import type { AgentDeliveryRecoveryPhase } from "@/lib/session-agent-delivery-recovery";
import type { TaskSession } from "@/lib/types/http";

const EQUAL_UPDATED_AT = "2026-09-28T08:01:00.000Z";
const EARLIER_UPDATED_AT = "2026-09-28T08:00:00.000Z";
const LATER_UPDATED_AT = "2026-09-28T08:02:00.000Z";
const UNCERTAIN_ERROR = {
  code: "DURABLE_DELIVERY_UNCERTAIN",
  message: "Old notice",
  agent_execution_id: "execution-1",
  details: "submission-1",
};
const UNRELATED_ERROR = { code: "NATIVE_RESTORE_FAILED", message: "Restore needs attention" };

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

function recovery(phase: AgentDeliveryRecoveryPhase, revision: number) {
  return {
    phase,
    revision,
    session_id: "session-1",
    agent_execution_id: "execution-1",
    submission_id: "submission-1",
    stream_id: "stream-1",
    incarnation_id: "incarnation-1",
    harness_generation: 2,
    prompt_generation: 3,
  };
}

describe("TaskSession delivery recovery hydration reconciliation", () => {
  it.each(["settled", "restored"] as const)(
    "rejects an older recovery revision after %s and preserves unrelated errors at equal timestamps",
    (phase) => {
      const currentRecovery = recovery(phase, 4);
      const merged = mergeTaskSession(
        session(
          {
            agent_delivery_recovery: currentRecovery,
            last_agent_error: UNRELATED_ERROR,
          },
          EQUAL_UPDATED_AT,
        ),
        session(
          {
            agent_delivery_recovery: recovery("uncertain", 3),
            last_agent_error: UNCERTAIN_ERROR,
          },
          EQUAL_UPDATED_AT,
        ),
      );

      expect(merged.metadata?.agent_delivery_recovery).toEqual(currentRecovery);
      expect(merged.metadata?.last_agent_error).toEqual(UNRELATED_ERROR);
    },
  );

  it("accepts a newer restored revision without clearing an unrelated error", () => {
    const restoredRecovery = recovery("restored", 3);
    const merged = mergeTaskSession(
      session(
        {
          agent_delivery_recovery: recovery("uncertain", 2),
          last_agent_error: UNRELATED_ERROR,
        },
        EARLIER_UPDATED_AT,
      ),
      session({ agent_delivery_recovery: restoredRecovery }, LATER_UPDATED_AT),
    );

    expect(merged.metadata?.agent_delivery_recovery).toEqual(restoredRecovery);
    expect(merged.metadata?.last_agent_error).toEqual(UNRELATED_ERROR);
  });
});

describe("TaskSession durable-delivery uncertainty error reconciliation", () => {
  it("clears only the matching uncertainty error when a newer restored revision arrives", () => {
    const restoredRecovery = recovery("restored", 3);
    const merged = mergeTaskSession(
      session(
        {
          agent_delivery_recovery: recovery("uncertain", 2),
          last_agent_error: UNCERTAIN_ERROR,
        },
        EARLIER_UPDATED_AT,
      ),
      session({ agent_delivery_recovery: restoredRecovery }, LATER_UPDATED_AT),
    );

    expect(merged.metadata?.agent_delivery_recovery).toEqual(restoredRecovery);
    expect(merged.metadata).not.toHaveProperty("last_agent_error");
  });

  it.each([
    {
      description: "a different execution",
      error: { ...UNCERTAIN_ERROR, agent_execution_id: "execution-other" },
    },
    {
      description: "a different submission",
      error: { ...UNCERTAIN_ERROR, details: "submission-other" },
    },
    {
      description: "a missing execution identity",
      error: {
        code: UNCERTAIN_ERROR.code,
        message: UNCERTAIN_ERROR.message,
        details: UNCERTAIN_ERROR.details,
      },
    },
    {
      description: "a missing submission identity",
      error: {
        code: UNCERTAIN_ERROR.code,
        message: UNCERTAIN_ERROR.message,
        agent_execution_id: UNCERTAIN_ERROR.agent_execution_id,
      },
    },
  ])("preserves a same-code error with $description", ({ error }) => {
    const merged = mergeTaskSession(
      session(
        {
          agent_delivery_recovery: recovery("uncertain", 2),
          last_agent_error: error,
        },
        EARLIER_UPDATED_AT,
      ),
      session({ agent_delivery_recovery: recovery("restored", 3) }, LATER_UPDATED_AT),
    );

    expect(merged.metadata?.last_agent_error).toEqual(error);
  });

  it("matches the legacy execution_id field when clearing the exact delivery error", () => {
    const legacyError = {
      code: UNCERTAIN_ERROR.code,
      message: UNCERTAIN_ERROR.message,
      execution_id: "execution-1",
      details: "submission-1",
    };
    const merged = mergeTaskSession(
      session(
        {
          agent_delivery_recovery: recovery("uncertain", 2),
          last_agent_error: legacyError,
        },
        EARLIER_UPDATED_AT,
      ),
      session({ agent_delivery_recovery: recovery("restored", 3) }, LATER_UPDATED_AT),
    );

    expect(merged.metadata).not.toHaveProperty("last_agent_error");
  });
});

describe("TaskSession delivery recovery revision reconciliation", () => {
  it("requires a newer recovery revision before clearing uncertainty", () => {
    const merged = mergeTaskSession(
      session(
        {
          agent_delivery_recovery: recovery("uncertain", 3),
          last_agent_error: UNCERTAIN_ERROR,
        },
        EARLIER_UPDATED_AT,
      ),
      session({ agent_delivery_recovery: recovery("restored", 3) }, LATER_UPDATED_AT),
    );

    expect(merged.metadata?.agent_delivery_recovery).toEqual(recovery("restored", 3));
    expect(merged.metadata?.last_agent_error).toEqual(UNCERTAIN_ERROR);
  });

  it("does not let a late uncertain snapshot restore the error after silent restoration", () => {
    const restoredRecovery = recovery("restored", 3);
    const merged = mergeTaskSession(
      session({ agent_delivery_recovery: restoredRecovery }, EQUAL_UPDATED_AT),
      session(
        {
          agent_delivery_recovery: recovery("uncertain", 2),
          last_agent_error: UNCERTAIN_ERROR,
        },
        EQUAL_UPDATED_AT,
      ),
    );

    expect(merged.metadata?.agent_delivery_recovery).toEqual(restoredRecovery);
    expect(merged.metadata).not.toHaveProperty("last_agent_error");
  });
});
