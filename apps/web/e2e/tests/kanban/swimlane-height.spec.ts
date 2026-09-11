import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import {
  withHeightWorkflows,
  expectCompactColumn,
  selectHeightWorkflow,
  expectNoDocumentOverflow,
} from "./swimlane-height-helpers";
import {
  seedLargeColumnTasks,
  expectBoundedMountedCards,
  scrollColumnToBottom,
  taskCards,
} from "./large-column-virtualization-helpers";

test("two sparse workflows fit without scrolling the board", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  // @covers AC-UI-ADAPTIVE-KANBAN-002.1, AC-UI-ADAPTIVE-KANBAN-002.2
  await testPage.setViewportSize({ width: 1440, height: 900 });
  const { settings } = await apiClient.getUserSettings();
  const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Compact second", "simple");
  try {
    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    const step = steps.find((item) => item.is_start_step) ?? steps[0];
    await apiClient.createTask(seedData.workspaceId, "Sparse first card", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const second = await apiClient.createTask(seedData.workspaceId, "Sparse second card", {
      workflow_id: workflow.id,
      workflow_step_id: step.id,
    });
    await apiClient.saveUserSettings({ workflow_filter_id: "", repository_ids: [] });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCard(second.id)).toBeVisible();
    const board = testPage.getByTestId("swimlane-container");
    await testInfo.attach("sparse-lane-geometry", {
      body: JSON.stringify({
        board: await board.boundingBox(),
        headers: await testPage
          .getByTestId("swimlane-header")
          .evaluateAll((elements) =>
            elements.map((element) => element.getBoundingClientRect().toJSON()),
          ),
        secondCard: await kanban.taskCard(second.id).boundingBox(),
        scrollTop: await board.evaluate((element) => element.scrollTop),
      }),
      contentType: "application/json",
    });
    await testInfo.attach("sparse-workflows", {
      body: await testPage.screenshot({ path: testInfo.outputPath("sparse-workflows.png") }),
      contentType: "image/png",
    });
    await expect
      .poll(async () => {
        const bounds = await board.boundingBox();
        const card = await kanban.taskCard(second.id).boundingBox();
        return card!.y + card!.height <= bounds!.y + bounds!.height;
      })
      .toBe(true);
    expect(await board.evaluate((element) => element.scrollTop)).toBe(0);
    const column = kanban.columnByStepId(seedData.startStepId);
    const height = (await column.boundingBox())!.height;
    expect(height).toBeGreaterThanOrEqual(200);
    expect(height).toBeLessThanOrEqual(400);
  } finally {
    await apiClient.deleteWorkflow(workflow.id);
    await apiClient.saveUserSettings({
      workflow_filter_id: settings.workflow_filter_id ?? "",
      repository_ids: (settings.repository_ids as string[]) ?? [],
    });
  }
});

test("compact lanes survive collapse, workflow filters and preview resizing", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  // @covers AC-UI-ADAPTIVE-KANBAN-002.3, AC-UI-ADAPTIVE-KANBAN-002.4, AC-UI-ADAPTIVE-KANBAN-002.5
  await testPage.setViewportSize({ width: 1440, height: 900 });
  await withHeightWorkflows(apiClient, seedData, async (second) => {
    await apiClient.saveUserSettings({ enable_preview_on_click: true });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const firstColumn = kanban.columnByStepId(seedData.startStepId);
    const secondColumn = kanban.columnByStepId(second.stepId);
    await expectCompactColumn(firstColumn);
    const toggle = testPage
      .getByTestId("swimlane-header")
      .getByRole("button", { name: "Compact second 1", exact: true });
    await toggle.click();
    await expect(secondColumn).toHaveCount(0);
    await expectCompactColumn(firstColumn);
    await toggle.click();
    await expectCompactColumn(secondColumn);
    await selectHeightWorkflow(testPage, "E2E Workflow");
    await expect(secondColumn).toHaveCount(0);
    await expect.poll(async () => (await firstColumn.boundingBox())!.height).toBeGreaterThan(400);
    await testPage.reload();
    await expect.poll(async () => (await firstColumn.boundingBox())!.height).toBeGreaterThan(400);
    await selectHeightWorkflow(testPage, "All Workflows");
    await expectCompactColumn(secondColumn);
    await kanban.taskCard(second.firstTaskId).click();
    await expect(testPage.getByTestId("task-preview-panel")).toBeVisible();
    await expectCompactColumn(firstColumn);
    await expectCompactColumn(secondColumn);
    await expectNoDocumentOverflow(testPage);
    await testInfo.attach("compact-preview", {
      body: await testPage.screenshot({ path: testInfo.outputPath("compact-preview.png") }),
      contentType: "image/png",
    });
    await testPage.keyboard.press("Escape");
    await expectCompactColumn(firstColumn);
  });
});

test("dense and sparse workflows size independently and keep the final task reachable", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  // @covers AC-UI-ADAPTIVE-KANBAN-002.2, AC-UI-ADAPTIVE-KANBAN-002.7, AC-UI-ADAPTIVE-KANBAN-001.7
  test.setTimeout(120_000);
  await testPage.setViewportSize({ width: 1440, height: 900 });
  await withHeightWorkflows(apiClient, seedData, async (second) => {
    await seedLargeColumnTasks(apiClient, seedData, "Dense height", 439);
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const dense = kanban.columnByStepId(seedData.startStepId);
    const sparse = kanban.columnByStepId(second.stepId);
    await expect.poll(async () => (await dense.boundingBox())?.height).toBe(400);
    await expect.poll(async () => (await sparse.boundingBox())?.height).toBe(200);
    await expectBoundedMountedCards(dense);
    await scrollColumnToBottom(dense.getByTestId("kanban-column-scroll"));
    const oldest = dense.getByTestId(`task-card-${second.firstTaskId}`);
    await expect(oldest).toBeInViewport();
    await expectBoundedMountedCards(dense);
    expect(await taskCards(dense).count()).toBeLessThan(50);
    await expectNoDocumentOverflow(testPage);
    await oldest.click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${second.firstTaskId}`));
  });
});

test("live content grows the lane after drag cancellation and shrinks after removal", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  // @covers AC-UI-ADAPTIVE-KANBAN-002.5, AC-UI-ADAPTIVE-KANBAN-002.7
  await testPage.setViewportSize({ width: 1440, height: 900 });
  await withHeightWorkflows(apiClient, seedData, async (second) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const column = kanban.columnByStepId(second.stepId);
    await expect.poll(async () => (await column.boundingBox())?.height).toBe(200);
    const cardBox = (await kanban.taskCard(second.taskId).boundingBox())!;
    await testPage.mouse.move(cardBox.x + 80, cardBox.y + 30);
    await testPage.mouse.down();
    await testPage.mouse.move(cardBox.x + 100, cardBox.y + 30, { steps: 4 });
    await expect(testPage.getByTestId("desktop-kanban-drag-end-reserve")).toHaveCount(1);
    const added = await Promise.all(
      ["Second", "Third", "Fourth", "Fifth"].map((title) =>
        apiClient.createTask(seedData.workspaceId, title, {
          workflow_id: second.workflowId,
          workflow_step_id: second.stepId,
        }),
      ),
    );
    for (const task of added) await expect(kanban.taskCard(task.id)).toBeAttached();
    expect((await column.boundingBox())!.height).toBe(200);
    await testPage.keyboard.press("Escape");
    await testPage.mouse.up();
    await expect.poll(async () => (await column.boundingBox())!.height).toBeGreaterThan(200);
    expect((await column.boundingBox())!.height).toBeLessThan(400);
    const lane = testPage.getByTestId("desktop-kanban-lane-grid").filter({ has: column });
    const heights = await lane
      .locator("[data-kanban-step-id]")
      .evaluateAll((elements) => elements.map((element) => element.getBoundingClientRect().height));
    expect(Math.max(...heights) - Math.min(...heights)).toBeLessThanOrEqual(1);
    const scroll = column.getByTestId("kanban-column-scroll");
    expect(
      await scroll.evaluate((element) => element.scrollHeight - element.clientHeight),
    ).toBeLessThanOrEqual(1);
    for (const task of added) await apiClient.deleteTask(task.id);
    await expect.poll(async () => (await column.boundingBox())!.height).toBe(200);
  });
});

test("a retained empty workflow keeps compact recovery controls", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  // @covers AC-UI-ADAPTIVE-KANBAN-002.5
  await withHeightWorkflows(apiClient, seedData, async (second) => {
    await apiClient.deleteTask(second.taskId);
    await apiClient.saveUserSettings({
      kanban_hidden_step_ids: { [second.workflowId]: [second.stepId] },
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expectCompactColumn(kanban.columnByStepId(seedData.startStepId));
    await testPage.getByTestId(`columns-menu-${second.workflowId}`).click();
    await testPage.getByTestId(`columns-menu-step-${second.stepId}`).click();
    await testPage.keyboard.press("Escape");
    await expect(testPage.getByTestId(`columns-menu-${second.workflowId}`)).toHaveCount(0);
    await expect
      .poll(async () => (await kanban.columnByStepId(seedData.startStepId).boundingBox())!.height)
      .toBeGreaterThan(400);
  });
});

test("tablet workflows retain two-column snapping inside compact lanes", async ({
  tabletTestPage,
  apiClient,
  seedData,
}, testInfo) => {
  // @covers AC-UI-ADAPTIVE-KANBAN-002.1, AC-UI-ADAPTIVE-KANBAN-002.7
  await withHeightWorkflows(apiClient, seedData, async (second) => {
    const kanban = new KanbanPage(tabletTestPage);
    await kanban.goto();
    await expect(tabletTestPage.getByTestId("tablet-kanban-layout")).toHaveCount(2);
    await expectCompactColumn(kanban.columnByStepId(seedData.startStepId));
    await expectCompactColumn(kanban.columnByStepId(second.stepId));
    await expect(kanban.taskCard(second.taskId)).toBeInViewport();
    await expectNoDocumentOverflow(tabletTestPage);
    await testInfo.attach("compact-tablet", {
      body: await tabletTestPage.screenshot({ path: testInfo.outputPath("compact-tablet.png") }),
      contentType: "image/png",
    });
  });
});
