import { expect, test } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";

useRegularMode();

function taskIdFromUrl(url: string): string {
  const match = url.match(/\/t\/([^/?]+)/);
  if (!match) throw new Error(`Task route missing from ${url}`);
  return match[1];
}

test.describe("Task create relationships", () => {
  test("selects a parent and related task from advanced settings", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const parent = await apiClient.createTask(seedData.workspaceId, "Relationship parent", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const related = await apiClient.createTask(
      seedData.workspaceId,
      "Relationship related candidate",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      },
    );
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await dialog.getByTestId("task-create-advanced-settings-trigger").click();

    const parentTrigger = dialog.getByTestId("task-create-parent-trigger");
    await parentTrigger.click();
    await dialog.getByTestId(`task-create-parent-option-${parent.id}`).click();
    const relatedTrigger = dialog.getByTestId("task-create-related-trigger");
    await relatedTrigger.click();
    await expect(dialog.getByTestId(`task-create-related-option-${parent.id}`)).toHaveCount(0);
    await dialog.getByTestId(`task-create-related-option-${related.id}`).click();
    await expect(parentTrigger).toContainText("Relationship parent");
    await expect(relatedTrigger).toContainText("1 related task");

    await dialog.getByTestId("task-title-input").fill("Created with parent and related tasks");
    await dialog
      .getByTestId("task-description-input")
      .fill("Verify parent and related links survive create and reload");
    await expect(dialog.getByTestId("submit-start-agent")).toBeEnabled({ timeout: 30_000 });
    await dialog.getByTestId("submit-start-agent-chevron").click();
    await testPage.getByTestId("submit-create-without-agent").click();
    await expect(dialog).not.toBeVisible({ timeout: 15_000 });

    const createdTaskId = taskIdFromUrl(testPage.url());
    const created = await apiClient.getTaskDependencies(createdTaskId);
    expect(created.parent_id).toBe(parent.id);
    expect(created.related?.map((task) => task.id)).toEqual([related.id]);

    await testPage.reload();
    const reloaded = await apiClient.getTaskDependencies(createdTaskId);
    expect(reloaded.parent_id).toBe(parent.id);
    expect(reloaded.related?.map((task) => task.id)).toEqual([related.id]);
    await expect(testPage.getByTestId("task-graph-button")).toBeVisible();
  });
});
