import { describe, expect, it } from "vitest";
import { derivePRTaskStatusSummary } from "./pr-task-status-summary";
import { makeTestPR as makePR } from "./pr-status-chip.test-fixtures";
import type { TaskPR, WorkflowAttention, WorkflowAttentionRun } from "@/lib/types/github";

const HEAD_SHA = "head-1";
const OBSERVED_AT = "2026-09-10T10:00:00Z";
const WORKFLOW_ATTENTION_ID = "workflow-attention";

function makeSummaryPR(overrides: Partial<TaskPR> = {}): TaskPR {
  return makePR({
    review_state: "",
    checks_state: "",
    mergeable_state: "",
    ...overrides,
  });
}

function makeAttention(
  state: WorkflowAttention["state"],
  runs: WorkflowAttentionRun[] = [],
  overrides: Partial<WorkflowAttention> = {},
): WorkflowAttention {
  return {
    state,
    head_sha: HEAD_SHA,
    observed_at: OBSERVED_AT,
    stale: false,
    runs,
    ...overrides,
  };
}

const approvalRun: WorkflowAttentionRun = {
  run_id: 7,
  run_attempt: 1,
  workflow_id: 9,
  name: "Run tests",
  url: "https://github.com/acme/widget/actions/runs/7",
  reason: "approval_required",
};

describe("workflow attention status summaries", () => {
  it("shows the maintainer approval reason when no check runs exist", () => {
    const summary = derivePRTaskStatusSummary(
      makeSummaryPR({
        head_sha: HEAD_SHA,
        checks_state: "unstable",
        workflow_attention: makeAttention("approval_required", [approvalRun]),
      }),
      false,
    );

    expect(summary.rows).toEqual([
      {
        kind: "ci",
        id: WORKFLOW_ATTENTION_ID,
        status: "awaiting_approval",
        tone: "warning",
        detail: {
          key: "github:workflowAttentionWorkflows",
          values: { names: "Run tests" },
        },
      },
    ]);
  });

  it("keeps an actual failed check beside workflow attention", () => {
    const summary = derivePRTaskStatusSummary(
      makeSummaryPR({
        head_sha: HEAD_SHA,
        checks_state: "failure",
        workflow_attention: makeAttention("action_required", [], { stale: true }),
      }),
      false,
    );

    expect(summary.rows).toEqual([
      { kind: "ci", status: "failed", tone: "danger" },
      {
        kind: "ci",
        id: WORKFLOW_ATTENTION_ID,
        status: "workflow_attention",
        tone: "warning",
      },
    ]);
  });

  it("explains an unexplained unstable check state without claiming approval", () => {
    const summary = derivePRTaskStatusSummary(makeSummaryPR({ checks_state: "unstable" }), false);

    expect(summary.rows).toEqual([
      { kind: "ci", status: "checks_not_successful", tone: "warning" },
    ]);
  });
});

describe("workflow attention fallbacks", () => {
  it("does not expose unstable mergeability when workflow approval explains it", () => {
    const summary = derivePRTaskStatusSummary(
      makeSummaryPR({
        head_sha: HEAD_SHA,
        mergeable_state: "unstable",
        workflow_attention: makeAttention("approval_required"),
      }),
      false,
    );

    expect(summary.rows).toEqual([
      { kind: "ci", id: WORKFLOW_ATTENTION_ID, status: "awaiting_approval", tone: "warning" },
    ]);
  });

  it("localizes unexplained unstable mergeability as an unsuccessful check", () => {
    const summary = derivePRTaskStatusSummary(
      makeSummaryPR({ mergeable_state: "unstable" }),
      false,
    );

    expect(summary.rows).toEqual([
      { kind: "ci", status: "checks_not_successful", tone: "warning" },
    ]);
  });

  it("keeps unavailable workflow evidence distinct from missing checks", () => {
    const summary = derivePRTaskStatusSummary(
      makeSummaryPR({
        head_sha: HEAD_SHA,
        workflow_attention: makeAttention("unknown", [], { stale: true }),
      }),
      false,
    );

    expect(summary.rows).toEqual([
      { kind: "ci", id: WORKFLOW_ATTENTION_ID, status: "workflow_unavailable", tone: "muted" },
    ]);
  });

  it("keeps an unstable check warning beside unavailable workflow evidence", () => {
    const summary = derivePRTaskStatusSummary(
      makeSummaryPR({
        head_sha: HEAD_SHA,
        checks_state: "unstable",
        workflow_attention: makeAttention("unknown", [], { stale: true }),
      }),
      false,
    );

    expect(summary.rows).toEqual([
      { kind: "ci", status: "checks_not_successful", tone: "warning" },
      { kind: "ci", id: WORKFLOW_ATTENTION_ID, status: "workflow_unavailable", tone: "muted" },
    ]);
  });
});
