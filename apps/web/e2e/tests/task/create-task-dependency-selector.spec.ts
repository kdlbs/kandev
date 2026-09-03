import type { Locator } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { GITLAB_HOST, GITLAB_PROJECT, gitLabMR } from "../../helpers/gitlab";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";

useRegularMode();

const DEPENDENCY_TRIGGER = "task-create-dependencies-trigger";
const DEPENDENCY_OPTION_PREFIX = "task-create-dependency-option-";
const SEARCH_PLACEHOLDER = "Search tasks or #PR/MR number...";

function taskIdFromUrl(url: string): string {
  const match = url.match(/\/t\/([^/?]+)/);
  if (!match) throw new Error(`Task route missing from ${url}`);
  return match[1];
}

async function openDependencyPicker(
  dialog: Locator,
): Promise<{ dependency: Locator; picker: Locator }> {
  const dependency = dialog.getByTestId(DEPENDENCY_TRIGGER);
  await dependency.click();
  const picker = dialog.getByTestId("task-create-dependencies-popover");
  await expect(picker).toBeVisible();
  return { dependency, picker };
}

test.describe("Task-create dependency selector", () => {
  test("selects, clears, and persists multiple predecessor tasks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const extraWorkflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Dependency selector workflow",
      "simple",
    );
    const taskOptions = {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    };
    const alpha = await apiClient.createTask(seedData.workspaceId, "Dependency Alpha", taskOptions);
    const beta = await apiClient.createTask(seedData.workspaceId, "Dependency Beta", taskOptions);
    const archived = await apiClient.createTask(
      seedData.workspaceId,
      "Archived dependency",
      taskOptions,
    );
    await apiClient.archiveTask(archived.id);

    try {
      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      await kanban.createTaskButton.first().click();

      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();
      const advancedSettings = dialog.getByTestId("task-create-advanced-settings");
      const advancedTrigger = advancedSettings.getByTestId("task-create-advanced-settings-trigger");
      await expect(advancedTrigger).toBeVisible();
      await expect(advancedTrigger).toHaveAttribute("aria-expanded", "false");
      await expect(dialog.getByTestId(DEPENDENCY_TRIGGER)).toHaveCount(0);
      await expect(dialog.getByTestId("agent-profile-selector")).toBeVisible();
      await expect(dialog.getByTestId("executor-profile-selector")).toBeVisible();
      const workflowSelector = dialog.getByTestId("workflow-selector-trigger");
      await expect(workflowSelector).toBeVisible();
      expect(
        await advancedSettings.evaluate((element) => {
          const dialogRoot = element.closest('[data-testid="create-task-dialog"]');
          if (!dialogRoot) return false;
          return [
            "agent-profile-selector",
            "executor-profile-selector",
            "workflow-selector-trigger",
          ].every((testId) => {
            const control = dialogRoot.querySelector(`[data-testid="${testId}"]`);
            return (
              control !== null &&
              Boolean(control.compareDocumentPosition(element) & Node.DOCUMENT_POSITION_FOLLOWING)
            );
          });
        }),
      ).toBe(true);
      await advancedTrigger.click();
      await expect(advancedTrigger).toHaveAttribute("aria-expanded", "true");
      await expect(
        advancedSettings.getByTestId("task-create-dependency-setting-label"),
      ).toContainText("Depends on");
      const settingGrid = advancedSettings.getByTestId("task-create-advanced-settings-grid");
      const settingGridBox = await settingGrid.boundingBox();
      const settingRow = advancedSettings.getByTestId("task-create-dependency-setting-row");
      const settingRowBox = await settingRow.boundingBox();
      const settingLabel = advancedSettings.getByTestId("task-create-dependency-setting-label");
      const settingLabelBox = await settingLabel.boundingBox();
      const selectorContainer = advancedSettings.getByTestId(
        "task-create-dependency-selector-container",
      );
      const selectorContainerBox = await selectorContainer.boundingBox();
      expect(settingGridBox).not.toBeNull();
      expect(settingRowBox).not.toBeNull();
      expect(settingLabelBox).not.toBeNull();
      expect(selectorContainerBox).not.toBeNull();
      expect(settingRowBox!.width).toBeLessThan(settingGridBox!.width * 0.75);
      expect(selectorContainerBox!.width).toBeLessThan(settingGridBox!.width * 0.75);
      expect(selectorContainerBox!.x).toBeGreaterThan(settingLabelBox!.x + settingLabelBox!.width);
      expect(Math.abs(selectorContainerBox!.y - settingLabelBox!.y)).toBeLessThanOrEqual(4);
      const settingInfo = advancedSettings.getByTestId("task-create-dependency-setting-info");
      await settingInfo.hover();
      await expect(testPage.locator('[data-slot="tooltip-content"]:visible').last()).toContainText(
        "This task waits until every selected task completes successfully.",
      );

      const dependency = dialog.getByTestId(DEPENDENCY_TRIGGER);
      await expect(dependency).toBeVisible();
      await expect(dependency).toContainText("No dependency");
      await dependency.click();

      const picker = dialog.getByTestId("task-create-dependencies-popover");
      await expect(picker).toBeVisible();
      const info = picker.getByTestId("task-create-dependency-info");
      await expect(info).toBeVisible();
      await info.hover();
      await expect(testPage.locator('[data-slot="tooltip-content"]:visible').last()).toContainText(
        "This task waits until every selected task completes successfully.",
      );

      const search = picker.getByPlaceholder(SEARCH_PLACEHOLDER);
      await search.fill("Dependency Alpha");
      const alphaOption = picker.getByTestId(`${DEPENDENCY_OPTION_PREFIX}${alpha.id}`);
      await expect(alphaOption).toBeVisible();
      await expect(alphaOption.getByTestId("task-create-dependency-task-icon")).toBeVisible();
      await expect(picker.getByText("Archived dependency")).toHaveCount(0);
      await alphaOption.click();
      await expect(dependency).toContainText("Dependency Alpha");

      await search.fill("Dependency Beta");
      const betaOption = picker.getByTestId(`${DEPENDENCY_OPTION_PREFIX}${beta.id}`);
      await expect(betaOption).toBeVisible();
      await betaOption.click();
      await expect(dependency).toContainText("2 dependencies");

      await search.fill("");
      await picker.getByTestId("task-create-no-dependency").click();
      await expect(dependency).toContainText("No dependency");

      await dependency.click();
      await search.fill("Dependency Alpha");
      await picker.getByTestId(`${DEPENDENCY_OPTION_PREFIX}${alpha.id}`).click();
      await search.fill("Dependency Beta");
      await picker.getByTestId(`${DEPENDENCY_OPTION_PREFIX}${beta.id}`).click();
      await testPage.keyboard.press("Escape");

      await dialog.getByTestId("task-title-input").fill("Task with two dependencies");
      await dialog
        .getByTestId("task-description-input")
        .fill("Created from the dependency selector");
      await expect(dialog.getByTestId("submit-start-agent")).toBeEnabled({ timeout: 30_000 });
      await dialog.getByTestId("submit-start-agent-chevron").click();
      await testPage.getByTestId("submit-create-without-agent").click();
      await expect(dialog).not.toBeVisible({ timeout: 15_000 });
      await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

      const createdTask = await apiClient.getTask(taskIdFromUrl(testPage.url()));
      const dependencies = await apiClient.getTaskDependencies(createdTask.id);
      expect(dependencies.depends_on).toHaveLength(2);
      expect(dependencies.depends_on?.map((dependencyRef) => dependencyRef.id)).toEqual(
        expect.arrayContaining([alpha.id, beta.id]),
      );
    } finally {
      await apiClient.deleteWorkflow(extraWorkflow.id).catch(() => {});
    }
  });

  test("finds a candidate by its linked GitHub PR number and shows the badge", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const taskOptions = {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    };
    const withPR = await apiClient.createTask(
      seedData.workspaceId,
      "Task with a linked PR",
      taskOptions,
    );
    await apiClient.createTask(seedData.workspaceId, "Task without a linked PR", taskOptions);
    await apiClient.mockGitHubAssociateTaskPR({
      workspace_id: seedData.workspaceId,
      task_id: withPR.id,
      owner: "kandev-e2e",
      repo: "dependency-fixtures",
      pr_number: 188,
      pr_url: "https://github.test/kandev-e2e/dependency-fixtures/pull/188",
      pr_title: "Dependency fixture PR",
      head_branch: "feature/dependency-fixture",
      base_branch: "main",
      author_login: "e2e",
      state: "open",
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    await dialog.getByTestId("task-create-advanced-settings-trigger").click();
    const { dependency, picker } = await openDependencyPicker(dialog);

    const option = picker.getByTestId(`${DEPENDENCY_OPTION_PREFIX}${withPR.id}`);
    const search = picker.getByPlaceholder(SEARCH_PLACEHOLDER);

    // Search by bare number.
    await search.fill("188");
    await expect(option).toBeVisible({ timeout: 15_000 });
    await expect(option.locator('[aria-label="Linked pull or merge request #188"]')).toHaveText(
      "#188",
    );

    // Search with a leading '#' matches the same candidate.
    await search.fill("#188");
    await expect(option).toBeVisible();

    await option.click();
    await expect(dependency).toContainText("#188 · Task with a linked PR");
  });

  test("finds a candidate by its linked GitLab MR number and shows the badge", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.configureGitLab(seedData.workspaceId, GITLAB_HOST);
    const iid = 4321;
    await apiClient.mockGitLabAddMRs(seedData.workspaceId, GITLAB_PROJECT, [
      gitLabMR(iid, "Dependency fixture MR"),
    ]);
    await apiClient.updateRepository(seedData.repositoryId, {
      provider: "gitlab",
      provider_host: GITLAB_HOST,
      provider_owner: "platform",
      provider_name: "kandev",
    });

    const taskOptions = {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    };
    const withMR = await apiClient.createTask(
      seedData.workspaceId,
      "Task with a linked MR",
      taskOptions,
    );
    await apiClient.linkTaskGitLabMR(seedData.workspaceId, {
      task_id: withMR.id,
      repository_id: seedData.repositoryId,
      mr_url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${iid}`,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    await dialog.getByTestId("task-create-advanced-settings-trigger").click();
    const { dependency, picker } = await openDependencyPicker(dialog);

    const option = picker.getByTestId(`${DEPENDENCY_OPTION_PREFIX}${withMR.id}`);
    const search = picker.getByPlaceholder(SEARCH_PLACEHOLDER);
    await search.fill(String(iid));
    await expect(option).toBeVisible({ timeout: 15_000 });
    await expect(option.locator(`[aria-label="Linked pull or merge request #${iid}"]`)).toHaveText(
      `#${iid}`,
    );
    await option.click();
    await expect(dependency).toContainText(`#${iid} · Task with a linked MR`);
  });
});
