import { describe, expect, it } from "vitest";
import type { TaskPR } from "@/lib/types/github";
import type { TaskPRInfo } from "./pr-task-automation";
import { derivePRTaskStatusSummary } from "./pr-task-status-summary";
import {
  compactWorkflowApprovalIsNewerThanFullPRs,
  getNegativeWorkflowApprovalDisclosure,
} from "./pr-task-workflow-projection";

const SUMMARY_UPDATED_AT = "2026-09-30T12:00:00Z";
const FULL_SYNCED_AT = "2026-09-30T11:00:00Z";
const FIRST_REPOSITORY = "org/first";
const SECOND_REPOSITORY = "org/second";

function makePR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "pr-1",
    workspace_id: "workspace-1",
    task_id: "task-1",
    repository_id: "repository-1",
    owner: "org",
    repo: "repo",
    pr_number: 42,
    pr_url: "",
    pr_title: "Test PR",
    head_branch: "feature",
    base_branch: "main",
    author_login: "alice",
    state: "open",
    review_state: "approved",
    checks_state: "success",
    mergeable_state: "clean",
    review_count: 1,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 1,
    checks_passing: 1,
    additions: 0,
    deletions: 0,
    created_at: "",
    merged_at: null,
    closed_at: null,
    last_synced_at: FULL_SYNCED_AT,
    updated_at: FULL_SYNCED_AT,
    ...overrides,
  };
}

function makePRInfo(overrides: Partial<TaskPRInfo> = {}): TaskPRInfo {
  return {
    number: 42,
    state: "open",
    statusSummaryUpdatedAt: SUMMARY_UPDATED_AT,
    workflowApprovalRequired: false,
    ...overrides,
  };
}

function summariesFor(prs: TaskPR[]) {
  return prs.map((pr) => derivePRTaskStatusSummary(pr, false));
}

describe("compact task workflow projection freshness", () => {
  it.each([true, false])("accepts a newer explicit approval value: %s", (required) => {
    expect(
      compactWorkflowApprovalIsNewerThanFullPRs(
        [makePR()],
        makePRInfo({
          workflowApprovalRequired: required,
        }),
      ),
    ).toBe(true);
  });

  it.each([
    { label: "missing approval value", info: { workflowApprovalRequired: undefined } },
    { label: "equal timestamp", info: { statusSummaryUpdatedAt: FULL_SYNCED_AT } },
    { label: "older timestamp", info: { statusSummaryUpdatedAt: "2026-09-30T10:00:00Z" } },
    { label: "malformed timestamp", info: { statusSummaryUpdatedAt: "tomorrow" } },
  ])("keeps full evidence authoritative for $label", ({ info }) => {
    expect(compactWorkflowApprovalIsNewerThanFullPRs([makePR()], makePRInfo(info))).toBe(false);
  });

  it("keeps newer full workflow evidence authoritative", () => {
    const pr = makePR({
      workflow_attention: {
        state: "approval_required",
        head_sha: "head-a",
        observed_at: "2026-09-30T13:00:00Z",
        stale: false,
        runs: [],
      },
    });
    expect(compactWorkflowApprovalIsNewerThanFullPRs([pr], makePRInfo())).toBe(false);
  });
});

describe("negative workflow approval disclosure", () => {
  it("retains every linked PR and independent status rows while removing approval", () => {
    const first = makePR({
      head_sha: "head-a",
      workflow_attention: {
        state: "approval_required",
        head_sha: "head-a",
        observed_at: "2026-09-30T10:30:00Z",
        stale: true,
        runs: [],
      },
    });
    const second = makePR({
      id: "pr-2",
      pr_number: 43,
      pr_title: "Merged sibling",
      state: "merged",
      checks_state: "pending",
    });
    const projection = getNegativeWorkflowApprovalDisclosure(
      [first, second],
      summariesFor([first, second]),
      makePRInfo({ count: 2 }),
    );

    expect(projection.count).toBe(2);
    expect(
      projection.summaries.map(({ number, title, author }) => ({ number, title, author })),
    ).toEqual([
      { number: 42, title: "Test PR", author: "alice" },
      { number: 43, title: "Merged sibling", author: "alice" },
    ]);
    const rows = projection.summaries.flatMap((summary) => summary.rows);
    expect(rows.map((row) => row.status)).toEqual([
      "approved",
      "passed",
      "mergeable",
      "merged",
      "approved",
      "in_progress",
    ]);
    expect(rows.some((row) => row.status === "awaiting_approval")).toBe(false);
  });

  it("preserves action-required and unavailable workflow evidence", () => {
    const actionRequired = makePR({
      head_sha: "head-a",
      workflow_attention: {
        state: "action_required",
        head_sha: "head-a",
        observed_at: "2026-09-30T10:30:00Z",
        stale: false,
        runs: [],
      },
    });
    const unavailable = makePR({
      id: "pr-2",
      pr_number: 43,
      head_sha: "head-b",
      workflow_attention: {
        state: "unknown",
        head_sha: "head-b",
        observed_at: "2026-09-30T10:30:00Z",
        stale: true,
        runs: [],
      },
    });
    const projection = getNegativeWorkflowApprovalDisclosure(
      [actionRequired, unavailable],
      summariesFor([actionRequired, unavailable]),
      makePRInfo({ count: 2 }),
    );
    const rows = projection.summaries.flatMap((summary) => summary.rows);

    expect(rows.map((row) => row.status)).toContain("workflow_attention");
    expect(rows.map((row) => row.status)).toContain("workflow_unavailable");
  });
});

describe("negative workflow conflict and state projection", () => {
  it("attributes a compact conflict by repository when PR numbers collide", () => {
    const first = makePR({
      id: "first-42",
      repository_id: "repository-first",
      repo: "first",
      pr_title: "First PR",
      mergeable_state: "clean",
    });
    const second = makePR({
      id: "second-42",
      repository_id: "repository-second",
      repo: "second",
      pr_title: "Second PR",
      mergeable_state: "clean",
    });
    const projection = getNegativeWorkflowApprovalDisclosure(
      [first, second],
      summariesFor([first, second]),
      makePRInfo({ hasMergeConflicts: true, mergeConflictRepository: SECOND_REPOSITORY }),
    );

    const firstRows = projection.summaries[0].rows;
    const secondRows = projection.summaries[1].rows;
    expect(firstRows.some((row) => row.status === "conflicts")).toBe(false);
    expect(firstRows.some((row) => row.status === "mergeable")).toBe(true);
    const conflict = secondRows.find((row) => row.status === "conflicts");
    expect(conflict?.detail?.values).toEqual({ repository: SECOND_REPOSITORY, number: 42 });
    expect(secondRows.some((row) => row.status === "mergeable" || row.status === "ready")).toBe(
      false,
    );
    expect(projection.summaries.map((summary) => summary.title)).toEqual(["First PR", "Second PR"]);
  });

  it("keeps independent queue evidence when a newer conflict replaces merge readiness", () => {
    const queued = makePR({ merge_queue_state: "awaiting_checks" });
    const projection = getNegativeWorkflowApprovalDisclosure(
      [queued],
      summariesFor([queued]),
      makePRInfo({ hasMergeConflicts: true }),
    );

    const rows = projection.summaries[0].rows;
    expect(rows.some((row) => row.status === "queue_awaiting_checks")).toBe(true);
    expect(rows.some((row) => row.status === "conflicts")).toBe(true);
  });

  it("retains an attributed compact conflict when its full PR record is absent", () => {
    const linked = makePR({ pr_number: 41, pr_title: "Linked PR" });
    const projection = getNegativeWorkflowApprovalDisclosure(
      [linked],
      summariesFor([linked]),
      makePRInfo({
        hasMergeConflicts: true,
        mergeConflictPRNumber: 43,
        mergeConflictRepository: "org/conflict",
      }),
    );

    expect(projection.count).toBe(2);
    expect(projection.summaries[0].title).toBe("Linked PR");
    expect(projection.summaries[1]).toMatchObject({
      number: 43,
      title: "",
      rows: [
        {
          id: "merge-conflict",
          status: "conflicts",
          detail: { values: { repository: "org/conflict", number: 43 } },
        },
      ],
    });
    expect(projection.summaries[1].author).toBeUndefined();
  });

  it("uses compact merged state for its PR without overwriting sibling details", () => {
    const open = makePR({ state: "open" });
    const mergedProjection = getNegativeWorkflowApprovalDisclosure(
      [open],
      summariesFor([open]),
      makePRInfo({ state: "Merged" }),
    );
    expect(mergedProjection.summaries[0].rows[0]).toMatchObject({
      kind: "state",
      status: "merged",
      tone: "merged",
    });

    const mergedSibling = makePR({ id: "pr-43", pr_number: 43, state: "merged" });
    const mixedProjection = getNegativeWorkflowApprovalDisclosure(
      [open, mergedSibling],
      summariesFor([open, mergedSibling]),
      makePRInfo({ state: "Merged", count: 2 }),
    );
    expect(mixedProjection.summaries[0].rows.some((row) => row.status === "merged")).toBe(true);
    expect(mixedProjection.summaries[0].rows.filter((row) => row.kind === "merge")).toEqual([]);
    expect(mixedProjection.summaries[1].rows.some((row) => row.status === "merged")).toBe(true);
    expect(mixedProjection.summaries[1].title).toBe("Test PR");
  });
});

describe("negative workflow conflict repository reconciliation", () => {
  it("attributes a conflict to its repository when approval targets a same-number PR", () => {
    const approvalPR = makePR({
      id: "approval-42",
      repository_id: "repository-first",
      repo: "first",
      pr_title: "Approval PR",
      mergeable_state: "clean",
    });
    const conflictPR = makePR({
      id: "conflict-42",
      repository_id: "repository-second",
      repo: "second",
      pr_title: "Conflict PR",
      mergeable_state: "clean",
    });
    const projection = getNegativeWorkflowApprovalDisclosure(
      [approvalPR, conflictPR],
      summariesFor([approvalPR, conflictPR]),
      makePRInfo({
        workflowApprovalRequired: true,
        workflowApprovalPRNumber: 42,
        workflowApprovalRepository: FIRST_REPOSITORY,
        hasMergeConflicts: true,
        mergeConflictPRNumber: 42,
        mergeConflictRepository: SECOND_REPOSITORY,
      }),
    );

    const firstRows = projection.summaries[0].rows;
    const secondRows = projection.summaries[1].rows;
    expect(firstRows.some((row) => row.status === "conflicts")).toBe(false);
    expect(firstRows.some((row) => row.status === "mergeable")).toBe(true);
    expect(secondRows.some((row) => row.status === "conflicts")).toBe(true);
    expect(secondRows.some((row) => row.status === "mergeable")).toBe(false);
  });

  it("clears only the cached conflict attributed by a newer negative projection", () => {
    const first = makePR({
      id: "first-42",
      repository_id: "repository-first",
      repo: "first",
      pr_title: "First PR",
      mergeable_state: "dirty",
    });
    const second = makePR({
      id: "second-42",
      repository_id: "repository-second",
      repo: "second",
      pr_title: "Second PR",
      mergeable_state: "dirty",
    });
    const projection = getNegativeWorkflowApprovalDisclosure(
      [first, second],
      summariesFor([first, second]),
      makePRInfo({
        hasMergeConflicts: false,
        mergeConflictPRNumber: 42,
        mergeConflictRepository: FIRST_REPOSITORY,
        workflowApprovalPRNumber: 42,
        workflowApprovalRepository: SECOND_REPOSITORY,
      }),
    );

    expect(projection.summaries[0].rows.some((row) => row.status === "conflicts")).toBe(false);
    expect(projection.summaries[0].rows.some((row) => row.status === "approved")).toBe(true);
    expect(projection.summaries[0].rows.some((row) => row.status === "passed")).toBe(true);
    expect(projection.summaries[1].rows.some((row) => row.status === "conflicts")).toBe(true);
  });
});

describe("negative terminal lifecycle reconciliation", () => {
  it.each([
    { label: "merged", compactState: "Merged", readyToMerge: true },
    { label: "closed", compactState: "Closed", readyToMerge: false },
    { label: "merged while queued", compactState: "Merged", mergeQueueState: "queued" },
  ])("removes open-only merge rows when the PR is $label", (scenario) => {
    const pr = makePR({
      state: "open",
      mergeable_state: "clean",
      ...(scenario.mergeQueueState ? { merge_queue_state: scenario.mergeQueueState } : {}),
    });
    const projection = getNegativeWorkflowApprovalDisclosure(
      [pr],
      [derivePRTaskStatusSummary(pr, scenario.readyToMerge ?? false)],
      makePRInfo({ state: scenario.compactState }),
    );

    const rows = projection.summaries[0].rows;
    expect(rows.some((row) => row.status === scenario.compactState.toLowerCase())).toBe(true);
    expect(rows.filter((row) => row.kind === "merge")).toEqual([]);
    expect(rows.some((row) => row.status === "approved")).toBe(true);
    expect(rows.some((row) => row.status === "passed")).toBe(true);
  });

  it.each([
    {
      label: "merged",
      compactState: "Merged",
      readyToMerge: true,
      mergeQueueState: undefined,
    },
    {
      label: "closed",
      compactState: "Closed",
      readyToMerge: false,
      mergeQueueState: "awaiting_checks",
    },
  ])("applies the $label state to its matching PR among siblings", (scenario) => {
    const target = makePR({
      id: "target-42",
      pr_number: 42,
      pr_title: "Terminal target",
      state: "open",
      mergeable_state: "clean",
      ...(scenario.mergeQueueState ? { merge_queue_state: scenario.mergeQueueState } : {}),
    });
    const sibling = makePR({
      id: "sibling-43",
      pr_number: 43,
      pr_title: "Open sibling",
      state: "open",
      mergeable_state: "clean",
    });
    const projection = getNegativeWorkflowApprovalDisclosure(
      [target, sibling],
      [
        derivePRTaskStatusSummary(target, scenario.readyToMerge ?? false),
        derivePRTaskStatusSummary(sibling, false),
      ],
      makePRInfo({ state: scenario.compactState, count: 2 }),
    );

    const targetSummary = projection.summaries[0];
    const siblingSummary = projection.summaries[1];
    expect(targetSummary.title).toBe("Terminal target");
    expect(
      targetSummary.rows.some((row) => row.status === scenario.compactState.toLowerCase()),
    ).toBe(true);
    expect(targetSummary.rows.filter((row) => row.kind === "merge")).toEqual([]);
    expect(targetSummary.rows.some((row) => row.status === "approved")).toBe(true);
    expect(targetSummary.rows.some((row) => row.status === "passed")).toBe(true);
    expect(siblingSummary.title).toBe("Open sibling");
    expect(
      siblingSummary.rows.some((row) => row.status === "merged" || row.status === "closed"),
    ).toBe(false);
    expect(siblingSummary.rows.some((row) => row.status === "mergeable")).toBe(true);
  });
});
