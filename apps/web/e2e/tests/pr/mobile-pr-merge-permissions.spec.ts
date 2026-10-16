import { test, expect } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { assertNoDocumentHorizontalOverflow, requireBox } from "../../helpers/layout-assertions";
import {
  MERGE_ACCESS_DENIED,
  MERGE_ACCESS_OWNER,
  MERGE_ACCESS_REPO,
  seedMergePermissionTask,
} from "../../helpers/github-merge-permission-fixture";
import { SessionPage } from "../../pages/session-page";

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.19
test("keeps denied and unknown merge access safe in the phone drawer and Review", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  await testPage.setViewportSize({ width: 320, height: 720 });
  const task = await seedMergePermissionTask(apiClient, seedData, false);
  const mutations: string[] = [];
  testPage.on("request", (request) => {
    if (request.method() === "PUT" && request.url().includes("/merge?"))
      mutations.push(request.url());
  });
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.tapPRStatusChip();
  const drawer = session.prStatusChipPopoverInner();
  await expect(drawer.getByTestId("pr-merge-access")).toHaveText(MERGE_ACCESS_DENIED);
  await expect(drawer.getByTestId("pr-merge-button")).toHaveCount(0);
  await assertNoDocumentHorizontalOverflow(testPage);
  await waitForFiniteAnimations(session.prStatusChipDrawer());
  await drawer.getByTestId("pr-merge-access").scrollIntoViewIfNeeded();
  await expect(drawer.getByTestId("pr-merge-access")).toBeInViewport();
  await prCapture.screenshot("mobile-pr-merge-access-denied", {
    caption: "Phone PR drawer explains that the target repository is read-only",
  });
  await session.prStatusChipDrawerClose().tap();
  await testPage.getByRole("button", { name: "Review", exact: true }).tap();
  const panel = testPage.getByTestId("mobile-review-panel");
  await expect(panel.getByTestId("pr-merge-access")).toHaveText(MERGE_ACCESS_DENIED);
  await expect(panel.getByTestId("pr-merge-button")).toHaveCount(0);
  await assertNoDocumentHorizontalOverflow(testPage);

  await apiClient.mockGitHubSetRepositoryDetails(MERGE_ACCESS_OWNER, MERGE_ACCESS_REPO, null);
  await testPage.reload();
  await session.waitForLoad();
  await testPage.getByRole("button", { name: "Review", exact: true }).tap();
  const retry = panel.getByRole("button", { name: "Check again", exact: true });
  await expect(retry).toBeVisible();
  expect((await requireBox(retry, "phone permission retry")).height).toBeGreaterThanOrEqual(44);
  await prCapture.screenshot("mobile-pr-merge-access-unavailable", {
    caption: "Phone Review offers a touch-sized permission retry",
  });
  await apiClient.mockGitHubSetRepositoryDetails(MERGE_ACCESS_OWNER, MERGE_ACCESS_REPO, false);
  await retry.tap();
  await expect(panel.getByTestId("pr-merge-access")).toHaveText(MERGE_ACCESS_DENIED);
  await assertNoDocumentHorizontalOverflow(testPage);
  await dwell(
    testPage,
    300,
    "negative-assertion",
    "A permission retry must not submit a merge request",
  );
  expect(mutations).toEqual([]);
  expect(await apiClient.mockGitHubGetMergeAttempts()).toEqual([]);
});
