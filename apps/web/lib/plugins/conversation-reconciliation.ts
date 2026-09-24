import type { PluginConversationMessage, PluginConversationTurn } from "./types";

export type ConversationChangeOperation = {
  kind: "upsert" | "remove";
  entity: "message" | "turn";
  id: string;
  message?: PluginConversationMessage;
  turn?: PluginConversationTurn;
};

export type ConversationChange = {
  protocol_version: number;
  scope_id: string;
  session_id: string;
  epoch: string;
  base_revision: string;
  revision: string;
  check?: boolean;
  terminal?: boolean;
  reset?: boolean;
  operations: ConversationChangeOperation[];
};

export type ConversationReconciliationState = {
  appliedRevision: string;
  epoch: string;
};

export type ConversationReconciliationResult =
  | {
      kind: "applied";
      state: ConversationReconciliationState;
      operations: ConversationChangeOperation[];
    }
  | { kind: "duplicate" | "recover"; state: ConversationReconciliationState };

const MAX_CONVERSATION_OPERATIONS = 256;

function parseRevision(value: unknown): bigint | null {
  if (typeof value !== "string" || !/^(0|[1-9][0-9]*)$/.test(value)) return null;
  try {
    return BigInt(value);
  } catch {
    return null;
  }
}

function validOperation(value: unknown): value is ConversationChangeOperation {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const operation = value as Partial<ConversationChangeOperation>;
  if (
    (operation.kind !== "upsert" && operation.kind !== "remove") ||
    (operation.entity !== "message" && operation.entity !== "turn") ||
    typeof operation.id !== "string" ||
    operation.id === ""
  ) {
    return false;
  }
  if (operation.kind === "remove") return true;
  return operation.entity === "message" ? Boolean(operation.message) : Boolean(operation.turn);
}

function validChangeMetadata(
  change: Partial<ConversationChange>,
  state: ConversationReconciliationState,
): change is ConversationChange {
  return (
    change.protocol_version === 2 &&
    typeof change.epoch === "string" &&
    change.epoch === state.epoch &&
    change.reset !== true &&
    Array.isArray(change.operations) &&
    change.operations.length <= MAX_CONVERSATION_OPERATIONS &&
    change.operations.every(validOperation)
  );
}

export function reconcileConversationChange(
  state: ConversationReconciliationState,
  value: unknown,
): ConversationReconciliationResult {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return { kind: "recover", state };
  }
  const change = value as Partial<ConversationChange>;
  const applied = parseRevision(state.appliedRevision);
  const base = parseRevision(change.base_revision);
  const revision = parseRevision(change.revision);
  if (applied === null || base === null || revision === null) {
    return { kind: "recover", state };
  }
  if (!validChangeMetadata(change, state) || revision <= base) {
    return { kind: "recover", state };
  }
  if (revision <= applied) return { kind: "duplicate", state };
  if (base !== applied) return { kind: "recover", state };
  return {
    kind: "applied",
    state: { ...state, appliedRevision: change.revision as string },
    operations: change.operations,
  };
}
