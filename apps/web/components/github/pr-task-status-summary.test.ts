import { describe, expect, it } from "vitest";
import type { TaskPR } from "@/lib/types/github";
import { derivePRTaskStatusSummary } from "./pr-task-status-summary";

function makePR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "pr-id",
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
