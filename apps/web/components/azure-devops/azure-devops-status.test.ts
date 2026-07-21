import { describe, expect, it } from "vitest";
import { getAzureDevOpsPullRequestPresentation } from "./azure-devops-status";
import type { AzureDevOpsTaskPullRequest } from "@/lib/types/azure-devops";

function taskPullRequest(
  overrides: Partial<AzureDevOpsTaskPullRequest> = {},
): AzureDevOpsTaskPullRequest {
  return {
    id: "link-1",
    taskId: "task-1",
    repositoryId: "repo-1",
    organizationUrl: "https://dev.azure.com/acme",
    projectId: "project-1",
    azureRepositoryId: "azure-repo-1",
    pullRequestId: 42,
    pullRequestUrl: "https://dev.azure.com/acme/project/_git/repo/pullrequest/42",
    title: "Ship integration",
    sourceBranch: "refs/heads/feature",
    targetBranch: "refs/heads/main",
    authorId: "user-1",
    authorName: "Alice",
    status: "active",
    isDraft: false,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

describe("getAzureDevOpsPullRequestPresentation", () => {
  it("prioritizes terminal Azure states", () => {
    expect(getAzureDevOpsPullRequestPresentation(taskPullRequest({ status: "completed" }))).toEqual(
      { provider: "azure_devops", label: "Completed", tone: "success" },
    );
    expect(getAzureDevOpsPullRequestPresentation(taskPullRequest({ status: "abandoned" }))).toEqual(
      { provider: "azure_devops", label: "Abandoned", tone: "muted" },
    );
  });

  it("surfaces policy failures ahead of review state", () => {
    expect(
      getAzureDevOpsPullRequestPresentation(
        taskPullRequest({ policyState: "failure", reviewState: "approved" }),
      ),
    ).toEqual({ provider: "azure_devops", label: "Policy failed", tone: "danger" });
  });

  it("distinguishes drafts, review waits, and ready pull requests", () => {
    expect(getAzureDevOpsPullRequestPresentation(taskPullRequest({ isDraft: true })).label).toBe(
      "Draft",
    );
    expect(
      getAzureDevOpsPullRequestPresentation(taskPullRequest({ reviewState: "waiting" })).label,
    ).toBe("Waiting for review");
    expect(
      getAzureDevOpsPullRequestPresentation(
        taskPullRequest({ reviewState: "approved", policyState: "success" }),
      ),
    ).toEqual({ provider: "azure_devops", label: "Ready", tone: "success" });
  });
});
