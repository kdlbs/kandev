import { test, expect } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import { KanbanPage } from "../../pages/kanban-page";
import { pasteTaskRepositoryURL } from "../../helpers/task-repository-picker";

async function clearSelectedRepositories(page: Page): Promise<void> {
  for (const testId of ["remove-repo-chip", "remote-chip-remove"]) {
    const removeButtons = page.getByTestId(testId);
    while ((await removeButtons.count()) > 0) {
      await removeButtons.first().click();
    }
  }
}

async function openRemoteAndPasteURL(page: Page, url: string): Promise<void> {
  await clearSelectedRepositories(page);
  await pasteTaskRepositoryURL(page, url);
}

test("remote repository gear applies task-only settings and cancels draft edits", async ({
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
  const kanban = new KanbanPage(testPage);
  await kanban.goto();
  await kanban.createTaskButton.first().click();
  await openRemoteAndPasteURL(testPage, "https://github.com/checkout-options/repo");
  await expect(testPage.getByTestId("repository-options-trigger")).toHaveCount(1);
  await apiClient.mockGitHubAddBranches("checkout-options", "other", [{ name: "main" }]);
  await pasteTaskRepositoryURL(testPage, "https://github.com/checkout-options/other");
  await testPage.getByTestId("repository-options-trigger").first().click();
  await expect(testPage.getByText("For this task only", { exact: true })).toBeVisible();
  await testPage.getByTestId("repository-options-download").click();
  await expect(testPage.getByRole("option", { name: "On demand", exact: true })).toBeEnabled();
  await testPage.getByRole("option", { name: "On demand", exact: true }).click();
  await expect(testPage.getByRole("listbox")).toHaveCount(0);
  await testPage.getByTestId("repository-options-folders").click();
  await testPage.getByRole("option", { name: "Selected folders", exact: true }).click();
  await expect(testPage.getByRole("listbox")).toHaveCount(0);
  await testPage
    .getByTestId("repository-options-directories")
    .fill("extensions/my-extension\npackages/shared");
  await expect(testPage.getByTestId("repository-options-apply")).toBeEnabled();
  await testPage.screenshot({
    animations: "disabled",
    path: testInfo.outputPath("repository-options-desktop.png"),
  });
  await testPage.getByTestId("repository-options-apply").click();
  await expect(testPage.getByTestId("repository-options-popover")).toHaveCount(0);
  await expect(testPage.getByTestId("repository-options-summary")).toContainText("2 folders");
  await testPage.getByTestId("repository-options-trigger").last().click();
  await expect(testPage.getByTestId("repository-options-download")).toHaveText("Standard");
  await expect(testPage.getByTestId("repository-options-folders")).toHaveText("All folders");
  await testPage.getByTestId("repository-options-cancel").click();
  await expect(testPage.getByTestId("repository-options-popover")).toHaveCount(0);

  await testPage.getByTestId("repository-options-trigger").first().click();
  await testPage.getByTestId("repository-options-directories").fill("../invalid");
  await expect(testPage.getByTestId("repository-options-apply")).toBeDisabled();
  await testPage.getByTestId("repository-options-cancel").click();
  await expect(testPage.getByTestId("repository-options-popover")).toHaveCount(0);
  await testPage.getByTestId("repository-options-trigger").first().click();
  await expect(testPage.getByTestId("repository-options-directories")).toHaveValue(
    "extensions/my-extension\npackages/shared",
  );
  await testPage.getByTestId("repository-options-cancel").click();
  await expect(testPage.getByTestId("repository-options-popover")).toHaveCount(0);
  await testPage.getByTestId("repository-options-trigger").first().click();
  await testPage.getByRole("button", { name: "Reset", exact: true }).click();
  await expect(testPage.getByTestId("repository-options-download")).toHaveText("Standard");
  await expect(testPage.getByTestId("repository-options-folders")).toHaveText("All folders");
  await testPage.getByTestId("repository-options-cancel").click();
  await expect(testPage.getByTestId("repository-options-popover")).toHaveCount(0);
  await testPage.getByTestId("task-title-input").fill("Repository options task");
  await testPage.getByTestId("task-description-input").fill("Use selected extension folders");
  await testPage.getByTestId("submit-start-agent-chevron").click();
  await testPage.getByTestId("submit-create-without-agent").click();
  await expect(testPage.getByTestId("create-task-dialog")).not.toBeVisible();
  const tasks = await apiClient.listTasks(seedData.workspaceId);
  const created = tasks.tasks.find((task) => task.title === "Repository options task");
  expect(created).toBeDefined();
  const response = await apiClient.rawRequest("GET", `/api/v1/tasks/${created!.id}`);
  const task = await response.json();
  expect(task.repositories).toHaveLength(2);
  expect(task.repositories[1]).not.toHaveProperty("checkout_options");
  expect(task.repositories[0].checkout_options).toEqual({
    version: 1,
    download_mode: "on_demand",
    sparse_directories: ["extensions/my-extension", "packages/shared"],
  });
  await kanban.createTaskButton.first().click();
  await clearSelectedRepositories(testPage);
  await expect(testPage.getByTestId("repository-options-trigger")).toHaveCount(0);
  await expect(testPage.getByTestId("repository-options-summary")).toHaveCount(0);
});
