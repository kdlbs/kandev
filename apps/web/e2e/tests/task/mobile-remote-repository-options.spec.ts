import { test, expect } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

test("remote checkout settings use a phone drawer with reachable actions", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  await apiClient.mockGitHubReset();
  await apiClient.saveUserSettings({
    workspace_id: seedData.workspaceId,
    workflow_filter_id: seedData.workflowId,
    task_create_last_used: {
      repository_id: seedData.repositoryId,
      branch: "main",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    },
  });
  await apiClient.mockGitHubAddBranches("checkout-options", "repo", [{ name: "main" }]);
  const mobile = new MobileKanbanPage(testPage);
  await mobile.goto();
  await mobile.mobileFab.click();
  await testPage.getByTestId("source-mode-remote").click();
  await expect(testPage.getByTestId("repository-options-trigger")).toHaveCount(0);
  await testPage.getByTestId("remote-repo-chip-trigger").first().click();
  await testPage.getByTestId("remote-repo-input").fill("https://github.com/checkout-options/repo");
  await testPage.getByTestId("remote-repo-input").press("Enter");
  const trigger = testPage.getByTestId("repository-options-trigger");
  expect((await trigger.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await trigger.click();
  const drawer = testPage.getByTestId("repository-options-drawer");
  await expect(drawer).toBeVisible();
  expect(
    await testPage.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
  const viewport = testPage.viewportSize()!;
  await expect
    .poll(async () => {
      const box = (await drawer.boundingBox())!;
      return (
        box.x >= 0 &&
        box.x + box.width <= viewport.width &&
        box.y >= 0 &&
        box.y + box.height <= viewport.height
      );
    })
    .toBe(true);
  await testPage.getByTestId("repository-options-download").click();
  await expect(testPage.getByRole("option", { name: "On demand", exact: true })).toBeEnabled();
  await testPage.getByRole("option", { name: "On demand", exact: true }).click();
  await expect(testPage.getByRole("listbox")).toHaveCount(0);
  await testPage.getByTestId("repository-options-folders").click();
  await testPage.getByRole("option", { name: "Selected folders", exact: true }).click();
  await expect(testPage.getByRole("listbox")).toHaveCount(0);
  await testPage.getByTestId("repository-options-directories").fill("extensions/one");
  await expect(testPage.getByTestId("repository-options-apply")).toBeEnabled();
  await testPage.screenshot({
    animations: "disabled",
    path: testInfo.outputPath("repository-options-mobile.png"),
  });
  const apply = testPage.getByTestId("repository-options-apply");
  expect((await apply.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await apply.click();
  await expect(drawer).not.toBeVisible();
  await expect(trigger).toBeFocused();
  await expect(testPage.getByTestId("repository-options-summary")).toContainText("1 folder");
});
