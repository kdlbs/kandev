import type { TFunction } from "i18next";
import { describe, expect, it } from "vitest";
import type { SessionRecoveryBlockProjection } from "@/lib/types/http";
import type { AgentDeliveryRecovery } from "@/lib/session-agent-delivery-recovery";
import {
  canResumeDeliveryBlock,
  deliveryRecoveryRequestIdentity,
  persistedDeliveryRecoveryNotice,
} from "./session-delivery-recovery-view";

const block: SessionRecoveryBlockProjection = {
  id: "current-block",
  incarnation_id: "incarnation",
  expected_generation: 1,
  reason: "unresolved_durable_work",
  consumer_reference: "agent_delivery",
  delivery_submission_id: "",
  delivery_stream_id: "",
  updated_at: "2026-10-12T12:00:00Z",
};
const restored: AgentDeliveryRecovery = {
  phase: "restored",
  revision: 2,
  sessionId: "session",
  agentExecutionId: "old-execution",
  submissionId: "old-submission",
  streamId: "old-stream",
  incarnationId: "incarnation",
  harnessGeneration: 1,
  promptGeneration: 1,
};

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.8
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-008.5
describe("legacy delivery Resume availability", () => {
  it("offers Resume with absent or historical restored metadata", () => {
    expect(canResumeDeliveryBlock(null, block)).toBe(true);
    expect(canResumeDeliveryBlock(restored, block)).toBe(true);
  });
  it.each(["uncertain", "reconnecting"] as const)(
    "keeps %s recovery on its existing path",
    (phase) => {
      expect(canResumeDeliveryBlock({ ...restored, phase }, block)).toBe(false);
    },
  );
  it.each([
    { delivery_submission_id: "identified" },
    { delivery_stream_id: "identified" },
    { consumer_reference: "workspace" },
    { reason: "native_state_missing" },
  ])("does not offer legacy Resume for unrelated or identified blocks: %o", (override) => {
    expect(canResumeDeliveryBlock(null, { ...block, ...override })).toBe(false);
  });
  it("offers Resume for a bound cancellation block without active recovery metadata", () => {
    const cancelled = {
      ...block,
      reason: "unknown_prompt_outcome",
      delivery_submission_id: "cancelled-prompt",
    };
    expect(canResumeDeliveryBlock(null, cancelled)).toBe(true);
    expect(canResumeDeliveryBlock(restored, cancelled)).toBe(true);
    expect(canResumeDeliveryBlock({ ...restored, phase: "uncertain" }, cancelled)).toBe(false);
  });
  it("does not infer recovery from a restored snapshot alone", () => {
    expect(canResumeDeliveryBlock(restored, undefined)).toBe(false);
  });
  it("shows the new block cause instead of a historical reconstruction warning", () => {
    const notice = persistedDeliveryRecoveryNotice(
      { ...restored, reconstruction: { processIdentityKnown: false } },
      block,
      ((key: string) => key) as TFunction,
    );
    expect(notice).toBe("task:deliveryRecoveryLegacyResume");
  });
  it("preserves the missing-record notice for an identified missing submission", () => {
    expect(
      persistedDeliveryRecoveryNotice(
        null,
        { ...block, reason: "missing_canonical_submission", delivery_submission_id: "missing" },
        ((key: string) => key) as TFunction,
      ),
    ).toBe("task:deliveryRecoveryMissingSubmission");
  });
  it("invalidates pending results when a newer block supersedes restored metadata", () => {
    const before = deliveryRecoveryRequestIdentity(restored, block);
    expect(deliveryRecoveryRequestIdentity(restored, { ...block, id: "new-block" })).not.toBe(
      before,
    );
    expect(deliveryRecoveryRequestIdentity(restored, undefined)).not.toBe(before);
  });
});
