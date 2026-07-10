import { describe, expect, it } from "vitest";
import { deriveAggregateCounts, hasNoChecksAtAll } from "./pr-ci-popover";
import type { CheckRun, TaskPR } from "@/lib/types/github";

function makePR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "id",
    task_id: "task",
    owner: "o",
    repo: "r",
    pr_number: 1,
    pr_url: "",
    pr_title: "Test PR",
    head_branch: "feat",
    base_branch: "main",
    author_login: "alice",
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

describe("deriveAggregateCounts", () => {
  it("reserves failed segment when failure state has stale all-passing counts", () => {
    expect(
      deriveAggregateCounts(
        makePR({
          checks_state: "failure",
          checks_total: 20,
          checks_passing: 20,
        }),
      ),
    ).toEqual({ passed: 19, failed: 1, inProgress: 0 });
  });

  it("reserves failed segment when failure state has no populated counts yet", () => {
    expect(deriveAggregateCounts(makePR({ checks_state: "failure" }))).toEqual({
      passed: 0,
      failed: 1,
      inProgress: 0,
    });
  });

  it("clamps over-counted passing checks before reserving failed segment", () => {
    expect(
      deriveAggregateCounts(
        makePR({
          checks_state: "failure",
          checks_total: 5,
          checks_passing: 8,
        }),
      ),
    ).toEqual({ passed: 4, failed: 1, inProgress: 0 });
  });

  it("reserves in-progress segment when pending state has stale all-passing counts", () => {
    expect(
      deriveAggregateCounts(
        makePR({
          checks_state: "pending",
          checks_total: 20,
          checks_passing: 20,
        }),
      ),
    ).toEqual({ passed: 19, failed: 0, inProgress: 1 });
  });

  it("reserves in-progress segment when pending state has no populated counts yet", () => {
    expect(deriveAggregateCounts(makePR({ checks_state: "pending" }))).toEqual({
      passed: 0,
      failed: 0,
      inProgress: 1,
    });
  });

  it("uses total checks for success state", () => {
    expect(
      deriveAggregateCounts(
        makePR({
          checks_state: "success",
          checks_total: 7,
          checks_passing: 3,
        }),
      ),
    ).toEqual({ passed: 7, failed: 0, inProgress: 0 });
  });

  it("falls back to aggregate passing and remaining counts for unknown state", () => {
    expect(
      deriveAggregateCounts(
        makePR({
          checks_total: 5,
          checks_passing: 3,
        }),
      ),
    ).toEqual({ passed: 3, failed: 0, inProgress: 2 });
  });
});

describe("hasNoChecksAtAll", () => {
  it("does not hide failed status just because aggregate counts are zero", () => {
    expect(hasNoChecksAtAll(makePR({ checks_state: "failure" }), null, false)).toBe(false);
  });

  it("does not hide checks while feedback is fetching", () => {
    expect(hasNoChecksAtAll(makePR(), null, true)).toBe(false);
  });

  it("does not hide checks when aggregate total is populated", () => {
    expect(hasNoChecksAtAll(makePR({ checks_total: 3 }), null, false)).toBe(false);
  });

  it("does not hide checks when feedback has live checks", () => {
    expect(hasNoChecksAtAll(makePR(), { checks: [{} as CheckRun] }, false)).toBe(false);
  });

  it("hides checks only when status and counts are empty", () => {
    expect(hasNoChecksAtAll(makePR(), null, false)).toBe(true);
  });
});
