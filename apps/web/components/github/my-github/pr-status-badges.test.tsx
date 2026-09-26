import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";

import { PRStatusBadges } from "./pr-status-badges";
import type { GitHubPR, GitHubPRStatus } from "@/lib/types/github";

afterEach(cleanup);

function makePR(): GitHubPR {
  return {
    number: 1,
    title: "Test PR",
    url: "https://api.github.com/repos/o/r/pulls/1",
    html_url: "https://github.com/o/r/pull/1",
    state: "open",
    head_branch: "feature",
    base_branch: "main",
    author_login: "octocat",
    repo_owner: "o",
    repo_name: "r",
    draft: false,
    mergeable: true,
    additions: 1,
    deletions: 0,
    requested_reviewers: [],
    created_at: "2026-08-06T00:00:00Z",
    updated_at: "2026-08-06T00:00:00Z",
    merged_at: null,
    closed_at: null,
  };
}

function makeStatus(pendingReviewCount: number): GitHubPRStatus {
  return {
    pr: makePR(),
    review_state: "",
    checks_state: "",
    mergeable_state: "clean",
    review_count: 0,
    pending_review_count: pendingReviewCount,
    checks_total: 0,
    checks_passing: 0,
  };
}

describe("PRStatusBadges review chip tooltip", () => {
  // The tooltip used to be `pendingReviewS`, interpolating {{pending}} into a
  // hardcoded "review(s)". i18next cannot select a plural from a value that is
  // not `count`, so the parenthetical was the only way English could stay
  // truthful — and every translation inherited it. These two cases pin the
  // plural-aware replacement.
  // The query is deliberately loose and the assertion exact: a wrong plural then
  // fails on the value ("expected '1 pending reviews' to be '1 pending review'")
  // rather than on the lookup, which would only report a missing element.
  it("uses the singular form for exactly one pending review", () => {
    render(<PRStatusBadges pr={makePR()} status={makeStatus(1)} />);
    expect(screen.getByTitle(/pending review/).getAttribute("title")).toBe("1 pending review");
  });

  it("uses the plural form for more than one pending review", () => {
    render(<PRStatusBadges pr={makePR()} status={makeStatus(3)} />);
    expect(screen.getByTitle(/pending review/).getAttribute("title")).toBe("3 pending reviews");
  });
});
