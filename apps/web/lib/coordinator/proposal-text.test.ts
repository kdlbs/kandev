import { describe, expect, it, vi } from "vitest";
import type { TFunction } from "i18next";
import type { Proposal, ProposalSpec } from "@/lib/api/domains/coordinator-api";
import {
  approvedCardFallbackTitle,
  approvedStatusLine,
  effectiveProposalSpec,
  isApprovalClaimStale,
  PROPOSAL_APPROVAL_STALE_MS,
  proposalStatusLine,
  resolveStepName,
  workflowStepLabel,
} from "./proposal-text";

const t = vi.fn((key: string, options?: Record<string, unknown>) => {
  if (!options) return key;
  const interpolated = Object.entries(options)
    .map(([k, v]) => `${k}=${String(v)}`)
    .join(",");
  return `${key}(${interpolated})`;
}) as unknown as TFunction;

function spec(overrides: Partial<ProposalSpec> = {}): ProposalSpec {
  return {
    title: "Add tests",
    description: "desc",
    rationale: "rationale",
    workflow_id: "wf-1",
    step_id: "step-1",
    repository_id: "repo-1",
    source_task_id: "t-1",
    ...overrides,
  };
}

function proposal(overrides: Partial<Proposal> = {}): Proposal {
  return {
    id: "p-1",
    coordinator_id: "c-1",
    workspace_id: "w-1",
    status: "pending",
    spec: spec(),
    final_spec: null,
    claimed_at: null,
    task_id: null,
    error: null,
    reject_reason: null,
    decided_by: null,
    created_at: "2026-09-27T00:00:00Z",
    updated_at: "2026-09-27T00:00:00Z",
    ...overrides,
  };
}

describe("effectiveProposalSpec", () => {
  it("uses final_spec when set", () => {
    const finalSpec = spec({ title: "Edited" });
    expect(effectiveProposalSpec(proposal({ final_spec: finalSpec }))).toBe(finalSpec);
  });

  it("falls back to spec when final_spec is null", () => {
    const p = proposal();
    expect(effectiveProposalSpec(p)).toBe(p.spec);
  });
});

describe("workflowStepLabel", () => {
  it("resolves workflow and step names", () => {
    const workflowNameById = new Map([["wf-1", "Build"]]);
    const stepNameByWorkflowStep = new Map([["wf-1:step-1", "In progress"]]);
    expect(workflowStepLabel(spec(), workflowNameById, stepNameByWorkflowStep)).toBe(
      "Build · In progress",
    );
  });

  it("falls back to the raw id when a name is unknown", () => {
    expect(workflowStepLabel(spec(), new Map(), new Map())).toBe("wf-1 · step-1");
  });
});

describe("resolveStepName", () => {
  it("resolves the step name", () => {
    const stepNameByWorkflowStep = new Map([["wf-1:step-1", "In progress"]]);
    expect(resolveStepName(spec(), stepNameByWorkflowStep)).toBe("In progress");
  });

  it("falls back to the raw step id", () => {
    expect(resolveStepName(spec(), new Map())).toBe("step-1");
  });
});

const NOW = new Date("2026-09-27T00:10:00Z").getTime();

describe("proposalStatusLine", () => {
  it("renders pending", () => {
    expect(proposalStatusLine(proposal({ status: "pending" }), t, NOW)).toBe(
      "coordinator:proposalStatusPending",
    );
  });

  it("renders approving with a null claimed_at (never stale)", () => {
    expect(proposalStatusLine(proposal({ status: "approving", claimed_at: null }), t, NOW)).toBe(
      "coordinator:proposalStatusApproving",
    );
  });

  it("renders approving when the claim is not yet stale", () => {
    const claimedAt = new Date(NOW - PROPOSAL_APPROVAL_STALE_MS + 1000).toISOString();
    expect(
      proposalStatusLine(proposal({ status: "approving", claimed_at: claimedAt }), t, NOW),
    ).toBe("coordinator:proposalStatusApproving");
  });

  it("renders approval-did-not-finish when the claim is stale", () => {
    const claimedAt = new Date(NOW - PROPOSAL_APPROVAL_STALE_MS).toISOString();
    expect(
      proposalStatusLine(proposal({ status: "approving", claimed_at: claimedAt }), t, NOW),
    ).toBe("coordinator:proposalStatusApprovingStale");
  });

  it("renders failed with the error text", () => {
    expect(proposalStatusLine(proposal({ status: "failed", error: "boom" }), t, NOW)).toBe(
      "coordinator:proposalStatusFailedWithError(error=boom)",
    );
  });

  it("renders failed with no error text when error is null", () => {
    expect(proposalStatusLine(proposal({ status: "failed", error: null }), t, NOW)).toBe(
      "coordinator:proposalStatusFailed",
    );
  });

  it("renders rejected with the reason", () => {
    expect(
      proposalStatusLine(proposal({ status: "rejected", reject_reason: "Not now" }), t, NOW),
    ).toBe("coordinator:proposalStatusRejectedWithReason(reason=Not now)");
  });

  it("renders rejected with no reason when reject_reason is null", () => {
    expect(proposalStatusLine(proposal({ status: "rejected", reject_reason: null }), t, NOW)).toBe(
      "coordinator:proposalStatusRejected",
    );
  });
});

describe("isApprovalClaimStale", () => {
  it("is never stale with a null claimed_at", () => {
    expect(isApprovalClaimStale(null, NOW)).toBe(false);
  });

  it("is not stale just under the threshold", () => {
    const claimedAt = new Date(NOW - PROPOSAL_APPROVAL_STALE_MS + 1).toISOString();
    expect(isApprovalClaimStale(claimedAt, NOW)).toBe(false);
  });

  it("is stale exactly at the threshold", () => {
    const claimedAt = new Date(NOW - PROPOSAL_APPROVAL_STALE_MS).toISOString();
    expect(isApprovalClaimStale(claimedAt, NOW)).toBe(true);
  });

  it("is stale well past the threshold", () => {
    const claimedAt = new Date(NOW - PROPOSAL_APPROVAL_STALE_MS - 60_000).toISOString();
    expect(isApprovalClaimStale(claimedAt, NOW)).toBe(true);
  });
});

describe("approvedStatusLine", () => {
  it("interpolates the card label", () => {
    expect(approvedStatusLine("KAN-432", t)).toBe(
      "coordinator:proposalStatusApproved(card=KAN-432)",
    );
  });
});

describe("approvedCardFallbackTitle", () => {
  it("returns the spec title", () => {
    expect(approvedCardFallbackTitle(spec({ title: "Fallback title" }))).toBe("Fallback title");
  });
});
