import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render } from "@testing-library/react";
import type { TaskPR } from "@/lib/types/github";
import { aggregatePRStatusColor, getPRStatusColor } from "./pr-task-icon";
import {
  describeMergeQueueEstimate,
  getMergeQueueSummaryStatus,
  PRMergeQueueStatus,
} from "./pr-merge-queue-status";

const QUEUE_STATUS_TEST_ID = "pr-merge-queue-status";

function makePR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "id",
    task_id: "task-1",
    owner: "acme",
    repo: "demo",
    pr_number: 42,
    pr_url: "",
    pr_title: "Queued PR",
    head_branch: "feature",
    base_branch: "main",
    author_login: "alice",
    state: "open",
    review_state: "",
    checks_state: "success",
    mergeable_state: "blocked",
    review_count: 0,
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
    last_synced_at: null,
    updated_at: "",
    ...overrides,
  };
}

afterEach(() => cleanup());

describe("merge queue estimate formatting", () => {
  it.each([
    [undefined, { kind: "none" }],
    [null, { kind: "none" }],
    [0, { kind: "sub_minute" }],
    [59, { kind: "sub_minute" }],
    [60, { kind: "minutes", minutes: 1 }],
    [61, { kind: "minutes", minutes: 2 }],
    [3600, { kind: "minutes", minutes: 60 }],
  ])("describes %s seconds without exposing raw provider units", (seconds, expected) => {
    expect(describeMergeQueueEstimate(seconds)).toEqual(expected);
  });
});

describe("merge queue status", () => {
  it.each([
    ["queued", "queue_queued"],
    ["awaiting_checks", "queue_awaiting_checks"],
    ["mergeable", "queue_mergeable"],
    ["unmergeable", "queue_unmergeable"],
    ["locked", "queue_locked"],
    ["future_state", "queue_queued"],
  ])("maps %s to %s", (state, expected) => {
    expect(getMergeQueueSummaryStatus(state)).toBe(expected);
  });

  it("uses generic queued copy for a future provider state", () => {
    const { getByTestId } = render(
      <PRMergeQueueStatus pr={makePR({ merge_queue_state: "future_state" })} />,
    );

    expect(getByTestId(QUEUE_STATUS_TEST_ID).textContent).toContain("Merge queue: Queued");
  });

  it("renders queue state, one-based position, and an available estimate", () => {
    const { getByTestId } = render(
      <PRMergeQueueStatus
        pr={makePR({
          merge_queue_state: "queued",
          merge_queue_position: 4,
          merge_queue_estimated_time_to_merge_seconds: 61,
        })}
      />,
    );

    const notice = getByTestId(QUEUE_STATUS_TEST_ID);
    expect(notice.textContent).toContain("Merge queue: Queued");
    expect(notice.textContent).toContain("Position 4");
    expect(notice.textContent).toContain("2 minutes");
  });

  it("renders a localized sub-minute estimate", () => {
    const { getByTestId } = render(
      <PRMergeQueueStatus
        pr={makePR({
          merge_queue_state: "awaiting_checks",
          merge_queue_position: 1,
          merge_queue_estimated_time_to_merge_seconds: 59,
        })}
      />,
    );

    expect(getByTestId(QUEUE_STATUS_TEST_ID).textContent).toContain("less than a minute");
  });

  it("renders nothing for an empty or terminal queue state", () => {
    const empty = render(<PRMergeQueueStatus pr={makePR()} />);
    expect(empty.queryByTestId(QUEUE_STATUS_TEST_ID)).toBeNull();
    empty.unmount();

    const terminal = render(
      <PRMergeQueueStatus pr={makePR({ state: "closed", merge_queue_state: "queued" })} />,
    );
    expect(terminal.queryByTestId(QUEUE_STATUS_TEST_ID)).toBeNull();
  });

  it("keeps terminal colors ahead of a stale queue entry", () => {
    expect(getPRStatusColor(makePR({ state: "merged", merge_queue_state: "queued" }))).toBe(
      "text-purple-500",
    );
    expect(getPRStatusColor(makePR({ state: "closed", merge_queue_state: "queued" }))).toBe(
      "text-red-500",
    );
  });

  it("keeps queue color ahead of other non-terminal PR states", () => {
    const overrides: Array<Partial<TaskPR>> = [
      { checks_state: "failure" },
      { review_state: "changes_requested" },
      { mergeable_state: "dirty" },
      { mergeable_state: "behind" },
      { mergeable_state: "draft" },
    ];
    for (const override of overrides) {
      expect(getPRStatusColor(makePR({ merge_queue_state: "queued", ...override }))).toBe(
        "text-[#966600]",
      );
    }
  });

  it("lets a failing sibling outrank a queued PR in the aggregate", () => {
    expect(
      aggregatePRStatusColor([
        makePR({ merge_queue_state: "queued" }),
        makePR({ pr_number: 2, checks_state: "failure" }),
      ]),
    ).toBe("text-red-500");
  });
});
