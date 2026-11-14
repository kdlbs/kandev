import { describe, expect, it } from "vitest";
import type { AnswerSpec } from "@/lib/api/domains/coordinator-api";
import type { Relay } from "@/lib/api/domains/coordinator-relay-api";
import { answerCurrencyOf } from "./use-answer-currency";

const base = { task_id: "t", sources: [], confidence: "low", needs_decision: false, rationale: "" };

function relay(parts: Partial<Relay>): Relay {
  return { task_id: "t", session_id: "s", clarification: null, permission: null, ...parts };
}

describe("answerCurrencyOf", () => {
  const clarification = {
    ...base,
    target: "clarification",
    pending_id: "p1",
    draft: {},
    snapshot: {},
  } as AnswerSpec;
  const permission = {
    ...base,
    target: "permission",
    pending_id: "p2",
    request_id: "r1",
    draft: { option_id: "allow" },
    snapshot: {},
  } as AnswerSpec;

  it("clarification is current only while its pending id is the live one", () => {
    const live = relay({ clarification: { pending_id: "p1", context: "", messages: [] } });
    expect(answerCurrencyOf(clarification, live)).toBe("current");
    const other = relay({ clarification: { pending_id: "p9", context: "", messages: [] } });
    expect(answerCurrencyOf(clarification, other)).toBe("stale");
    expect(answerCurrencyOf(clarification, relay({}))).toBe("stale");
  });

  it("permission needs both the request id and the pending id to match", () => {
    const message = (meta: Record<string, string>) =>
      relay({ permission: { message: { metadata: meta } as never } });
    expect(answerCurrencyOf(permission, message({ request_id: "r1", pending_id: "p2" }))).toBe(
      "current",
    );
    expect(answerCurrencyOf(permission, message({ request_id: "r2", pending_id: "p2" }))).toBe(
      "stale",
    );
    expect(answerCurrencyOf(permission, relay({}))).toBe("stale");
  });
});
