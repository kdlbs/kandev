import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReviewFile } from "./types";

vi.mock("./review-diff-toolbar", () => ({
  FileDiffToolbar: (props: Record<string, unknown>) => (
    <span data-testid="review-toolbar-props" data-props={JSON.stringify(props)} />
  ),
}));

import { ReviewDiffHeader } from "./review-diff-header";

afterEach(cleanup);

const file: ReviewFile = {
  path: "src/app.ts",
  diff: "@@ -1 +1 @@",
  status: "modified",
  additions: 1,
  deletions: 1,
  staged: false,
  source: "pr",
  repository_id: "repo-2",
  repository_name: "frontend",
};

describe("ReviewDiffHeader external context", () => {
  it("keeps the exact repo base and rejects a published branch from another repo", () => {
    render(
      <ReviewDiffHeader
        file={file}
        isReviewed={false}
        isStale={false}
        sessionId="session-1"
        collapsed={false}
        wordWrap={false}
        expandUnchanged={false}
        baseBranchByRepo={{ frontend: "develop" }}
        taskId="task-1"
        publishedPRBranch="feature/other-repo"
        publishedPRRepositoryId="repo-1"
        onCheckboxChange={vi.fn()}
        onDiscard={vi.fn()}
        onToggleCollapse={vi.fn()}
        onToggleExpandUnchanged={vi.fn()}
        onToggleWordWrap={vi.fn()}
      />,
    );

    const props = JSON.parse(screen.getByTestId("review-toolbar-props").dataset.props ?? "{}");
    expect(props).toMatchObject({
      filePath: "src/app.ts",
      repositoryId: "repo-2",
      repo: "frontend",
      baseBranch: "develop",
      taskId: "task-1",
    });
    expect(props).not.toHaveProperty("publishedBranch");
  });

  it("does not inherit a published PR without a matching published repository", () => {
    render(
      <ReviewDiffHeader
        file={file}
        isReviewed={false}
        isStale={false}
        sessionId="session-1"
        collapsed={false}
        wordWrap={false}
        expandUnchanged={false}
        baseBranchByRepo={{ frontend: "develop" }}
        taskId="task-1"
        publishedPRBranch="feature/unknown-repository"
        onCheckboxChange={vi.fn()}
        onDiscard={vi.fn()}
        onToggleCollapse={vi.fn()}
        onToggleExpandUnchanged={vi.fn()}
        onToggleWordWrap={vi.fn()}
      />,
    );

    const props = JSON.parse(screen.getByTestId("review-toolbar-props").dataset.props ?? "{}");
    expect(props).not.toHaveProperty("publishedBranch");
    expect(props).not.toHaveProperty("publishedPullRequestNumber");
  });

  it("forwards the published branch without guessing a fork PR ref", () => {
    render(
      <ReviewDiffHeader
        file={{ ...file, repository_id: "repo-1" }}
        isReviewed={false}
        isStale={false}
        sessionId="session-1"
        collapsed={false}
        wordWrap={false}
        expandUnchanged={false}
        baseBranchByRepo={{ frontend: "main" }}
        taskId="task-1"
        publishedPRBranch="contributor:feature/share"
        publishedPRRepositoryId="repo-1"
        onCheckboxChange={vi.fn()}
        onDiscard={vi.fn()}
        onToggleCollapse={vi.fn()}
        onToggleExpandUnchanged={vi.fn()}
        onToggleWordWrap={vi.fn()}
      />,
    );

    const props = JSON.parse(screen.getByTestId("review-toolbar-props").dataset.props ?? "{}");
    expect(props).toMatchObject({
      publishedBranch: "contributor:feature/share",
    });
    expect(props).not.toHaveProperty("publishedPullRequestNumber");
  });
});
