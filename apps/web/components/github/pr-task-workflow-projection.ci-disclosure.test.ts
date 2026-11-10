import { describe, expect, it } from "vitest";
import type { TaskPR } from "@/lib/types/github";
import type { TaskPRInfo } from "./pr-task-automation";
import { derivePRTaskStatusSummary } from "./pr-task-status-summary";
import {
  applyCompactCIDisclosure,
  getCompactWorkflowStatusSummaries,
  getTaskPRIconDisclosureProjection,
} from "./pr-task-workflow-projection";

const SUMMARY_UPDATED_AT = "2026-09-30T12:00:00Z";
const FULL_SYNCED_AT = "2026-09-30T11:00:00Z";

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

const compactFailedRow = {
  kind: "ci",
  id: "ci-state",
  status: "failed",
  tone: "danger",
} as const;

describe("compact CI disclosure identity", () => {
  it("keeps repository attribution when two repositories share a PR number", () => {
    const prs = [
      makePR({ repository_id: "repo-first", owner: "org", repo: "first", pr_title: "First" }),
      makePR({ repository_id: "repo-second", owner: "org", repo: "second", pr_title: "Second" }),
    ];
    const result = applyCompactCIDisclosure(
      summariesFor(prs),
      prs,
      makePRInfo({ ciState: "failed", ciPRNumber: 42, ciRepository: "org/second" }),
      true,
    );
    expect(result).toHaveLength(2);
    expect(result[0].title).toBe("First");
    expect(result[0].rows).not.toContainEqual(expect.objectContaining({ status: "failed" }));
    expect(result[1].title).toBe("Second");
    expect(result[1].rows).toContainEqual(expect.objectContaining({ status: "failed" }));
  });

  it("does not guess a PR when a number is ambiguous without a repository", () => {
    const prs = [
      makePR({ repository_id: "repo-first", owner: "org", repo: "first", pr_title: "First" }),
      makePR({ repository_id: "repo-second", owner: "org", repo: "second", pr_title: "Second" }),
    ];
    const result = applyCompactCIDisclosure(
      summariesFor(prs),
      prs,
      makePRInfo({ ciState: "failed", ciPRNumber: 42 }),
      true,
    );
    expect(result[0].rows).not.toContainEqual(compactFailedRow);
    expect(result[1].rows).not.toContainEqual(compactFailedRow);
    expect(result).toContainEqual(
      expect.objectContaining({ number: 42, title: "", rows: [compactFailedRow] }),
    );
  });
});

describe("compact CI disclosure authority", () => {
  it("is a no-op without compact authority", () => {
    const prs = [makePR({ checks_state: "failure" })];
    const summaries = summariesFor(prs);
    expect(applyCompactCIDisclosure(summaries, prs, makePRInfo({ ciState: "none" }), false)).toBe(
      summaries,
    );
  });
});

describe("compact CI disclosure through the shared projection", () => {
  it.each([
    {
      full: "failure" as const,
      compact: "in_progress" as const,
      stale: "failed",
      next: "in_progress",
    },
    { full: "success" as const, compact: "failed" as const, stale: "passed", next: "failed" },
    {
      full: "failure" as const,
      compact: "approval_required" as const,
      stale: "failed",
      next: "awaiting_approval",
    },
  ])(
    "replaces a stale $stale outcome with the newer $next aggregate",
    ({ full, compact, stale, next }) => {
      const prs = [makePR({ checks_state: full })];
      const disclosure = getTaskPRIconDisclosureProjection(
        prs,
        summariesFor(prs),
        makePRInfo({ ciState: compact }),
        false,
        true,
      );
      expect(disclosure.summaries[0].rows).not.toContainEqual(
        expect.objectContaining({ status: stale }),
      );
      expect(disclosure.summaries[0].rows).toContainEqual(
        expect.objectContaining({ status: next }),
      );
    },
  );
});

describe("compact CI sibling reconciliation", () => {
  it("clears sibling check outcomes ruled out by a newer aggregate", () => {
    const prs = [
      makePR({
        repository_id: "repo-a",
        owner: "org",
        repo: "a",
        pr_number: 42,
        checks_state: "failure",
      }),
      makePR({
        repository_id: "repo-b",
        owner: "org",
        repo: "b",
        pr_number: 43,
        checks_state: "pending",
      }),
    ];
    const result = applyCompactCIDisclosure(
      summariesFor(prs),
      prs,
      makePRInfo({ ciState: "in_progress", ciPRNumber: 43, ciRepository: "org/b" }),
      true,
    );
    expect(result[0].rows).not.toContainEqual(expect.objectContaining({ status: "failed" }));
    expect(result[1].rows).toContainEqual(expect.objectContaining({ status: "in_progress" }));
  });
});

describe("compact approval row identity", () => {
  it("adds one approval row when approval and CI share the same PR identity", () => {
    const summaries = getCompactWorkflowStatusSummaries(
      makePRInfo({
        ciState: "approval_required",
        ciPRNumber: 42,
        ciRepository: "org/repo",
        workflowApprovalRequired: true,
        workflowApprovalPRNumber: 42,
        workflowApprovalRepository: "org/repo",
      }),
      true,
    );
    const approvalRows = summaries.flatMap((summary) =>
      summary.rows.filter((row) => row.status === "awaiting_approval"),
    );
    expect(approvalRows).toHaveLength(1);
  });

  it("keeps both approval rows when the PR identities differ", () => {
    const summaries = getCompactWorkflowStatusSummaries(
      makePRInfo({
        ciState: "approval_required",
        ciPRNumber: 43,
        ciRepository: "org/other",
        workflowApprovalRequired: true,
        workflowApprovalPRNumber: 42,
        workflowApprovalRepository: "org/repo",
      }),
      true,
    );
    const approvalRows = summaries.flatMap((summary) =>
      summary.rows.filter((row) => row.status === "awaiting_approval"),
    );
    expect(approvalRows).toHaveLength(2);
  });
});
