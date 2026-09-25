import { describe, expect, it } from "vitest";
import {
  reconcileConversationChange,
  type ConversationChange,
  type ConversationReconciliationState,
} from "./conversation-reconciliation";

const state: ConversationReconciliationState = { appliedRevision: "10", epoch: "epoch-1" };

function change(overrides: Partial<ConversationChange> = {}): ConversationChange {
  return {
    protocol_version: 2,
    scope_id: "scope-1",
    session_id: "session-1",
    epoch: "epoch-1",
    base_revision: "10",
    revision: "11",
    operations: [],
    ...overrides,
  };
}

describe("conversation revision reconciliation", () => {
  it("applies a contiguous coverage batch and advances the revision", () => {
    const result = reconcileConversationChange(state, change({ revision: "11" }));

    expect(result.kind).toBe("applied");
    expect(result.state).toEqual({ appliedRevision: "11", epoch: "epoch-1" });
  });

  it("requests repair when a revision interval starts after the applied revision", () => {
    const result = reconcileConversationChange(
      state,
      change({ base_revision: "11", revision: "12" }),
    );

    expect(result.kind).toBe("recover");
    expect(result.state).toBe(state);
  });

  it("treats an already covered batch as a duplicate", () => {
    const result = reconcileConversationChange(
      { appliedRevision: "12", epoch: "epoch-1" },
      change({ base_revision: "10", revision: "12" }),
    );

    expect(result.kind).toBe("duplicate");
    expect(result.state.appliedRevision).toBe("12");
  });

  it("requests repair for a changed epoch, reset, malformed revision, or oversized batch", () => {
    expect(reconcileConversationChange(state, change({ epoch: "epoch-2" })).kind).toBe("recover");
    expect(reconcileConversationChange(state, change({ reset: true })).kind).toBe("recover");
    expect(reconcileConversationChange(state, change({ revision: "not-a-revision" })).kind).toBe(
      "recover",
    );
    expect(
      reconcileConversationChange(
        state,
        change({
          operations: Array.from({ length: 257 }, (_, index) => ({
            kind: "remove",
            entity: "message",
            id: `message-${index}`,
          })),
        }),
      ).kind,
    ).toBe("recover");
  });
});
