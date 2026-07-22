import { describe, expect, it } from "vitest";
import { resolvePRReviewRepositoryIdentity } from "./review-repository-identity";

const REPOSITORY_ID = "repo-widgets";
const REPOSITORY_NAME = "widgets";
const PRIMARY_BRANCH = "feat/first";
const SIBLING_BRANCH = "feat/second";

const primaryPR = {
  repository_id: REPOSITORY_ID,
  repo: REPOSITORY_NAME,
  head_branch: PRIMARY_BRANCH,
};

const siblingPR = {
  ...primaryPR,
  head_branch: SIBLING_BRANCH,
};

const taskRepositories = [
  {
    repository_id: REPOSITORY_ID,
    base_branch: "main",
    checkout_branch: PRIMARY_BRANCH,
    position: 0,
  },
  {
    repository_id: REPOSITORY_ID,
    base_branch: "main",
    checkout_branch: SIBLING_BRANCH,
    position: 1,
  },
];

const worktrees = [
  {
    repositoryId: REPOSITORY_ID,
    branchSlug: "feat-first",
    branch: PRIMARY_BRANCH,
    path: "/tasks/example/widgets",
    position: 0,
  },
  {
    repositoryId: REPOSITORY_ID,
    branchSlug: "feat-second",
    branch: SIBLING_BRANCH,
    path: "/tasks/example/widgets-feat-second",
    position: 1,
  },
];

describe("resolvePRReviewRepositoryIdentity", () => {
  it("uses the selected sibling worktree subpath for same-repo multi-branch review", () => {
    expect(
      resolvePRReviewRepositoryIdentity({
        pr: siblingPR,
        workspaceRepositoryName: REPOSITORY_NAME,
        taskRepositories,
        worktrees,
      }),
    ).toBe("widgets-feat-second");
  });

  it("keeps the flat worktree identity for the primary branch", () => {
    expect(
      resolvePRReviewRepositoryIdentity({
        pr: primaryPR,
        workspaceRepositoryName: REPOSITORY_NAME,
        taskRepositories,
        worktrees,
      }),
    ).toBe(REPOSITORY_NAME);
  });

  it("derives the sibling identity while live worktree metadata is still hydrating", () => {
    expect(
      resolvePRReviewRepositoryIdentity({
        pr: siblingPR,
        workspaceRepositoryName: REPOSITORY_NAME,
        taskRepositories,
        worktrees: [],
      }),
    ).toBe("widgets-feat-second");
  });
});
