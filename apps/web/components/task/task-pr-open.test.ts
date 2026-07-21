import { describe, it, expect } from "vitest";
import { resolveTaskPROpenAction, resolveTaskReviewOpenAction } from "./task-pr-open";
import type { TaskPR } from "@/lib/types/github";
import type { TaskMR } from "@/lib/types/gitlab";

function makePR(overrides: Partial<TaskPR>): TaskPR {
  return {
    id: "pr-1",
    task_id: "task-1",
    owner: "kdlbs",
    repo: "kandev",
    pr_number: 1,
    pr_url: "https://github.com/kdlbs/kandev/pull/1",
    pr_title: "Test PR",
    head_branch: "feature/x",
    base_branch: "main",
    author_login: "jcfs",
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
    created_at: "2026-01-01T00:00:00Z",
    merged_at: null,
    closed_at: null,
    last_synced_at: null,
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

describe("resolveTaskPROpenAction", () => {
  it("returns none when the task has no linked PRs", () => {
    expect(resolveTaskPROpenAction([])).toEqual({ kind: "none" });
  });

  it("opens directly when exactly one PR is linked", () => {
    const pr = makePR({ id: "only" });
    expect(resolveTaskPROpenAction([pr])).toEqual({ kind: "open", pr });
  });

  it("asks for a pick when several PRs are linked", () => {
    const prs = [makePR({ id: "a" }), makePR({ id: "b", pr_number: 2 })];
    expect(resolveTaskPROpenAction(prs)).toEqual({ kind: "pick", prs });
  });
});

describe("resolveTaskReviewOpenAction", () => {
  it("opens one GitLab merge request directly", () => {
    const mr = { id: "mr-1", mr_url: "https://gitlab.com/a/b/-/merge_requests/2" } as TaskMR;
    expect(resolveTaskReviewOpenAction([], [mr])).toEqual({
      kind: "open",
      url: mr.mr_url,
    });
  });

  it("uses the provider-aware picker for mixed linked reviews", () => {
    const pr = makePR({ id: "pr" });
    const mr = { id: "mr", mr_url: "https://gitlab.com/a/b/-/merge_requests/2" } as TaskMR;
    expect(resolveTaskReviewOpenAction([pr], [mr])).toEqual({ kind: "pick" });
  });
});
