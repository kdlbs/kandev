import { test, expect, type SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";

const OWNER = "testorg";
const REPO = "testrepo";
const PR_NUMBER = 190;

async function seedSidebarAutomation(
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<{ navigationTaskId: string; targetTaskId: string }> {
  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("test-user");
  const stepOptions = {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  };
  const navigationTask = await apiClient.seedTask(
    seedData.workspaceId,
    "Mobile automation navigation",
    stepOptions,
  );
  const targetTask = await apiClient.seedTask(
    seedData.workspaceId,
    "Mobile automation target",
    stepOptions,
  );
  await apiClient.mockGitHubAssociateTaskPR({
    task_id: targetTask.task_id,
    workspace_id: seedData.workspaceId,
    repository_id: seedData.repositoryId,
    owner: OWNER,
    repo: REPO,
    pr_number: PR_NUMBER,
    pr_url: `https://github.com/${OWNER}/${REPO}/pull/${PR_NUMBER}`,
    pr_title: "Mobile sidebar automation indicators",
    head_branch: "feat/mobile-sidebar-automation-indicators",
    base_branch: "main",
    author_login: "test-user",
    state: "open",
    review_state: "approved",
    checks_state: "success",
    mergeable_state: "clean",
  });
  await apiClient.updateTaskCIAutomationOptions(targetTask.task_id, {
    repository_id: seedData.repositoryId,
    pr_number: PR_NUMBER,
    auto_fix_enabled: true,
    auto_merge_enabled: true,
  });
  return { navigationTaskId: navigationTask.task_id, targetTaskId: targetTask.task_id };
}

test.describe("Mobile sidebar PR automation indicators", () => {
  test("shows touch indicators, details, and terminal-state cleanup", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    const { navigationTaskId, targetTaskId } = await seedSidebarAutomation(apiClient, seedData);

    await expect
      .poll(
        async () => {
          const response = await apiClient.listTasks(seedData.workspaceId);
          return response.tasks.find((task) => task.id === targetTaskId)?.status_summary
            ?.pull_request;
        },
        { timeout: 15_000 },
      )
      .toMatchObject({
        auto_fix_enabled: true,
        auto_merge_enabled: true,
      });

    await testPage.goto(`/t/${navigationTaskId}`);
    await new SessionPage(testPage).waitForLoad();
    await testPage.getByTestId("mobile-session-menu").tap();

    const sheet = testPage.getByRole("dialog", { name: "Tasks" });
    const targetRow = sheet.locator(`[data-task-row-id="${targetTaskId}"]`);
    await expect(targetRow).toBeVisible();
    const icon = targetRow.getByTestId(`pr-task-icon-${targetTaskId}`);
    await expect(icon).toBeVisible();
    await expect(icon.getByTestId("pr-task-automation-auto-fix")).toBeVisible();
    await expect(icon.getByTestId("pr-task-automation-auto-merge")).toBeVisible();

    const navigationURL = testPage.url();
    await icon.tap();
    const drawer = testPage.getByTestId(`pr-task-automation-drawer-${targetTaskId}`);
    await expect(drawer).toBeVisible();
    const automationDetails = drawer.getByTestId("pr-task-automation-details");
    await expect(automationDetails).toBeVisible();
    await expect(automationDetails.getByText(`PR #${PR_NUMBER}`)).toBeVisible();
    await expect(automationDetails.getByText("Auto-fix")).toBeVisible();
    await expect(automationDetails.getByText("Auto-merge")).toBeVisible();
    await expect
      .poll(async () =>
        testPage.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      )
      .toBe(true);
    await prCapture.screenshot("sidebar-automation-indicators-mobile", {
      caption: "Mobile task switcher shows touch-sized PR automation indicators and details.",
    });

    await testPage.keyboard.press("Escape");
    await expect(drawer).toHaveCount(0);
    await expect(icon).toBeFocused();
    await expect(testPage).toHaveURL(navigationURL);

    await apiClient.mockGitHubAssociateTaskPR({
      task_id: targetTaskId,
      workspace_id: seedData.workspaceId,
      repository_id: seedData.repositoryId,
      owner: OWNER,
      repo: REPO,
      pr_number: PR_NUMBER,
      pr_url: `https://github.com/${OWNER}/${REPO}/pull/${PR_NUMBER}`,
      pr_title: "Mobile sidebar automation indicators",
      head_branch: "feat/mobile-sidebar-automation-indicators",
      base_branch: "main",
      author_login: "test-user",
      state: "closed",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "clean",
    });
    await expect
      .poll(async () => {
        const response = await apiClient.listTasks(seedData.workspaceId);
        const pullRequest = response.tasks.find((task) => task.id === targetTaskId)?.status_summary
          ?.pull_request;
        return pullRequest?.auto_fix_enabled === true || pullRequest?.auto_merge_enabled === true;
      })
      .toBe(false);
    await expect(icon.getByTestId("pr-task-automation-auto-fix")).toHaveCount(0);
    await expect(icon.getByTestId("pr-task-automation-auto-merge")).toHaveCount(0);
  });
});
