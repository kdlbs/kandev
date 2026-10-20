import { test, expect } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { requireBox } from "../../helpers/layout-assertions";
import {
  MERGE_ACCESS_DENIED,
  seedMergePermissionTask,
} from "../../helpers/github-merge-permission-fixture";
import { SessionPage } from "../../pages/session-page";

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.15
// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.16
test("denies manual merge into a read-only target despite a writable fork", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  const task = await seedMergePermissionTask(apiClient, seedData, false);
  const mutations: string[] = [];
  testPage.on("request", (request) => {
    if (request.method() === "PUT" && request.url().includes("/merge?"))
      mutations.push(request.url());
  });
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.hoverPRTopbar();
  const popover = session.prTopbarPopover();
  await expect(popover.getByTestId("pr-merge-access")).toHaveText(MERGE_ACCESS_DENIED);
  await expect(popover.getByTestId("pr-merge-button")).toHaveCount(0);
  await expect(popover.locator('[data-testid^="pr-merge-method-"]')).toHaveCount(0);
  await waitForFiniteAnimations(popover);
  await expect(popover).toHaveCSS("opacity", "1");
  await prCapture.screenshot("desktop-pr-merge-access-denied", {
    caption: "Read-only target repository replaces the merge action with an explanation",
  });

  await session.prTopbarButton().blur();
  await session.hoverPRChip();
  await expect(session.prChipPopover().getByTestId("pr-merge-access")).toHaveText(
    MERGE_ACCESS_DENIED,
  );
  await testPage.mouse.move(0, 0);
  const detailTab = session.prDetailTab();
  if (await detailTab.isVisible()) await detailTab.click();
  else await session.prTopbarButton().click();
  const detail = testPage.getByTestId("change-request-detail");
  await expect(detail.getByTestId("pr-merge-access")).toHaveText(MERGE_ACCESS_DENIED);
  await expect(detail.getByTestId("pr-merge-button")).toHaveCount(0);
  await expect(detail.getByTestId("pr-merge-button-more")).toHaveCount(0);
  await dwell(
    testPage,
    300,
    "negative-assertion",
    "Observe all merge surfaces for delayed mutation requests",
  );
  expect(mutations).toEqual([]);
  expect(await apiClient.mockGitHubGetMergeAttempts()).toEqual([]);
});

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.17
// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.20
test("checks unknown permissions before offering a direct merge", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  const task = await seedMergePermissionTask(apiClient, seedData, true);
  let failReads = true;
  await testPage.route("**/merge-access?*", async (route) => {
    if (failReads)
      await route.fulfill({ status: 503, json: { error: "Permission read unavailable" } });
    else await route.continue();
  });
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  const detailTab = session.prDetailTab();
  if (await detailTab.isVisible()) await detailTab.click();
  else await session.prTopbarButton().click();
  const detail = testPage.getByTestId("change-request-detail");
  const retry = detail.getByRole("button", { name: "Check again", exact: true });
  await expect(retry).toBeVisible();
  await expect(detail.getByTestId("pr-merge-button")).toHaveCount(0);
  expect((await requireBox(retry, "permission retry")).height).toBeCloseTo(28, 0);
  await prCapture.screenshot("desktop-pr-merge-access-unavailable", {
    caption: "Unavailable permissions offer a read-only retry",
  });
  failReads = false;
  const [response] = await Promise.all([
    testPage.waitForResponse((candidate) => candidate.url().includes("/merge-access?")),
    retry.click(),
  ]);
  expect(await response.json()).toMatchObject({ state: "allowed" });
  expect(await apiClient.mockGitHubGetMergeAttempts()).toEqual([]);
  const merge = detail.getByTestId("pr-merge-button");
  await expect(merge).toBeEnabled();
  await merge.click();
  await expect(testPage.getByText("PR merged", { exact: true })).toBeVisible();
  expect(await apiClient.mockGitHubGetMergeAttempts()).toHaveLength(1);
});
