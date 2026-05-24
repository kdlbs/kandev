import { describe, it, expect } from "vitest";
import type { GitHubPR, GitHubPRStatus } from "@/lib/types/github";
import { pickPRForLaunch } from "./pr-list";

function makePR(overrides?: Partial<GitHubPR>): GitHubPR {
  return {
    number: 42,
    title: "Test PR",
    url: "https://github.com/owner/repo/pull/42",
    html_url: "https://github.com/owner/repo/pull/42",
    state: "open",
    head_branch: "",
    base_branch: "",
    author_login: "author",
    repo_owner: "owner",
    repo_name: "repo",
    draft: false,
    mergeable: true,
    additions: 0,
    deletions: 0,
    requested_reviewers: [],
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    merged_at: null,
    closed_at: null,
    ...overrides,
  };
}

function makeStatus(pr: GitHubPR): GitHubPRStatus {
  return {
    pr,
    review_state: "",
    checks_state: "",
    mergeable_state: "unknown",
    review_count: 0,
    pending_review_count: 0,
    checks_total: 0,
    checks_passing: 0,
  };
}

describe("pickPRForLaunch", () => {
  it("prefers the enriched PR from the batched status (which carries head/base branches)", () => {
    const rawSearchPR = makePR({ head_branch: "", base_branch: "" });
    const enrichedPR = makePR({ head_branch: "feature/x", base_branch: "main" });

    const result = pickPRForLaunch(rawSearchPR, makeStatus(enrichedPR));

    expect(result.head_branch).toBe("feature/x");
    expect(result.base_branch).toBe("main");
  });

  it("falls back to the raw PR when no status is loaded yet", () => {
    const rawPR = makePR({ head_branch: "feature/raw", base_branch: "main" });

    expect(pickPRForLaunch(rawPR, undefined).head_branch).toBe("feature/raw");
    expect(pickPRForLaunch(rawPR, null).head_branch).toBe("feature/raw");
  });
});
