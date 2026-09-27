import type { PluginHostV2ManagedAgentInputReceipt } from "@kandev/plugin-sdk";

export type ManagedConversationSubmitState =
  | "loading"
  | "ready"
  | "paused"
  | "disconnected"
  | "revoked"
  | "unsupported"
  | "unavailable"
  | "detached";

export type ManagedConversationInputIntent = {
  requestId: string;
  idempotencyKey: string;
  occurrenceKey: string;
  origin: "human";
  payload: string;
};

/** Keep the newest host receipt for each durable input, then restore FIFO order. */
export function orderManagedConversationInputs(
  receipts: readonly PluginHostV2ManagedAgentInputReceipt[],
): PluginHostV2ManagedAgentInputReceipt[] {
  const latest = new Map<
    string,
    { receipt: PluginHostV2ManagedAgentInputReceipt; index: number }
  >();
  receipts.forEach((receipt, index) => {
    const previous = latest.get(receipt.hostInputId);
    if (!previous || Date.parse(receipt.updatedAt) >= Date.parse(previous.receipt.updatedAt)) {
      latest.set(receipt.hostInputId, { receipt, index });
    }
  });
  return [...latest.values()]
    .sort((left, right) => {
      const sequence = left.receipt.sequence - right.receipt.sequence;
      return sequence || left.index - right.index;
    })
    .map(({ receipt }) => receipt);
}

/** Build once per draft and retain this value until the Host confirms admission. */
export function createManagedConversationInputIntent(
  payload: string,
  occurrenceKey: string,
): ManagedConversationInputIntent {
  return {
    requestId: occurrenceKey,
    idempotencyKey: occurrenceKey,
    occurrenceKey,
    origin: "human",
    payload,
  };
}

export function canSubmitManagedConversationInput(input: {
  state: ManagedConversationSubmitState;
  readOnly: boolean;
}): boolean {
  return !input.readOnly && (input.state === "ready" || input.state === "paused");
}

export function canRecoverManagedConversation(input: {
  supported: boolean;
  readOnly: boolean;
  sessionState?: string;
  sessionResourceVersion: string;
  executionId?: string;
}): boolean {
  return (
    input.supported &&
    !input.readOnly &&
    Boolean(input.sessionResourceVersion && input.executionId && input.sessionState) &&
    input.sessionState !== "RUNNING" &&
    input.sessionState !== "STARTING"
  );
}
