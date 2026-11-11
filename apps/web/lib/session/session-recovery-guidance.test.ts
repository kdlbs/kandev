import { describe, expect, it } from "vitest";
import { WebSocketRequestError } from "@/lib/ws/client";
import { sessionId, taskId, type TaskSession } from "@/lib/types/http";
import { contextContinuationDetails } from "@/lib/services/session-recovery-service";
import {
  beginSessionRecoveryGuidanceOperation,
  clearSessionRecoveryGuidance,
  invalidateSessionRecoveryGuidance,
  matchesContinuationDetailsToIdentity,
  publishSessionRecoveryGuidance,
  selectSessionRecoveryGuidance,
  sessionRecoveryGuidanceIdentityFromSession,
  type SessionRecoveryGuidanceIdentity,
} from "./session-recovery-guidance";

const TASK_ID = "task-1";
const SESSION_ID = "session-1";
const BLOCK_ID = "block-1";
const INCARNATION_ID = "incarnation-1";
const UNKNOWN_OUTCOME = "unknown_prompt_outcome";

const target = (overrides: Partial<SessionRecoveryGuidanceIdentity> = {}) => ({
  taskId: TASK_ID,
  sessionId: SESSION_ID,
  requestKey: `${TASK_ID}\u0000${SESSION_ID}\u0000${BLOCK_ID}`,
  errorStamp: "failure-1",
  generation: 3,
  blockId: BLOCK_ID,
  incarnationId: INCARNATION_ID,
  reason: UNKNOWN_OUTCOME,
  ...overrides,
});

function continuationHint(overrides: Record<string, unknown> = {}) {
  return new WebSocketRequestError("Saved history is available", "CONFLICT", {
    kind: "session_restore_required",
    recovery_action: "continue_from_history",
    reason: UNKNOWN_OUTCOME,
    session_id: SESSION_ID,
    generation: 3,
    block_id: BLOCK_ID,
    incarnation_id: INCARNATION_ID,
    ...overrides,
  });
}

describe("canonical projection arrival", () => {
  it("publishes a matching canonical projection that arrived after the request began", () => {
    const owner = {};
    const operation = beginSessionRecoveryGuidanceOperation(
      owner,
      target({ generation: undefined, blockId: undefined, incarnationId: undefined }),
    );
    const projected = target({ requestKey: "presentation-revision-2" });

    expect(publishSessionRecoveryGuidance(owner, operation, continuationHint(), projected)).toBe(
      true,
    );
    expect(selectSessionRecoveryGuidance(owner, projected)?.details).toMatchObject({
      session_id: SESSION_ID,
      generation: 3,
      block_id: BLOCK_ID,
    });
    expect(
      selectSessionRecoveryGuidance(owner, target({ requestKey: "presentation-revision-3" })),
    ).not.toBeNull();
  });
});

describe("session recovery guidance validation", () => {
  it("rejects foreign targets, changed blocks, and invalid generations", () => {
    const owner = {};
    const identity = target();
    const operation = beginSessionRecoveryGuidanceOperation(owner, identity);

    expect(
      publishSessionRecoveryGuidance(
        owner,
        operation,
        continuationHint({ session_id: "other" }),
        identity,
      ),
    ).toBe(false);
    expect(
      publishSessionRecoveryGuidance(
        owner,
        operation,
        continuationHint({ generation: 1.5 }),
        identity,
      ),
    ).toBe(false);
    expect(
      publishSessionRecoveryGuidance(
        owner,
        operation,
        continuationHint({ block_id: "block-2" }),
        identity,
      ),
    ).toBe(false);
    expect(selectSessionRecoveryGuidance(owner, identity)).toBeNull();
  });

  it("accepts stale guidance only when it names the current canonical block", () => {
    expect(
      matchesContinuationDetailsToIdentity(
        contextContinuationDetails(continuationHint()),
        target(),
      ),
    ).toBe(true);
    expect(
      matchesContinuationDetailsToIdentity(
        contextContinuationDetails(continuationHint({ block_id: "block-2" })),
        target(),
      ),
    ).toBe(false);
    expect(
      matchesContinuationDetailsToIdentity(
        contextContinuationDetails(continuationHint({ block_id: undefined })),
        target(),
      ),
    ).toBe(false);
  });

  it("keeps guidance identity stable across delivery presentation revisions", () => {
    const base: TaskSession = {
      id: sessionId(SESSION_ID),
      task_id: taskId(TASK_ID),
      state: "FAILED",
      started_at: "2026-11-11T00:00:00Z",
      updated_at: "2026-11-11T00:00:00Z",
      metadata: {
        agent_delivery_recovery: {
          phase: "uncertain",
          revision: 1,
          session_id: SESSION_ID,
          agent_execution_id: "execution-1",
          submission_id: "submission-1",
          stream_id: "stream-1",
          incarnation_id: INCARNATION_ID,
          harness_generation: 3,
          prompt_generation: 1,
        },
      },
      session_recovery_blocks: [
        {
          id: BLOCK_ID,
          incarnation_id: INCARNATION_ID,
          expected_generation: 3,
          reason: UNKNOWN_OUTCOME,
          consumer_reference: "agent_delivery",
          updated_at: "2026-11-11T00:00:00Z",
        },
      ],
    };
    const before = sessionRecoveryGuidanceIdentityFromSession(TASK_ID, base);
    const after = sessionRecoveryGuidanceIdentityFromSession(TASK_ID, {
      ...base,
      metadata: {
        ...base.metadata,
        agent_delivery_recovery: {
          ...(base.metadata?.agent_delivery_recovery as Record<string, unknown>),
          revision: 2,
        },
      },
      session_recovery_blocks: base.session_recovery_blocks?.map((block) => ({
        ...block,
        updated_at: "2026-11-11T00:05:00Z",
      })),
    });

    expect(after.requestKey).toBe(before.requestKey);
  });
});

describe("session recovery guidance ownership", () => {
  it("rejects late operation results and stale success while retaining newer guidance", () => {
    const owner = {};
    const identity = target();
    const oldOperation = beginSessionRecoveryGuidanceOperation(owner, identity);
    const currentOperation = beginSessionRecoveryGuidanceOperation(owner, identity);

    expect(publishSessionRecoveryGuidance(owner, oldOperation, continuationHint(), identity)).toBe(
      false,
    );
    expect(
      publishSessionRecoveryGuidance(owner, currentOperation, continuationHint(), identity),
    ).toBe(true);
    expect(clearSessionRecoveryGuidance(owner, oldOperation, identity)).toBe(false);
    expect(selectSessionRecoveryGuidance(owner, identity)).not.toBeNull();
    expect(clearSessionRecoveryGuidance(owner, currentOperation, identity)).toBe(true);
    expect(selectSessionRecoveryGuidance(owner, identity)).toBeNull();
  });

  it("retains same-incident guidance after an untyped continuation failure", () => {
    const owner = {};
    const identity = target();
    const originalOperation = beginSessionRecoveryGuidanceOperation(owner, identity);
    publishSessionRecoveryGuidance(owner, originalOperation, continuationHint(), identity);

    beginSessionRecoveryGuidanceOperation(owner, identity);

    expect(selectSessionRecoveryGuidance(owner, identity)?.details.reason).toBe(UNKNOWN_OUTCOME);
  });

  it("invalidates successor incidents and deleted sessions without crossing store owners", () => {
    const owner = {};
    const otherOwner = {};
    const identity = target();
    const operation = beginSessionRecoveryGuidanceOperation(owner, identity);
    publishSessionRecoveryGuidance(owner, operation, continuationHint(), identity);

    expect(selectSessionRecoveryGuidance(owner, target({ blockId: "successor-block" }))).toBeNull();
    expect(selectSessionRecoveryGuidance(otherOwner, identity)).toBeNull();
    expect(invalidateSessionRecoveryGuidance(owner, TASK_ID, SESSION_ID)).toBe(true);
    expect(selectSessionRecoveryGuidance(owner, identity)).toBeNull();
    expect(selectSessionRecoveryGuidance(otherOwner, identity)).toBeNull();
  });
});
