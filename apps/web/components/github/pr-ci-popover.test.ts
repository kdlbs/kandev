import { describe, expect, it } from "vitest";
import { deriveAggregateCounts, hasNoChecksAtAll } from "./pr-ci-popover";
import type { TaskPR } from "@/lib/types/github";

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
});

describe("hasNoChecksAtAll", () => {
  it("does not hide failed status just because aggregate counts are zero", () => {
    expect(hasNoChecksAtAll(makePR({ checks_state: "failure" }), null, false)).toBe(false);
  });

  it("hides checks only when status and counts are empty", () => {
    expect(hasNoChecksAtAll(makePR(), null, false)).toBe(true);
  });
});
