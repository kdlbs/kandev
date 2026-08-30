import { type CDPSession, type Locator, type Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

const TASK_TITLE = "Mobile auto-hide drag source";

async function openMobileMenu(mobile: MobileKanbanPage) {
  await mobile.mobileMenuButton.click();
  await mobile.menuCard.waitFor({ state: "visible" });
}

async function closeMobileMenu(page: Page, mobile: MobileKanbanPage) {
  await expect(async () => {
    if ((await mobile.menuCard.count()) > 0) {
      await page.keyboard.press("Escape");
    }
    await expect(mobile.menuCard).toHaveCount(0, { timeout: 1_000 });
  }).toPass({ timeout: 15_000 });
}

async function openColumnsMenu(
  page: Page,
  mobile: MobileKanbanPage,
  workflowId: string,
): Promise<Locator> {
  const trigger = mobile.menuCard.getByTestId(`columns-menu-${workflowId}`);
  await trigger.click();
  await expect(trigger).toHaveAttribute("data-state", "open");
  const menu = page.locator('[role="menu"]:visible');
  await expect(menu).toBeVisible();
  return menu;
}

async function touchDragToTarget(page: Page, card: Locator, target: Locator) {
  const box = await card.boundingBox();
  if (!box) throw new Error("mobile drag source has no layout box");
  const x = box.x + box.width / 2;
  const y = box.y + box.height / 2;
  const cdp: CDPSession = await page.context().newCDPSession(page);
  await cdp.send("Input.dispatchTouchEvent", {
    type: "touchStart",
    touchPoints: [{ x, y, id: 1 }],
  });
  await dwell(
    page,
    350,
    "library-timer",
    "dnd-kit's TouchSensor needs its 250ms activation delay before the move target appears",
  );
  await cdp.send("Input.dispatchTouchEvent", {
    type: "touchMove",
    touchPoints: [{ x: x + 14, y, id: 1 }],
  });
  await expect(target).toBeVisible();
  await expectMinTouchTarget(target);
  const targetBox = await target.boundingBox();
  if (!targetBox) throw new Error("mobile drop target has no layout box");
  const targetX = targetBox.x + targetBox.width / 2;
  const targetY = targetBox.y + targetBox.height / 2;
  for (let step = 1; step <= 10; step += 1) {
    await cdp.send("Input.dispatchTouchEvent", {
      type: "touchMove",
      touchPoints: [
        {
          x: x + ((targetX - x) * step) / 10,
          y: y + ((targetY - y) * step) / 10,
          id: 1,
        },
      ],
    });
  }
  await expect(target).toHaveClass(/border-primary/);
  await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
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
  await openMobileMenu(mobile);
  await expect(mobile.menuCard.getByTestId(`columns-menu-${otherWorkflow.id}`)).toHaveCount(0);
  const columnsMenu = await openColumnsMenu(testPage, mobile, workflow.id);

  const autoHideToggle = columnsMenu.getByTestId(`columns-menu-auto-hide-empty-${workflow.id}`);
  await expectMinTouchTarget(autoHideToggle);
  await autoHideToggle.click();
  await expect(autoHideToggle).toHaveAttribute("aria-checked", "true");
  await testPage.keyboard.press("Escape");
  await closeMobileMenu(testPage, mobile);

  await expect(testPage.getByTestId(`kanban-column-${hiddenDestination.id}`)).toHaveCount(0);
  const recoveredTarget = testPage.getByTestId(`mobile-drop-target-${hiddenDestination.id}`);
  await touchDragToTarget(testPage, mobile.taskCard(task.id), recoveredTarget);
  const overflow = await testPage.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(overflow.scrollWidth).toBeLessThanOrEqual(overflow.clientWidth);
  await expect(recoveredTarget).toHaveCount(0);
  await expect
    .poll(async () => (await apiClient.getTask(task.id)).workflow_step_id)
    .toBe(hiddenDestination.id);
  await expect(testPage.getByTestId(`kanban-column-${hiddenDestination.id}`)).toBeVisible();

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
