import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";

// Exercises the regular task-create dialog (New Task in the sidebar); run with office off.
useRegularMode();

test.describe("Kanban board", () => {
  test("displays a seeded task card", async ({ testPage, apiClient, seedData }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "E2E Kanban Test Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);

    await kanban.goto();

    const card = kanban.taskCardByTitle("E2E Kanban Test Task");
    await expect(card).toBeVisible();
    await expect(kanban.taskCard(task.id)).toBeVisible();
  });

  test("shows create task button", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.createTaskButton.first()).toBeVisible();
  });

  test("opens task preview from kanban card", async ({ testPage, apiClient, seedData }) => {
    // Enable preview-on-click so clicking a card opens the preview panel
    await apiClient.saveUserSettings({ enable_preview_on_click: true });

    const task = await apiClient.createTask(seedData.workspaceId, "Detail View Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);

    await kanban.goto();

    const card = kanban.taskCard(task.id);
    await expect(card).toBeVisible();
    await card.click();

    // After clicking a card with preview enabled, URL gets ?taskId=... param.
    // Use toHaveURL (polling assertion) since replaceState doesn't fire navigation events.
    await expect(testPage).toHaveURL(/taskId=/, { timeout: 10000 });
  });

  // The desktop kanban header centers the search input absolutely. When the
  // preview panel opens, the kanban area shrinks (`kanbanWidth = container -
  // previewWidth`); the header narrows along with it. Below ~1100px there is
  // no longer room between the left/right action groups for the centered
  // search, so the header hides it (see useIsHeaderNarrow in kanban-header).
  //
  // Post-overhaul: the always-on AppSidebar (~320px expanded) permanently eats
  // horizontal space, so the header's own clientWidth is viewport − sidebar.
  // The default Desktop Chrome 1280px viewport now leaves the header below the
  // 1100px narrow threshold even with no preview open, so this test forces a
  // viewport wide enough that the centered search shows with the sidebar
  // present (1500 - ~320 = ~1180 >= 800), and opening the 500px preview drops
  // it below the threshold (1500 - ~320 - 500 = ~680 < 800).
  test("hides header search when preview panel narrows the kanban area", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 1500, height: 800 });
    await apiClient.saveUserSettings({ enable_preview_on_click: true });

    const task = await apiClient.createTask(seedData.workspaceId, "Header Squeeze Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // With no preview open, the header is wide enough (viewport - sidebar >=
    // 800px) that the centered search is visible.
    const search = testPage.getByTestId("kanban-header-search");
    await expect(search).toBeVisible();
    const stageNavigator = testPage.getByTestId("desktop-kanban-stage-navigator");
    await expect(stageNavigator).toHaveCount(0);

    // Open the preview - the header width drops below 800px and the centered
    // search must hide.
    const card = kanban.taskCard(task.id);
    await expect(card).toBeVisible();
    await card.click();
    await expect(testPage.getByTestId("task-preview-panel")).toBeVisible({ timeout: 10_000 });
    await expect(search).toBeHidden({ timeout: 5_000 });
    await expect(stageNavigator).toHaveCount(0);

    // Closing the preview restores the full kanban width and brings the
    // search back.
    await testPage.keyboard.press("Escape");
    await expect(testPage.getByTestId("task-preview-panel")).toBeHidden({ timeout: 5_000 });
    await expect(search).toBeVisible({ timeout: 5_000 });
    await expect(stageNavigator).toHaveCount(0);
  });
  test("pans overflowing board space without blocking card moves", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await testPage.setViewportSize({ width: 1280, height: 800 });
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Mouse pan workflow");
    const sourceStep = await apiClient.createWorkflowStep(workflow.id, "Source", 0, {
      is_start_step: true,
    });
    const targetStep = await apiClient.createWorkflowStep(workflow.id, "Target", 1);
    for (let position = 2; position < 6; position += 1) {
      await apiClient.createWorkflowStep(workflow.id, `Overflow ${position}`, position);
    }
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
    });
    const task = await apiClient.createTask(seedData.workspaceId, "Mouse pan move task", {
      workflow_id: workflow.id,
      workflow_step_id: sourceStep.id,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const scrollWindow = testPage.getByTestId("desktop-kanban-scroll-window");
    const sourceScrollRegion = kanban
      .columnByStepId(sourceStep.id)
      .getByTestId("kanban-column-scroll");
    await expect(sourceScrollRegion).toBeVisible();
    const [windowBox, sourceScrollBox] = await Promise.all([
      scrollWindow.boundingBox(),
      sourceScrollRegion.boundingBox(),
    ]);
    if (!windowBox || !sourceScrollBox) throw new Error("Kanban pan targets have no layout boxes");

    const startX = sourceScrollBox.x + Math.min(sourceScrollBox.width - 12, 80);
    const startY = sourceScrollBox.y + sourceScrollBox.height - 8;
    await testPage.mouse.move(startX, startY);
    await testPage.mouse.down();
    await testPage.mouse.move(startX - 60, startY);
    await expect
      .poll(() => scrollWindow.evaluate((element) => element.scrollLeft))
      .toBeGreaterThan(0);
    await prCapture.screenshot("desktop-kanban-mouse-pan", {
      caption: "Desktop Kanban after dragging empty board space to pan horizontally.",
    });

    const pannedPosition = await scrollWindow.evaluate((element) => element.scrollLeft);
    expect(pannedPosition).toBeGreaterThan(0);
    await testPage.mouse.move(windowBox.x - 8, startY);
    await scrollWindow.evaluate((element) => {
      element.scrollLeft = 0;
    });
    const reentryX = windowBox.x + 16;
    await testPage.mouse.move(reentryX, startY);
    await expect.poll(() => scrollWindow.evaluate((element) => element.scrollLeft)).toBe(0);
    await testPage.mouse.up();
    await expect(scrollWindow).not.toHaveClass(/cursor-grab/);
    await testPage.mouse.move(startX - 140, startY);
    await expect.poll(() => scrollWindow.evaluate((element) => element.scrollLeft)).toBe(0);
    await expect
      .poll(() =>
        testPage.evaluate(
          () => document.documentElement.scrollWidth === document.documentElement.clientWidth,
        ),
      )
      .toBe(true);

    await scrollWindow.evaluate((element) => {
      element.scrollLeft = 0;
    });
    const sourceCard = kanban.taskCard(task.id);
    const targetColumn = kanban.columnByStepId(targetStep.id);
    const sourceBox = await sourceCard.boundingBox();
    if (!sourceBox) throw new Error("Kanban DnD source has no layout box");
    await testPage.mouse.move(
      sourceBox.x + sourceBox.width / 2,
      sourceBox.y + sourceBox.height / 2,
    );
    await testPage.mouse.down();
    await testPage.mouse.move(
      sourceBox.x + sourceBox.width / 2 + 20,
      sourceBox.y + sourceBox.height / 2,
    );
    await expect(sourceCard).toHaveClass(/opacity-50/);
    const targetBox = await targetColumn.boundingBox();
    if (!targetBox) throw new Error("Kanban DnD target has no layout box");
    await testPage.mouse.move(
      targetBox.x + targetBox.width / 2,
      targetBox.y + targetBox.height / 2,
    );
    await testPage.mouse.up();

    await expect
      .poll(async () => (await apiClient.getTask(task.id)).workflow_step_id)
      .toBe(targetStep.id);
    await expect(kanban.taskCardInColumn("Mouse pan move task", targetStep.id)).toBeVisible();
  });
});
