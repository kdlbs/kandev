import { type Locator, type Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

const TASK_TITLE = "Mobile auto-hide drag source";

async function openMobileMenu(page: Page) {
  await page.getByRole("button", { name: "Open menu" }).click();
  await page.getByTestId("mobile-home-menu-card").waitFor({ state: "visible" });
}

async function closeMobileMenu(page: Page) {
  const card = page.getByTestId("mobile-home-menu-card");
  await expect(async () => {
    if ((await card.count()) > 0) {
      await page.keyboard.press("Escape");
    }
    await expect(card).toHaveCount(0, { timeout: 1_000 });
  }).toPass({ timeout: 15_000 });
}

async function openColumnsMenu(page: Page, workflowId: string) {
  const trigger = page.getByTestId(`columns-menu-${workflowId}`);
  await trigger.click();
  await expect(trigger).toHaveAttribute("data-state", "open");
}

async function beginPointerDrag(page: Page, card: Locator) {
  const box = await card.boundingBox();
  if (!box) throw new Error("mobile drag source has no layout box");
  const x = box.x + box.width / 2;
  const y = box.y + box.height / 2;
  await page.mouse.move(x, y);
  await page.mouse.down();
  await page.mouse.move(x + 20, y, { steps: 4 });
}

async function expectMinTouchTarget(locator: Locator) {
  await expect(async () => {
    const box = await locator.boundingBox();
    if (!box) throw new Error("touch target has no layout box");
    expect(box.height).toBeGreaterThanOrEqual(44);
  }).toPass({ timeout: 5_000 });
}

test("keeps auto-hide scoped and restores mobile move targets during drag", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Mobile auto-hide E2E");
  const sourceStep = await apiClient.createWorkflowStep(workflow.id, "Source", 0, {
    is_start_step: true,
  });
  const hiddenDestination = await apiClient.createWorkflowStep(workflow.id, "Hidden target", 1);
  const otherWorkflow = await apiClient.createWorkflow(seedData.workspaceId, "Unaffected workflow");
  const otherStart = await apiClient.createWorkflowStep(otherWorkflow.id, "Other source", 0, {
    is_start_step: true,
  });
  const otherEmpty = await apiClient.createWorkflowStep(otherWorkflow.id, "Other empty", 1);
  const task = await apiClient.createTask(seedData.workspaceId, TASK_TITLE, {
    workflow_id: workflow.id,
    workflow_step_id: sourceStep.id,
  });
  await apiClient.createTask(seedData.workspaceId, "Unaffected task", {
    workflow_id: otherWorkflow.id,
    workflow_step_id: otherStart.id,
  });
  await apiClient.saveUserSettings({
    workspace_id: seedData.workspaceId,
    workflow_filter_id: workflow.id,
    kanban_hidden_step_ids: {},
    workflow_ids_with_auto_hide_empty_steps: [],
  });

  const mobile = new MobileKanbanPage(testPage);
  await mobile.goto();
  await openMobileMenu(testPage);
  await expect(testPage.getByTestId(`columns-menu-${otherWorkflow.id}`)).toHaveCount(0);
  await openColumnsMenu(testPage, workflow.id);

  const autoHideToggle = testPage.getByTestId(`columns-menu-auto-hide-empty-${workflow.id}`);
  await expectMinTouchTarget(autoHideToggle);
  await autoHideToggle.click();
  await expect(autoHideToggle).toHaveAttribute("aria-checked", "true");
  await testPage.keyboard.press("Escape");
  await closeMobileMenu(testPage);

  await expect(testPage.getByTestId(`kanban-column-${hiddenDestination.id}`)).toHaveCount(0);
  await beginPointerDrag(testPage, mobile.taskCard(task.id));

  const recoveredTarget = testPage.getByTestId(`mobile-drop-target-${hiddenDestination.id}`);
  await expect(recoveredTarget).toBeVisible();
  await expectMinTouchTarget(recoveredTarget);
  const overflow = await testPage.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(overflow.scrollWidth).toBeLessThanOrEqual(overflow.clientWidth);
  await testPage.keyboard.press("Escape");
  await testPage.mouse.up();
  await expect(recoveredTarget).toHaveCount(0);

  const persistedSettings = (await apiClient.getUserSettings()).settings;
  expect(persistedSettings.workflow_ids_with_auto_hide_empty_steps).toEqual([workflow.id]);

  await apiClient.saveUserSettings({
    workspace_id: seedData.workspaceId,
    workflow_filter_id: otherWorkflow.id,
  });
  await testPage.reload();
  await mobile.mobileKanbanLayout().waitFor({ state: "visible" });
  await expect(mobile.boardNavigator).toContainText("Unaffected workflow");
  await expect(testPage.getByTestId(`kanban-column-${otherEmpty.id}`)).toHaveCount(1);
});
