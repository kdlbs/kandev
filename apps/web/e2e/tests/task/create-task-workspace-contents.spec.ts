import fs from "node:fs";
import path from "node:path";
import { expect, test } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";
import type { Page } from "@playwright/test";

useRegularMode();

async function addFolderFromMenu(page: Page, folderPath: string) {
  const menu = page.getByTestId("workspace-source-menu");
  await menu.getByTestId("workspace-source-menu-folder").click();
  const picker = menu.getByTestId("folder-picker-inline");
  await expect(picker).toBeVisible();
  await picker.getByTestId("folder-picker-path-input").fill(folderPath);
  await picker.getByTestId("folder-picker-path-go").click();
  await expect(picker.getByTestId("folder-picker-choose")).toBeEnabled({ timeout: 15_000 });
  await picker.getByTestId("folder-picker-choose").click();
}

test.describe("Task creation workspace contents", () => {
  test("leaves a first-open empty workspace without a repository chip", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workspaceName = "First-open empty workspace";
    const workspace = await apiClient.createWorkspace(workspaceName);

    try {
      const workflow = await apiClient.createWorkflow(
        workspace.id,
        "First-open workflow",
        "simple",
      );
      await apiClient.saveUserSettings({
        workspace_id: workspace.id,
        workflow_filter_id: workflow.id,
        task_create_last_used: {
          repository_id: "",
          branch: "",
          agent_profile_id: seedData.agentProfileId,
          executor_profile_id: seedData.worktreeExecutorProfileId,
          workflow_ids_by_workspace: { [workspace.id]: workflow.id },
          workspace_sources_by_workspace: {},
        },
      });

      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      await kanban.createTaskButton.first().click();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();
      await expect(dialog.getByTestId("repo-chip")).toHaveCount(0);
      await expect(dialog.getByTestId("add-repository")).toHaveAttribute(
        "aria-label",
        "Add Repository/Folder",
      );

      const scratchHint = "An empty scratch workspace will be created.";
      const footer = dialog.getByTestId("task-create-dialog-footer");
      await expect(footer.getByText(scratchHint, { exact: true })).toHaveCount(1);
      await expect(dialog.getByText(scratchHint, { exact: true })).toHaveCount(1);
    } finally {
      await testPage
        .getByTestId("submit-cancel")
        .click()
        .catch(() => undefined);
      await apiClient.deleteWorkspace(workspace.id, workspaceName).catch(() => undefined);
    }
  });

  test("restores the complete last-used source order", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const folderPath = path.join(backend.tmpDir, "restored-task-assets");
    fs.mkdirSync(folderPath, { recursive: true });
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      task_create_last_used: {
        workspace_sources_by_workspace: {
          [seedData.workspaceId]: [
            { kind: "folder", local_path: folderPath, display_name: "restored-assets" },
            {
              kind: "repository",
              repository_id: seedData.repositoryId,
              base_branch: "main",
              checkout_branch: "main",
            },
          ],
        },
      },
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.getByTestId("workspace-folder-selection")).toContainText("restored-assets");
    await expect(dialog.getByTestId("repo-chip")).toHaveCount(1);

    const sourceIds = await dialog
      .locator('[data-testid="workspace-folder-selection"], [data-testid="repo-chip"]')
      .evaluateAll((elements) => elements.map((element) => element.getAttribute("data-testid")));
    expect(sourceIds).toEqual(["workspace-folder-selection", "repo-chip"]);
    await dialog.getByTestId("submit-cancel").click();
    await expect(dialog).not.toBeVisible();
  });

  test("appends a folder from the unified menu and persists an explicit empty state", async ({
    testPage,
    apiClient,
    backend,
  }) => {
    const folderPath = path.join(backend.tmpDir, "task-assets");
    fs.mkdirSync(folderPath, { recursive: true });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    const { executors } = await apiClient.listExecutors();
    const localExecutor = executors.find((executor) =>
      ["local", "local_pc"].includes(executor.type),
    );
    const localProfile = localExecutor?.profiles?.[0];
    expect(
      localProfile,
      "a direct local executor profile is required by the fixture",
    ).toBeDefined();
    await dialog.getByTestId("executor-profile-selector").click();
    await testPage.getByRole("option", { name: new RegExp(localProfile!.name) }).click();
    const removeRepository = dialog.getByTestId("remove-repo-chip");
    const removeFolder = dialog.getByTestId("remove-workspace-folder");
    while ((await removeRepository.count()) > 0) await removeRepository.first().click();
    while ((await removeFolder.count()) > 0) await removeFolder.first().click();
    const add = dialog.getByTestId("add-repository");
    await expect(add).toHaveAttribute("aria-label", "Add Repository/Folder");
    await add.click();

    const options = testPage.getByTestId("workspace-source-menu-options");
    const labels = await options
      .locator("button")
      .evaluateAll((buttons) =>
        buttons.map((button) => button.querySelector("span.font-medium")?.textContent?.trim()),
      );
    expect(labels).toEqual(["Repository", "Local Folder", "Repository Set"]);
    const repositoryOption = options.getByTestId("workspace-source-menu-repository");
    await expect(repositoryOption).toHaveCSS("font-size", "12px");
    await expect(repositoryOption).toHaveCSS("min-height", "28px");
    await addFolderFromMenu(testPage, folderPath);
    await expect(dialog.getByTestId("workspace-folder-selection")).toContainText("task-assets");

    await add.click();
    await testPage.getByTestId("workspace-source-menu-repository").click();
    const localRepository = testPage
      .getByTestId("task-repository-local-option")
      .filter({ hasText: "E2E Repo" });
    await expect(localRepository).toBeVisible({ timeout: 15_000 });
    await localRepository.click();
    await expect(dialog.getByTestId("repo-chip")).toHaveCount(1);

    await dialog.getByTestId("task-title-input").fill("Task with a local folder");
    await dialog
      .getByTestId("task-description-input")
      .fill("Keep the folder beside the repository");
    const responsePromise = testPage.waitForResponse(
      (response) =>
        response.url().endsWith("/api/v1/tasks") && response.request().method() === "POST",
    );
    await dialog.getByTestId("submit-start-agent-chevron").click();
    await testPage.getByTestId("submit-create-without-agent").click();
    const response = await responsePromise;
    expect(response.ok()).toBe(true);
    const created = (await response.json()) as { id: string };
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    const task = await apiClient.getTask(created.id);
    expect(task.repositories).toEqual([expect.objectContaining({ position: 1 })]);
    expect(task.workspace_folders).toEqual([
      expect.objectContaining({
        local_path: folderPath,
        display_name: "task-assets",
        position: 0,
      }),
    ]);

    await kanban.createTaskButton.first().click();
    const emptyDialog = testPage.getByTestId("create-task-dialog");
    await expect(emptyDialog).toBeVisible();
    await expect(emptyDialog.getByTestId("add-repository")).toHaveAttribute(
      "aria-label",
      "Add Repository/Folder",
    );
  });
});
