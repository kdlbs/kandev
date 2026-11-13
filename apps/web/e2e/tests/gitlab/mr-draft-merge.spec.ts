import { test, expect } from "../../fixtures/test-base";
import { GITLAB_HOST, seedGitLabReview } from "../../helpers/gitlab";
import { GitLabPage } from "../../pages/gitlab-page";

const DRAFT_IID = 91;

test.describe("GitLab draft merge request", () => {
  test("a draft disables Merge with a hint, and Mark as ready then enables it", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await seedGitLabReview(apiClient, seedData.workspaceId, DRAFT_IID, "Draft: hold for review");
    await apiClient.mockGitLabAddMRs(seedData.workspaceId, "platform/kandev", [
      {
        id: DRAFT_IID + 10_000,
        iid: DRAFT_IID,
        project_id: 101,
        title: "Draft: hold for review",
        url: `${GITLAB_HOST}/platform/kandev/-/merge_requests/${DRAFT_IID}`,
        web_url: `${GITLAB_HOST}/platform/kandev/-/merge_requests/${DRAFT_IID}`,
        state: "opened",
        head_branch: "main",
        head_sha: `sha-${DRAFT_IID}`,
        base_branch: "main",
        author_username: "contributor",
        project_namespace: "platform",
        project_path: "platform/kandev",
        body: "Held on purpose",
        draft: true,
        merge_status: "can_be_merged",
        has_conflicts: false,
        additions: 1,
        deletions: 0,
        reviewers: [],
        assignees: [],
        labels: [],
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      },
    ]);
    await apiClient.updateRepository(seedData.repositoryId, {
      provider: "gitlab",
      provider_host: GITLAB_HOST,
      provider_owner: "platform",
      provider_name: "kandev",
    });

    const gitlab = new GitLabPage(testPage);
    await gitlab.goto();
    await gitlab.startMRTask(DRAFT_IID);
    await gitlab.openLinkedMR(DRAFT_IID);

    const panel = testPage.getByTestId("mr-detail-panel").last();
    const merge = panel.getByRole("button", { name: "Merge", exact: true });
    await expect(panel.getByTestId("mr-merge-draft-hint")).toBeVisible();
    await expect(merge).toBeDisabled();

    await panel.getByTestId("mr-mark-ready-button").click();
    await expect(
      testPage.getByText("Merge request marked as ready", { exact: true }),
    ).toBeVisible();
    await expect(panel.getByTestId("mr-merge-draft-hint")).toHaveCount(0);
    await expect(panel.getByTestId("mr-mark-ready-button")).toHaveCount(0);
    await expect(merge).toBeEnabled();
    await expect(panel.getByTestId("mr-merge-checking-hint")).toHaveCount(0);
  });
});
