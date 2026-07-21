import { describe, it, expect } from "vitest";
import { shouldHideApproveButton } from "./pr-detail-panel";
import type { TaskPR, PRFeedback, GitHubPR, PRReview } from "@/lib/types/github";

function makeTaskPR(overrides: Partial<TaskPR> = {}): TaskPR {
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

function makeGitHubPR(overrides: Partial<GitHubPR> = {}): GitHubPR {
  return {
    number: 1,
    title: "Test PR",
    url: "",
    html_url: "",
    state: "open",
    head_branch: "feat",
    base_branch: "main",
    author_login: "alice",
    repo_owner: "o",
    repo_name: "r",
    draft: false,
    mergeable: true,
    additions: 0,
    deletions: 0,
    ...overrides,
  } as GitHubPR;
}

function makeFeedback(
  overrides: {
    pr?: Partial<GitHubPR>;
    reviews?: PRReview[];
  } = {},
): PRFeedback {
  return {
    pr: makeGitHubPR(overrides.pr),
    reviews: overrides.reviews ?? [],
    comments: [],
    checks: [],
    has_issues: false,
  };
}

describe("shouldHideApproveButton", () => {
  it("hides when PR is closed", () => {
    expect(shouldHideApproveButton(makeTaskPR({ state: "closed" }), null, "bob")).toBe(true);
  });

  it("hides when PR is merged", () => {
    expect(shouldHideApproveButton(makeTaskPR({ state: "merged" }), null, "bob")).toBe(true);
  });

  // Regression: pre-fix this returned false (button shown), so the green
  // Approve button appeared on every PR — including the viewer's own — during
  // the brief window before /api/v1/github/status resolved client-side.
  it("hides when current user is unknown (status not loaded yet)", () => {
    expect(shouldHideApproveButton(makeTaskPR({ author_login: "alice" }), null, null)).toBe(true);
    expect(shouldHideApproveButton(makeTaskPR({ author_login: "alice" }), null, "")).toBe(true);
    expect(shouldHideApproveButton(makeTaskPR({ author_login: "alice" }), null, "   ")).toBe(true);
  });

  it("hides when current user authored the PR (case-insensitive)", () => {
    expect(shouldHideApproveButton(makeTaskPR({ author_login: "Alice" }), null, "alice")).toBe(
      true,
    );
    expect(shouldHideApproveButton(makeTaskPR({ author_login: "alice" }), null, "ALICE")).toBe(
      true,
    );
  });

  it("hides when current user has already approved", () => {
    const feedback = makeFeedback({
      pr: { author_login: "alice" },
      reviews: [
        {
          id: 1,
          author: "bob",
          author_avatar: "",
          state: "APPROVED",
          body: "",
          created_at: "",
        },
      ],
    });
    expect(shouldHideApproveButton(makeTaskPR({ author_login: "alice" }), feedback, "bob")).toBe(
      true,
    );
  });

  it("shows when current user is a different open reviewer", () => {
    expect(shouldHideApproveButton(makeTaskPR({ author_login: "alice" }), null, "bob")).toBe(false);
  });

  it("prefers feedback.pr.author_login over taskPR.author_login when both present", () => {
    // taskPR may be stale; live feedback wins. Here the stored author looks
    // like a different user but feedback says it's actually us — must hide.
    const feedback = makeFeedback({ pr: { author_login: "bob" } });
    expect(shouldHideApproveButton(makeTaskPR({ author_login: "alice" }), feedback, "bob")).toBe(
      true,
    );
  });
});
