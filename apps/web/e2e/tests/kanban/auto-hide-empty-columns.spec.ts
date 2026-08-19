import { type Locator, type Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

const TASK_TITLE = "Auto-hide drag source";

async function openColumnsMenu(page: Page, workflowId: string) {
  const trigger = page.getByTestId(`columns-menu-${workflowId}`);
  await trigger.click();
  await expect(trigger).toHaveAttribute("data-state", "open");
}

async function closeColumnsMenu(page: Page, workflowId: string) {
  const trigger = page.getByTestId(`columns-menu-${workflowId}`);
  if ((await trigger.getAttribute("data-state")) === "open") {
    await page.keyboard.press("Escape");
  }
  await expect(trigger).not.toHaveAttribute("data-state", "open");
}

async function beginPointerDrag(page: Page, card: Locator) {
  const box = await card.boundingBox();
  if (!box) throw new Error("drag source has no layout box");
  const x = box.x + box.width / 2;
  const y = box.y + box.height / 2;
  await page.mouse.move(x, y);
  await page.mouse.down();
  await page.mouse.move(x + 20, y, { steps: 4 });
}

test("auto-hides empty columns without changing drag destinations", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await testPage.setViewportSize({ width: 1440, height: 900 });
  const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Auto-hide E2E");
  const sourceStep = await apiClient.createWorkflowStep(workflow.id, "Source", 0, {
    is_start_step: true,
  });
  const autoHiddenStep = await apiClient.createWorkflowStep(workflow.id, "Auto hidden", 1);
  const manuallyHiddenStep = await apiClient.createWorkflowStep(workflow.id, "Manual hidden", 2);
  const task = await apiClient.createTask(seedData.workspaceId, TASK_TITLE, {
    workflow_id: workflow.id,
    workflow_step_id: sourceStep.id,
  });
  await apiClient.saveUserSettings({
    workspace_id: seedData.workspaceId,
    workflow_filter_id: workflow.id,
    kanban_hidden_step_ids: {},
    workflow_ids_with_auto_hide_empty_steps: [],
  });

  const kanban = new KanbanPage(testPage);
  await kanban.goto();

  // Compatibility: the feature is off until this workflow opts in.
  await expect(kanban.columnByStepId(sourceStep.id)).toBeVisible();
  await expect(kanban.columnByStepId(autoHiddenStep.id)).toBeVisible();
  await expect(kanban.columnByStepId(manuallyHiddenStep.id)).toBeVisible();

  await openColumnsMenu(testPage, workflow.id);
  const autoHideToggle = testPage.getByTestId(`columns-menu-auto-hide-empty-${workflow.id}`);
  await expect(autoHideToggle).toHaveAttribute("aria-checked", "false");
  await autoHideToggle.click();
  await expect(autoHideToggle).toHaveAttribute("aria-checked", "true");
  await testPage.getByTestId(`columns-menu-step-${manuallyHiddenStep.id}`).click();
  await closeColumnsMenu(testPage, workflow.id);

  await expect(kanban.columnByStepId(sourceStep.id)).toBeVisible();
  await expect(kanban.columnByStepId(autoHiddenStep.id)).toHaveCount(0);
  await expect(kanban.columnByStepId(manuallyHiddenStep.id)).toHaveCount(0);

  await testPage.reload();
  await expect(kanban.columnByStepId(sourceStep.id)).toBeVisible();
  await expect(kanban.columnByStepId(autoHiddenStep.id)).toHaveCount(0);
  await openColumnsMenu(testPage, workflow.id);
  await expect(autoHideToggle).toHaveAttribute("aria-checked", "true");
  await closeColumnsMenu(testPage, workflow.id);

  // Occupancy is computed before text search: filtering the card does not
  // make its source column disappear.
  const search = testPage.getByTestId("kanban-header-search").getByRole("textbox");
  await search.fill("does not match the source task");
  await expect(kanban.taskCard(task.id)).toHaveCount(0);
  await expect(kanban.columnByStepId(sourceStep.id)).toBeVisible();
  await search.fill("");
  await expect(kanban.taskCard(task.id)).toBeVisible();

  await beginPointerDrag(testPage, kanban.taskCard(task.id));
  await expect(kanban.columnByStepId(autoHiddenStep.id)).toBeVisible();
  await expect(kanban.columnByStepId(manuallyHiddenStep.id)).toHaveCount(0);
  await testPage.keyboard.press("Escape");
  await testPage.mouse.up();
  await expect(kanban.columnByStepId(autoHiddenStep.id)).toHaveCount(0);
  await expect
    .poll(async () => (await apiClient.getTask(task.id)).workflow_step_id)
    .toBe(sourceStep.id);

  await beginPointerDrag(testPage, kanban.taskCard(task.id));
  const destination = kanban.columnByStepId(autoHiddenStep.id);
  await expect(destination).toBeVisible();
  const destinationBox = await destination.boundingBox();
  if (!destinationBox) throw new Error("auto-hidden destination has no layout box");
  await testPage.mouse.move(
    destinationBox.x + destinationBox.width / 2,
    destinationBox.y + Math.min(160, destinationBox.height / 2),
    { steps: 12 },
  );
  await testPage.mouse.up();

  await expect
    .poll(async () => (await apiClient.getTask(task.id)).workflow_step_id)
    .toBe(autoHiddenStep.id);
  await expect(kanban.taskCardInColumn(TASK_TITLE, autoHiddenStep.id)).toBeVisible();
  const settledDestinationBox = await destination.boundingBox();
  if (!settledDestinationBox) throw new Error("occupied destination has no layout box");
  const viewportWidth = await testPage.evaluate(() => window.innerWidth);
  expect(settledDestinationBox.x).toBeLessThan(viewportWidth);
  expect(settledDestinationBox.x + settledDestinationBox.width).toBeGreaterThan(0);
  await expect(kanban.columnByStepId(manuallyHiddenStep.id)).toHaveCount(0);
});
