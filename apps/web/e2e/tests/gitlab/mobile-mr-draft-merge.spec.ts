import { test, expect } from "../../fixtures/test-base";
import { GITLAB_HOST, GITLAB_PROJECT, gitLabMR, seedGitLabReview } from "../../helpers/gitlab";
import { GitLabPage } from "../../pages/gitlab-page";

test("marks a draft merge request ready from the mobile Review panel", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await testPage.setViewportSize({ width: 393, height: 851 });
  const iid = 92;
  const title = "Draft: mobile readiness check";
  await seedGitLabReview(apiClient, seedData.workspaceId, iid, title);
  await apiClient.mockGitLabAddMRs(seedData.workspaceId, GITLAB_PROJECT, [
    gitLabMR(iid, title, { draft: true }),
  ]);
  await apiClient.updateRepository(seedData.repositoryId, {
    provider: "gitlab",
    provider_host: GITLAB_HOST,
    provider_owner: "platform",
    provider_name: "kandev",
  });

  const gitlab = new GitLabPage(testPage);
  await gitlab.goto();
  await gitlab.startMRTask(iid);
  await gitlab.openLinkedMR(iid);

  const panel = testPage.getByTestId("mr-detail-panel").last();
  await expect(panel.getByTestId("mr-merge-draft-hint")).toBeVisible();
  const ready = panel.getByTestId("mr-mark-ready-button");
  await expect(ready).toBeVisible();
  const bounds = await ready.boundingBox();
  expect(bounds, "Mark as ready has no touch target").not.toBeNull();
  if (bounds) {
    expect(Math.round(bounds.height), "Mark as ready touch target height").toBeGreaterThanOrEqual(
      44,
    );
  }

  await ready.tap();
  await expect(testPage.getByText("Merge request marked as ready", { exact: true })).toBeVisible();
  await expect(panel.getByTestId("mr-merge-draft-hint")).toHaveCount(0);
  await expect(panel.getByTestId("mr-mark-ready-button")).toHaveCount(0);
  await expect(panel.getByRole("button", { name: "Merge", exact: true })).toBeEnabled();
});
