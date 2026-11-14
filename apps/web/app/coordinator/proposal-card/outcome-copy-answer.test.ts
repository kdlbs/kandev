import { describe, expect, it } from "vitest";
import type { TFunction } from "i18next";
import type { KindProposal } from "@/lib/api/domains/coordinator-api";
import {
  answerRefusedKey,
  kindApprovedToast,
  kindFailureText,
  policyLineText,
} from "./outcome-copy";

const t = ((key: string) => key) as unknown as TFunction;

function answer(overrides: Record<string, unknown> = {}): KindProposal {
  return {
    id: "p1",
    coordinator_id: "c1",
    workspace_id: "w1",
    status: "failed",
    kind: "answer",
    spec: { task_id: "t1" },
    final_spec: null,
    error: null,
    ...overrides,
  } as unknown as KindProposal;
}

describe("answer outcome copy", () => {
  it.each([
    ["invalid_draft", "coordinator:outcomeInvalidDraft"],
    ["option_not_offered", "coordinator:outcomeOptionNotOffered"],
    ["resolution_in_progress", "coordinator:outcomeResolutionInProgress"],
    ["not_delivered", "coordinator:outcomeNotDelivered"],
    ["stale", "coordinator:answerRefusedStale"],
  ])("failure code %s maps to %s", (code, key) => {
    expect(kindFailureText(answer({ error: code }), t)).toBe(key);
  });

  it("refusal copy distinguishes an unavailable task", () => {
    expect(answerRefusedKey("task_archived")).toBe("coordinator:answerRefusedTaskUnavailable");
    expect(answerRefusedKey("stale")).toBe("coordinator:answerRefusedStale");
    expect(answerRefusedKey(null)).toBe("coordinator:answerRefusedStale");
  });

  it("has its own approval toast and policy line", () => {
    expect(kindApprovedToast(answer(), "KAN-1", "", t)).toBe("coordinator:toastApprovedAnswer");
    expect(policyLineText(answer({ status: "pending" }) as never, t)).toBe(
      "coordinator:policyAnswer",
    );
  });
});
