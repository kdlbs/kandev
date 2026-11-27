import { describe, expect, it } from "vitest";
import type { TFunction } from "i18next";
import type { KindProposal } from "@/lib/api/domains/coordinator-api";
import { kindApprovedToast, kindFailureText, policyLineText } from "./outcome-copy";

const t = ((key: string, params?: Record<string, unknown>) =>
  params ? `${key}|${JSON.stringify(params)}` : key) as unknown as TFunction;

function kindRow(
  kind: KindProposal["kind"],
  overrides: Record<string, unknown> = {},
): KindProposal {
  return {
    id: "p1",
    coordinator_id: "c1",
    workspace_id: "w1",
    status: "failed",
    kind,
    spec: { task_id: "t1", rationale: "r" },
    final_spec: null,
    claimed_at: null,
    task_id: null,
    error: null,
    reject_reason: null,
    decided_by: null,
    created_at: "2026-09-27T00:00:00Z",
    updated_at: "2026-09-27T00:00:00Z",
    ...overrides,
  } as KindProposal;
}

describe("kindFailureText", () => {
  it.each([
    ["resume", "outcome_unknown", "coordinator:outcomeUnknown"],
    ["message", "task_archived", "coordinator:outcomeTaskArchived"],
    ["resume", "not_resumable", "coordinator:outcomeNotResumable"],
    ["message", "not_accepting", "coordinator:outcomeNotAccepting"],
    ["message", "queue_full", "coordinator:outcomeQueueFull"],
    ["move", "task_left_workflow", "coordinator:outcomeTaskLeftWorkflow"],
    ["move", "moved", "coordinator:outcomeMoved"],
    ["move", "step_missing", "coordinator:outcomeStepMissing"],
    ["move", "step_is_done", "coordinator:outcomeStepIsDone"],
    ["move", "step_starts_agent", "coordinator:outcomeStepStartsAgent"],
    ["move", "agent_running", "coordinator:outcomeAgentRunning"],
    ["move", "step_full", "coordinator:outcomeStepFull"],
  ] as const)("%s %s", (kind, error, key) => {
    expect(kindFailureText(kindRow(kind, { error }), t)).toBe(key);
  });

  it("uses the generic copy for a code the table does not name for that kind", () => {
    expect(kindFailureText(kindRow("resume", { error: "queue_full" }), t)).toBe(
      'coordinator:outcomeOther|{"error":"queue_full"}',
    );
  });

  it("uses the no-error copy for an empty code", () => {
    expect(kindFailureText(kindRow("move", { error: null }), t)).toBe(
      "coordinator:outcomeOtherNoError",
    );
  });
});

describe("kindApprovedToast", () => {
  it("resume, with and without deferred", () => {
    expect(kindApprovedToast(kindRow("resume", { outcome: {} }), "K-1", "", t)).toContain(
      "toastApprovedResume|",
    );
    expect(
      kindApprovedToast(kindRow("resume", { outcome: { deferred: true } }), "K-1", "", t),
    ).toContain("toastApprovedResumeDeferred");
  });

  it("message", () => {
    expect(kindApprovedToast(kindRow("message"), "K-1", "", t)).toContain("toastApprovedMessage");
  });

  it("move: plain, queued and noop", () => {
    expect(kindApprovedToast(kindRow("move", { outcome: null }), "K-1", "Review", t)).toContain(
      "toastApprovedMove|",
    );
    expect(
      kindApprovedToast(kindRow("move", { outcome: { queued: true } }), "K-1", "Review", t),
    ).toContain("toastApprovedMoveQueued");
    expect(
      kindApprovedToast(
        kindRow("move", { outcome: { noop: true, queued: true } }),
        "K-1",
        "Review",
        t,
      ),
    ).toContain("toastApprovedMoveNoop");
  });
});

describe("policyLineText", () => {
  it("names the action from the stored kind", () => {
    expect(policyLineText(kindRow("resume"), t)).toBe("coordinator:policyResume");
    expect(policyLineText(kindRow("message"), t)).toBe("coordinator:policyMessage");
    expect(policyLineText(kindRow("move"), t)).toBe("coordinator:policyMove");
  });
});
