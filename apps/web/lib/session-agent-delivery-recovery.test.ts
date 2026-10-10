import { describe, expect, it } from "vitest";
import {
  isAgentDeliveryRecoveryContinuationEligible,
  readAgentDeliveryRecovery,
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
