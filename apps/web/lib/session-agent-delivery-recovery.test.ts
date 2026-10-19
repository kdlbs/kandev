import { describe, expect, it } from "vitest";
import {
  isAgentDeliveryRecoveryContinuationEligible,
  readAgentDeliveryRecovery,
  readAgentDeliveryHistoryGaps,
} from "./session-agent-delivery-recovery";

const validRecovery = {
  phase: "reconnecting",
  revision: 2,
  session_id: "session-1",
  agent_execution_id: "exec-1",
  submission_id: "submission-1",
  stream_id: "stream-1",
  incarnation_id: "inc-1",
  harness_generation: 3,
  prompt_generation: 9,
  message: "Reconnecting",
};

const historyGap = {
  first_sequence: 5495,
  last_sequence: 5499,
  received_sequence: 5494,
  projected_sequence: 5494,
};

it("reads an honest missing interval without changing the contiguous cursors", () => {
  expect(
    readAgentDeliveryRecovery({
      agent_delivery_recovery: { ...validRecovery, history_gap: historyGap },
    })?.historyGap,
  ).toEqual({
    firstSequence: 5495,
    lastSequence: 5499,
    receivedSequence: 5494,
    projectedSequence: 5494,
  });
});

it.each([
  { ...historyGap, first_sequence: 0 },
  { ...historyGap, last_sequence: 5493 },
  { ...historyGap, received_sequence: 5495 },
  { ...historyGap, projected_sequence: 5493 },
  { ...historyGap, last_sequence: Number.MAX_SAFE_INTEGER + 1 },
])("rejects invalid history gap evidence %j", (gap) => {
  expect(
    readAgentDeliveryRecovery({ agent_delivery_recovery: { ...validRecovery, history_gap: gap } })
      ?.historyGap,
  ).toBeUndefined();
});

it("keeps archived loss visible independently of the current prompt recovery", () => {
  const record = {
    stream_id: "stream-1",
    incarnation_id: "inc-1",
    harness_generation: 3,
    submission_id: "submission-1",
    payload_hash: "hash",
    native_session_id: "native",
    gap: historyGap,
  };
  expect(
    readAgentDeliveryHistoryGaps({
      agent_delivery_history_gaps: { "stream-1": record, malformed: { ...record, gap: null } },
    }),
  ).toEqual([
    {
      streamId: "stream-1",
      firstSequence: 5495,
      lastSequence: 5499,
      receivedSequence: 5494,
      projectedSequence: 5494,
    },
  ]);
  expect(readAgentDeliveryHistoryGaps({ agent_delivery_history_gaps: null })).toEqual([]);
});

describe("readAgentDeliveryRecovery", () => {
  it("reads the persisted identity and phase", () => {
    expect(readAgentDeliveryRecovery({ agent_delivery_recovery: validRecovery })).toEqual({
      phase: "reconnecting",
      revision: 2,
      sessionId: "session-1",
      agentExecutionId: "exec-1",
      submissionId: "submission-1",
      streamId: "stream-1",
      incarnationId: "inc-1",
      harnessGeneration: 3,
      promptGeneration: 9,
      message: "Reconnecting",
    });
  });

  it("rejects malformed and incomplete snapshots", () => {
    expect(readAgentDeliveryRecovery(null)).toBeNull();
    expect(
      readAgentDeliveryRecovery({ agent_delivery_recovery: { ...validRecovery, revision: 0 } }),
    ).toBeNull();
    expect(
      readAgentDeliveryRecovery({
        agent_delivery_recovery: { ...validRecovery, submission_id: "" },
      }),
    ).toBeNull();
    expect(
      readAgentDeliveryRecovery({
        agent_delivery_recovery: { ...validRecovery, phase: "stopped" },
      }),
    ).toBeNull();
  });

  it("keeps a reconstructed record visible while missing process proof blocks continuation", () => {
    const recovery = readAgentDeliveryRecovery({
      agent_delivery_recovery: {
        phase: "uncertain",
        revision: 1,
        session_id: "session-1",
        agent_execution_id: "",
        submission_id: "prompt:message-1",
        stream_id: "stream-1",
        incarnation_id: "incarnation-1",
        harness_generation: 7,
        prompt_generation: 0,
        reconstruction: { process_identity_known: false },
      },
    });

    expect(recovery).toMatchObject({
      submissionId: "prompt:message-1",
      promptGeneration: 0,
      reconstruction: { processIdentityKnown: false },
    });
    expect(recovery && isAgentDeliveryRecoveryContinuationEligible(recovery)).toBe(false);
  });
});

it("accepts a continued revision so late uncertain snapshots cannot restore the old block", () => {
  expect(
    readAgentDeliveryRecovery({
      agent_delivery_recovery: { ...validRecovery, phase: "continued", revision: 5 },
    })?.phase,
  ).toBe("continued");
});

it.each(["continued", "restored"] as const)(
  "accepts the %s terminal revision so late uncertain snapshots cannot restore the old block",
  (phase) => {
    expect(
      readAgentDeliveryRecovery({
        agent_delivery_recovery: { ...validRecovery, phase, revision: 5 },
      })?.phase,
    ).toBe(phase);
  },
);
