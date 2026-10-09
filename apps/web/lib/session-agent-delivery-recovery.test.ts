import { describe, expect, it } from "vitest";
import { readAgentDeliveryRecovery } from "./session-agent-delivery-recovery";

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
});
