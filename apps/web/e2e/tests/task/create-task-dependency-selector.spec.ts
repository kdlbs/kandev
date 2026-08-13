import { expect, test } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";

useRegularMode();

const DEPENDENCY_TRIGGER = "task-create-dependencies-trigger";
const DEPENDENCY_OPTION_PREFIX = "task-create-dependency-option-";

function taskIdFromUrl(url: string): string {
  const match = url.match(/\/t\/([^/?]+)/);
  if (!match) throw new Error(`Task route missing from ${url}`);
  return match[1];
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

      const dependency = dialog.getByTestId(DEPENDENCY_TRIGGER);
      await expect(dependency).toBeVisible();
      await expect(dependency).toContainText("No dependency");
      await dependency.click();

      const picker = dialog.getByTestId("task-create-dependencies-popover");
      await expect(picker).toBeVisible();
      const info = picker.getByTestId("task-create-dependency-info");
      await expect(info).toBeVisible();
      await info.hover();
      await expect(testPage.getByRole("tooltip")).toContainText(
        "This task waits until every selected task completes successfully.",
      );

      const search = picker.getByPlaceholder("Search tasks...");
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
});
