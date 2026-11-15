import { describe, expect, it } from "vitest";
import type { TaskPR } from "@/lib/types/github";
import {
  aggregatePRCIState,
  getPRCIState,
  isAllSkippedCheckSummary,
  skippedCheckCount,
} from "./pr-ci-state";

function taskPR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "pr-1",
    workspace_id: "ws-1",
    task_id: "task-1",
    owner: "acme",
    repo: "widget",
    pr_number: 42,
    pr_url: "https://github.com/acme/widget/pull/42",
    pr_title: "CI bubble",
    head_branch: "feature/ci",
    base_branch: "main",
    author_login: "octocat",
    state: "open",
    review_state: "",
    checks_state: "",
    mergeable_state: "clean",
    review_count: 0,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 0,
    checks_passing: 0,
    additions: 0,
    deletions: 0,
    created_at: "2026-11-10T12:00:00Z",
    merged_at: null,
    closed_at: null,
    last_synced_at: null,
    updated_at: "2026-11-10T12:00:00Z",
    head_sha: "head-a",
    ...overrides,
  };
}

function withAttention(
  overrides: Partial<TaskPR>,
  attention: Partial<NonNullable<TaskPR["workflow_attention"]>>,
): TaskPR {
  return taskPR({
    head_sha: "head-a",
    ...overrides,
    workflow_attention: {
      state: "none",
      head_sha: "head-a",
      observed_at: "2026-11-10T12:00:00Z",
      stale: false,
      runs: [],
      ...attention,
    },
  });
}

describe("getPRCIState", () => {
  it("prioritizes failure over every other signal", () => {
    expect(getPRCIState(taskPR({ checks_state: "failure" }))).toBe("failed");
    expect(
      getPRCIState(withAttention({ checks_state: "failure" }, { state: "approval_required" })),
    ).toBe("failed");
  });

  it("shows approval even when no checks have started", () => {
    expect(getPRCIState(withAttention({}, { state: "approval_required" }))).toBe(
      "approval_required",
    );
  });

  it("keeps other workflow attention distinct from approval", () => {
    expect(getPRCIState(withAttention({}, { state: "action_required" }))).toBe("attention");
    expect(getPRCIState(taskPR({ checks_state: "unstable" }))).toBe("attention");
  });

  it("shows running work only from an explicit pending rollup", () => {
    expect(getPRCIState(taskPR({ checks_state: "pending" }))).toBe("in_progress");
    expect(getPRCIState(taskPR({ checks_state: "", checks_total: 4, checks_passing: 1 }))).toBe(
      "none",
    );
  });

  it("keeps successful, skipped-only, and unknown CI quiet", () => {
    expect(getPRCIState(taskPR({ checks_state: "success" }))).toBe("none");
    expect(getPRCIState(taskPR({ checks_state: "" }))).toBe("none");
    expect(
      getPRCIState(
        taskPR({
          check_summary: { head_sha: "head-a", observed: 2, skipped: 2 },
        }),
      ),
    ).toBe("none");
  });

  it("never infers CI from review, draft, queue, or mergeability", () => {
    expect(getPRCIState(taskPR({ review_state: "changes_requested" }))).toBe("none");
    expect(getPRCIState(taskPR({ mergeable_state: "draft" }))).toBe("none");
    expect(getPRCIState(taskPR({ mergeable_state: "dirty" }))).toBe("none");
    expect(getPRCIState(taskPR({ merge_queue_state: "queued" }))).toBe("none");
  });

  it("keeps draft CI independent from the muted main glyph", () => {
    expect(getPRCIState(taskPR({ mergeable_state: "draft", checks_state: "failure" }))).toBe(
      "failed",
    );
  });

  it("filters terminal pull requests", () => {
    expect(getPRCIState(taskPR({ state: "merged", checks_state: "failure" }))).toBe("none");
    expect(getPRCIState(taskPR({ state: "closed", checks_state: "pending" }))).toBe("none");
  });

  it("ignores workflow attention for a mismatched head", () => {
    expect(
      getPRCIState(
        withAttention({ head_sha: "head-b" }, { state: "approval_required", head_sha: "head-a" }),
      ),
    ).toBe("none");
  });
});

describe("aggregatePRCIState", () => {
  it("returns none with no identity when every PR is quiet", () => {
    expect(aggregatePRCIState([taskPR({ checks_state: "success" })])).toEqual({
      state: "none",
      stale: false,
    });
  });

  it("selects the highest-priority state over open PRs", () => {
    const result = aggregatePRCIState([
      taskPR({ id: "a", pr_number: 42, checks_state: "pending" }),
      taskPR({ id: "b", pr_number: 43, checks_state: "failure" }),
      taskPR({ id: "c", pr_number: 44, state: "merged", checks_state: "failure" }),
    ]);
    expect(result.state).toBe("failed");
    expect(result.identity).toEqual({ number: 43, repository: "acme/widget" });
  });

  it("prefers fresh attention over stale evidence for the same state", () => {
    const result = aggregatePRCIState([
      withAttention({ id: "a", pr_number: 42 }, { state: "approval_required", stale: true }),
      withAttention({ id: "b", pr_number: 43 }, { state: "approval_required" }),
    ]);
    expect(result.state).toBe("approval_required");
    expect(result.stale).toBe(false);
    expect(result.identity?.number).toBe(43);
  });

  it("breaks ties with deterministic PR identity ordering", () => {
    const result = aggregatePRCIState([
      withAttention({ id: "a", pr_number: 48 }, { state: "action_required" }),
      withAttention({ id: "b", pr_number: 47 }, { state: "action_required" }),
    ]);
    expect(result.identity?.number).toBe(47);
  });

  it("keeps all-terminal associations quiet", () => {
    expect(
      aggregatePRCIState([
        taskPR({ id: "a", pr_number: 42, state: "merged", checks_state: "failure" }),
        taskPR({ id: "b", pr_number: 43, state: "closed", checks_state: "pending" }),
      ]).state,
    ).toBe("none");
  });
});

describe("skipped metadata", () => {
  it("recognizes a nonempty all-skipped snapshot", () => {
    const pr = taskPR({ check_summary: { head_sha: "head-a", observed: 3, skipped: 3 } });
    expect(isAllSkippedCheckSummary(pr)).toBe(true);
    expect(skippedCheckCount(pr)).toBe(3);
  });

  it("does not treat mixed or empty snapshots as all skipped", () => {
    expect(
      isAllSkippedCheckSummary(
        taskPR({ check_summary: { head_sha: "head-a", observed: 3, skipped: 2 } }),
      ),
    ).toBe(false);
    expect(
      isAllSkippedCheckSummary(
        taskPR({ check_summary: { head_sha: "head-a", observed: 0, skipped: 0 } }),
      ),
    ).toBe(false);
  });

  it("ignores malformed or wrong-head metadata", () => {
    expect(
      isAllSkippedCheckSummary(
        taskPR({ check_summary: { head_sha: "head-b", observed: 2, skipped: 2 } }),
      ),
    ).toBe(false);
    expect(
      isAllSkippedCheckSummary(
        taskPR({ check_summary: { head_sha: "head-a", observed: 1, skipped: 2 } }),
      ),
    ).toBe(false);
    expect(skippedCheckCount(taskPR({ check_summary: null }))).toBe(0);
  });
});
