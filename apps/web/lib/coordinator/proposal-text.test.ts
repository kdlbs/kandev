import { describe, expect, it, vi } from "vitest";
import type { TFunction } from "i18next";
import type { Proposal, ProposalSpec } from "@/lib/api/domains/coordinator-api";
import {
  approvedCardFallbackTitle,
  approvedStatusLine,
  effectiveProposalSpec,
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

describe("proposalStatusLine", () => {
  it("renders pending", () => {
    expect(proposalStatusLine(proposal({ status: "pending" }), t)).toBe(
      "coordinator:proposalStatusPending",
    );
  });

  it("renders approving", () => {
    expect(proposalStatusLine(proposal({ status: "approving" }), t)).toBe(
      "coordinator:proposalStatusApproving",
    );
  });

  it("renders failed with the error text", () => {
    expect(proposalStatusLine(proposal({ status: "failed", error: "boom" }), t)).toBe(
      "coordinator:proposalStatusFailedWithError(error=boom)",
    );
  });

  it("renders failed with no error text when error is null", () => {
    expect(proposalStatusLine(proposal({ status: "failed", error: null }), t)).toBe(
      "coordinator:proposalStatusFailed",
    );
  });

  it("renders rejected with the reason", () => {
    expect(proposalStatusLine(proposal({ status: "rejected", reject_reason: "Not now" }), t)).toBe(
      "coordinator:proposalStatusRejectedWithReason(reason=Not now)",
    );
  });

  it("renders rejected with no reason when reject_reason is null", () => {
    expect(proposalStatusLine(proposal({ status: "rejected", reject_reason: null }), t)).toBe(
      "coordinator:proposalStatusRejected",
    );
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
