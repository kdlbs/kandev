import { describe, expect, it } from "vitest";
import {
  canRecoverManagedConversation,
  canSubmitManagedConversationInput,
  createManagedConversationInputIntent,
  orderManagedConversationInputs,
} from "./managed-conversation";
import type { PluginHostV2ManagedAgentInputReceipt } from "@kandev/plugin-sdk";

function receipt(
  hostInputId: string,
  sequence: number,
  state: PluginHostV2ManagedAgentInputReceipt["state"] = "accepted",
): PluginHostV2ManagedAgentInputReceipt {
  return {
    hostInputId,
    occurrenceKey: `occurrence-${hostInputId}`,
    sequence,
    origin: "human",
    payload: `message ${hostInputId}`,
    coalesceKey: "",
    conversationRevision: 3,
    state,
    createdAt: "2026-09-26T10:00:00Z",
    updatedAt: "2026-09-26T10:00:00Z",
    queueEntryId: "",
    executionId: "",
    turnId: "",
    supersededBy: "",
  };
}

describe("managed conversation UI contract", () => {
  it("orders and deduplicates input receipts by their durable sequence", () => {
    expect(
      orderManagedConversationInputs([
        receipt("later", 8),
        receipt("earlier", 2),
        receipt("later", 8, "running"),
      ]).map(({ hostInputId, state }) => [hostInputId, state]),
    ).toEqual([
      ["earlier", "accepted"],
      ["later", "running"],
    ]);
  });

  it("keeps the occurrence and idempotency keys stable for an uncertain retry", () => {
    const intent = createManagedConversationInputIntent("review the blocked change", "input-key");

    expect(intent).toEqual({
      requestId: "input-key",
      idempotencyKey: "input-key",
      occurrenceKey: "input-key",
      origin: "human",
      payload: "review the blocked change",
    });
    expect(createManagedConversationInputIntent(intent.payload, intent.occurrenceKey)).toEqual(
      intent,
    );
  });

  it("allows queue admission while paused but blocks read-only, revoked and unsupported states", () => {
    expect(canSubmitManagedConversationInput({ state: "ready", readOnly: false })).toBe(true);
    expect(canSubmitManagedConversationInput({ state: "paused", readOnly: false })).toBe(true);
    expect(canSubmitManagedConversationInput({ state: "ready", readOnly: true })).toBe(false);
    expect(canSubmitManagedConversationInput({ state: "revoked", readOnly: false })).toBe(false);
    expect(canSubmitManagedConversationInput({ state: "unsupported", readOnly: false })).toBe(
      false,
    );
    expect(canSubmitManagedConversationInput({ state: "disconnected", readOnly: false })).toBe(
      false,
    );
  });

  it("requires a current session version and execution fence for recovery", () => {
    const base = {
      supported: true,
      readOnly: false,
      sessionState: "FAILED",
      sessionResourceVersion: "session-v5",
      executionId: "execution-5",
    };
    expect(canRecoverManagedConversation(base)).toBe(true);
    expect(canRecoverManagedConversation({ ...base, executionId: undefined })).toBe(false);
    expect(canRecoverManagedConversation({ ...base, sessionResourceVersion: "" })).toBe(false);
    expect(canRecoverManagedConversation({ ...base, sessionState: "RUNNING" })).toBe(false);
    expect(canRecoverManagedConversation({ ...base, readOnly: true })).toBe(false);
  });
});
