import { describe, expect, it } from "vitest";
import type { TaskPR } from "@/lib/types/github";
import { derivePRTaskStatusSummary } from "./pr-task-status-summary";

const SKIPPED_COUNT_DETAIL_KEY = "github:checksSkippedCount";

function makePR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "pr-id",
    workspace_id: "workspace-1",
    task_id: "task-id",
    owner: "kdlbs",
    repo: "kandev",
    pr_number: 2966,
    pr_url: "https://github.com/kdlbs/kandev/pull/2966",
    pr_title: "Make pull request status easier to scan",
    head_branch: "feat/readable-pr-summary",
    base_branch: "main",
    author_login: "octocat",
    state: "open",
    review_state: "",
    checks_state: "",
    mergeable_state: "",
    review_count: 0,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 0,
    checks_passing: 0,
    additions: 0,
    deletions: 0,
    created_at: "",
    merged_at: null,
    closed_at: null,
    last_synced_at: null,
    updated_at: "",
    ...overrides,
  };
}

describe("structured PR task status summary", () => {
  it("derives separate review, CI, and ready-to-merge rows", () => {
    const pr = makePR({
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "clean",
    });

    const summary = derivePRTaskStatusSummary(pr, true);

    expect(summary).toEqual({
      number: 2966,
      title: "Make pull request status easier to scan",
      author: "octocat",
      rows: [
        { kind: "review", status: "approved", tone: "success" },
        { kind: "ci", status: "passed", tone: "success" },
        { kind: "merge", status: "ready", tone: "success" },
      ],
    });
  });

  it("preserves unknown provider values while omitting missing rows", () => {
    const summary = derivePRTaskStatusSummary(
      makePR({
        review_state: "queued_for_review" as unknown as TaskPR["review_state"],
        checks_state: "",
        mergeable_state: "unknown",
      }),
      false,
    );

    expect(summary.rows).toEqual([
      {
        kind: "review",
        status: "raw",
        tone: "muted",
        rawValue: "queued_for_review",
      },
    ]);
  });

  it("uses a terminal state row and omits mergeability for merged PRs", () => {
    const summary = derivePRTaskStatusSummary(
      makePR({
        state: "merged",
        review_state: "approved",
        checks_state: "success",
        mergeable_state: "clean",
      }),
      false,
    );

    expect(summary.rows).toEqual([
      { kind: "state", status: "merged", tone: "merged" },
      { kind: "review", status: "approved", tone: "success" },
      { kind: "ci", status: "passed", tone: "success" },
    ]);
  });

  it("shows blocked as a wait instead of claiming the PR is ready", () => {
    const summary = derivePRTaskStatusSummary(
      makePR({
        review_state: "approved",
        checks_state: "success",
        mergeable_state: "blocked",
      }),
      false,
    );

    expect(summary.rows.at(-1)).toEqual({
      kind: "merge",
      status: "blocked",
      tone: "muted",
    });
  });

  it("maps requested changes, failed CI, and conflicts to danger rows", () => {
    const summary = derivePRTaskStatusSummary(
      makePR({
        review_state: "changes_requested",
        checks_state: "failure",
        mergeable_state: "dirty",
      }),
      false,
    );

    expect(summary.rows).toEqual([
      { kind: "review", status: "changes_requested", tone: "danger" },
      { kind: "ci", status: "failed", tone: "danger" },
      { kind: "merge", status: "conflicts", tone: "danger" },
    ]);
  });

  it("keeps draft precedence over a supplied ready state", () => {
    const summary = derivePRTaskStatusSummary(makePR({ mergeable_state: "draft" }), true);

    expect(summary.rows).toEqual([{ kind: "merge", status: "draft", tone: "muted" }]);
  });

  it("omits an empty author identity", () => {
    const summary = derivePRTaskStatusSummary(makePR({ author_login: "  " }), false);

    expect(summary).not.toHaveProperty("author");
  });
});

describe("queued PR task status summaries", () => {
  it("replaces mergeability with queue semantics and metadata", () => {
    const summary = derivePRTaskStatusSummary(
      makePR({
        merge_queue_state: "awaiting_checks",
        merge_queue_position: 3,
        merge_queue_estimated_time_to_merge_seconds: 61,
        mergeable_state: "dirty",
      }),
      false,
    );

    expect(summary.rows.at(-1)).toEqual({
      kind: "merge",
      status: "queue_awaiting_checks",
      tone: "queued",
      detail: {
        key: "github:mergeQueuePositionAndEstimate",
        values: { position: 3, count: 2 },
      },
    });
  });

  it("uses generic queued copy for an unknown provider enum", () => {
    const summary = derivePRTaskStatusSummary(
      makePR({
        merge_queue_state: "future_provider_state",
        merge_queue_position: 1,
      }),
      false,
    );

    expect(summary.rows.at(-1)).toEqual({
      kind: "merge",
      status: "queue_queued",
      tone: "queued",
      detail: {
        key: "github:mergeQueuePosition",
        values: { position: 1 },
      },
    });
  });

  it("does not surface queue metadata for a terminal PR", () => {
    const summary = derivePRTaskStatusSummary(
      makePR({
        state: "merged",
        merge_queue_state: "queued",
        merge_queue_position: 1,
      }),
      false,
    );

    expect(summary.rows).not.toContainEqual(expect.objectContaining({ tone: "queued" }));
  });
});

describe("skipped check explanations", () => {
  it("shows a muted all-skipped row without claiming success", () => {
    const summary = derivePRTaskStatusSummary(
      makePR({
        head_sha: "head-a",
        check_summary: { head_sha: "head-a", observed: 3, skipped: 3 },
      }),
      false,
    );

    expect(summary.rows).toContainEqual({
      kind: "ci",
      status: "all_skipped",
      tone: "muted",
      detail: { key: "github:lastDetailedObservation" },
    });
    expect(summary.rows).not.toContainEqual(expect.objectContaining({ status: "passed" }));
  });

  it("keeps skipped counts separate from passed counts", () => {
    const summary = derivePRTaskStatusSummary(
      makePR({
        head_sha: "head-a",
        checks_state: "success",
        check_summary: { head_sha: "head-a", observed: 4, skipped: 2 },
      }),
      false,
    );

    expect(summary.rows).toContainEqual({
      kind: "ci",
      status: "passed",
      tone: "success",
      detail: { key: SKIPPED_COUNT_DETAIL_KEY, values: { count: 2 } },
    });
  });

  it("keeps failure primary beside skipped counts", () => {
    const summary = derivePRTaskStatusSummary(
      makePR({
        head_sha: "head-a",
        checks_state: "failure",
        check_summary: { head_sha: "head-a", observed: 3, skipped: 1 },
      }),
      false,
    );

    expect(summary.rows).toContainEqual({
      kind: "ci",
      status: "failed",
      tone: "danger",
      detail: { key: SKIPPED_COUNT_DETAIL_KEY, values: { count: 1 } },
    });
  });

  it("does not treat skipped plus cancelled as all skipped", () => {
    const summary = derivePRTaskStatusSummary(
      makePR({
        head_sha: "head-a",
        check_summary: { head_sha: "head-a", observed: 2, skipped: 1 },
      }),
      false,
    );

    expect(summary.rows).not.toContainEqual(expect.objectContaining({ status: "all_skipped" }));
    expect(summary.rows).not.toContainEqual(expect.objectContaining({ status: "passed" }));
  });

  it("ignores stale metadata from a previous head", () => {
    const summary = derivePRTaskStatusSummary(
      makePR({
        head_sha: "head-b",
        check_summary: { head_sha: "head-a", observed: 2, skipped: 2 },
      }),
      false,
    );

    expect(summary.rows).not.toContainEqual(expect.objectContaining({ status: "all_skipped" }));
  });
});

describe("draft conflict precedence", () => {
  it("keeps a merge conflict visible beside draft CI", () => {
    const summary = derivePRTaskStatusSummary(
      makePR({
        mergeable_state: "draft",
        has_merge_conflicts: true,
        checks_state: "failure",
      }),
      false,
    );

    expect(summary.rows).toContainEqual({ kind: "ci", status: "failed", tone: "danger" });
    expect(summary.rows).toContainEqual({ kind: "merge", status: "conflicts", tone: "danger" });
    expect(summary.rows).not.toContainEqual(expect.objectContaining({ status: "draft" }));
  });
});

describe("retained all-skipped history beside newer live CI", () => {
  const allSkipped = { head_sha: "head-a", observed: 3, skipped: 3 };

  it.each([
    {
      label: "failure",
      checksState: "failure" as const,
      expected: { kind: "ci" as const, status: "failed" as const, tone: "danger" as const },
    },
    {
      label: "pending",
      checksState: "pending" as const,
      expected: { kind: "ci" as const, status: "in_progress" as const, tone: "warning" as const },
    },
    {
      label: "unstable",
      checksState: "unstable" as const,
      expected: {
        kind: "ci" as const,
        status: "checks_not_successful" as const,
        tone: "warning" as const,
      },
    },
  ])(
    "keeps newer $label primary and qualifies the skipped history",
    ({ checksState, expected }) => {
      const summary = derivePRTaskStatusSummary(
        makePR({ head_sha: "head-a", checks_state: checksState, check_summary: allSkipped }),
        false,
      );

      expect(summary.rows).toContainEqual({
        ...expected,
        detail: { key: "github:lastDetailedObservationAllSkipped" },
      });
      expect(summary.rows).not.toContainEqual(expect.objectContaining({ status: "all_skipped" }));
    },
  );

  it("keeps a newer successful rollup primary without claiming mixed skipped checks", () => {
    const summary = derivePRTaskStatusSummary(
      makePR({ head_sha: "head-a", checks_state: "success", check_summary: allSkipped }),
      false,
    );

    expect(summary.rows).toContainEqual({ kind: "ci", status: "passed", tone: "success" });
    expect(summary.rows).not.toContainEqual(expect.objectContaining({ status: "all_skipped" }));
    expect(summary.rows).not.toContainEqual(
      expect.objectContaining({
        detail: expect.objectContaining({ key: SKIPPED_COUNT_DETAIL_KEY }),
      }),
    );
  });

  it("keeps a mixed skipped count beside a newer failure", () => {
    const summary = derivePRTaskStatusSummary(
      makePR({
        head_sha: "head-a",
        checks_state: "failure",
        check_summary: { head_sha: "head-a", observed: 4, skipped: 2 },
      }),
      false,
    );

    expect(summary.rows).toContainEqual({
      kind: "ci",
      status: "failed",
      tone: "danger",
      detail: { key: SKIPPED_COUNT_DETAIL_KEY, values: { count: 2 } },
    });
  });
});
