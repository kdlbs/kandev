import { type Locator, type Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { dwell } from "../../helpers/causal-waits";
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

async function cancelPointerDrag(
  page: Page,
  apiClient: ApiClient,
  taskId: string,
  sourceStepId: string,
) {
  await page.keyboard.press("Escape");
  await page.mouse.move(0, 0);
  await page.mouse.up();
  await expect(page.getByTestId("desktop-kanban-drag-end-reserve")).toHaveCount(0);
  await expect
    .poll(async () => (await apiClient.getTask(taskId)).workflow_step_id)
    .toBe(sourceStepId);
}

async function renderedColumnWidths(page: Page) {
  return page
    .locator("[data-kanban-step-id]")
    .evaluateAll((columns) => columns.map((column) => column.getBoundingClientRect().width));
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

  // @covers AC-UI-ADAPTIVE-KANBAN-001.9
  const widthsBeforeDrag = await renderedColumnWidths(testPage);
  expect(widthsBeforeDrag.length).toBe(3);
  await beginPointerDrag(testPage, kanban.taskCard(task.id));
  await expect(testPage.getByTestId("desktop-kanban-drag-end-reserve")).toHaveCount(1);
  const widthsDuringDrag = await renderedColumnWidths(testPage);
  expect(widthsDuringDrag).toHaveLength(widthsBeforeDrag.length);
  widthsDuringDrag.forEach((width, index) => {
    expect(Math.abs(width - widthsBeforeDrag[index])).toBeLessThanOrEqual(1);
  });
  const dragOverflow = await testPage.evaluate(() => {
    const scrollWindow = document.querySelector<HTMLElement>(
      '[data-testid="desktop-kanban-scroll-window"]',
    );
    if (!scrollWindow) throw new Error("desktop Kanban scroll window is missing");
    return {
      documentClientWidth: document.documentElement.clientWidth,
      documentScrollWidth: document.documentElement.scrollWidth,
      scrollbarWidth: getComputedStyle(scrollWindow).scrollbarWidth,
    };
  });
  expect(dragOverflow.documentScrollWidth).toBe(dragOverflow.documentClientWidth);
  expect(dragOverflow.scrollbarWidth).toBe("none");
  await cancelPointerDrag(testPage, apiClient, task.id, sourceStep.id);

  await openColumnsMenu(testPage, workflow.id);
  const autoHideToggle = testPage.getByTestId(`columns-menu-auto-hide-empty-${workflow.id}`);
  await expect(autoHideToggle).toHaveAttribute("aria-checked", "false");
  await autoHideToggle.click();
  await expect(autoHideToggle).toHaveAttribute("aria-checked", "true");
  await expect
    .poll(async () => {
      const { settings } = await apiClient.getUserSettings();
      return (
        (settings.workflow_ids_with_auto_hide_empty_steps as string[] | undefined)?.includes(
          workflow.id,
        ) ?? false
      );
    })
    .toBe(true);
  await expect(kanban.columnByStepId(autoHiddenStep.id)).toHaveCount(0);
  await testPage.getByTestId(`columns-menu-step-${manuallyHiddenStep.id}`).click();
  await expect
    .poll(async () => {
      const { settings } = await apiClient.getUserSettings();
      return (
        (settings.kanban_hidden_step_ids as Record<string, string[]> | undefined)?.[
          workflow.id
        ]?.includes(manuallyHiddenStep.id) ?? false
      );
    })
    .toBe(true);
  await closeColumnsMenu(testPage, workflow.id);
  await expect
    .poll(
      async () =>
        (
          (await apiClient.getUserSettings()).settings.kanban_hidden_step_ids as
            | Record<string, string[]>
            | undefined
        )?.[workflow.id] ?? [],
      { message: "Manual column visibility was persisted" },
    )
    .toContain(manuallyHiddenStep.id);

  await expect(kanban.columnByStepId(sourceStep.id)).toBeVisible();
  await expect(kanban.columnByStepId(autoHiddenStep.id)).toHaveCount(0);
  await expect(kanban.columnByStepId(manuallyHiddenStep.id)).toHaveCount(0);

  await testPage.reload();
  await expect(kanban.columnByStepId(sourceStep.id)).toBeVisible();
  await expect(kanban.columnByStepId(autoHiddenStep.id)).toHaveCount(0);
  await expect(kanban.columnByStepId(manuallyHiddenStep.id)).toHaveCount(0);
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
  await cancelPointerDrag(testPage, apiClient, task.id, sourceStep.id);
  await expect(kanban.columnByStepId(autoHiddenStep.id)).toHaveCount(0);

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

test("keeps the drag reserve after overflowing minimum-width tracks", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await testPage.setViewportSize({ width: 1440, height: 900 });
  const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Overflow reserve E2E");
  const steps = await Promise.all(
    ["Source", "Second", "Third", "Fourth", "Fifth", "Sixth"].map((title, index) =>
      apiClient.createWorkflowStep(workflow.id, title, index, { is_start_step: index === 0 }),
    ),
  );
  const task = await apiClient.createTask(seedData.workspaceId, "Overflow reserve sample", {
    workflow_id: workflow.id,
    workflow_step_id: steps[0].id,
  });
  await apiClient.saveUserSettings({
    workspace_id: seedData.workspaceId,
    workflow_filter_id: workflow.id,
    kanban_hidden_step_ids: {},
    workflow_ids_with_auto_hide_empty_steps: [],
  });

  const kanban = new KanbanPage(testPage);
  await kanban.goto();
  await expect(kanban.columnByStepId(steps[5].id)).toBeVisible();

  const scrollWindow = testPage.getByTestId("desktop-kanban-scroll-window");
  const scrollWidthBeforeDrag = await scrollWindow.evaluate((element) => element.scrollWidth);
  const clientWidth = await scrollWindow.evaluate((element) => element.clientWidth);
  await beginPointerDrag(testPage, kanban.taskCard(task.id));
  const scrollWidthDuringDrag = await scrollWindow.evaluate((element) => element.scrollWidth);
  expect(scrollWidthDuringDrag - scrollWidthBeforeDrag).toBeGreaterThanOrEqual(
    clientWidth - 280 - 1,
  );
  await cancelPointerDrag(testPage, apiClient, task.id, steps[0].id);
});

for (const width of [1440, 1920]) {
  test(`keeps Done under a held drag at the final column (${width}px)`, async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    // @covers AC-UI-ADAPTIVE-KANBAN-001.4, AC-UI-ADAPTIVE-KANBAN-003.9
    await testPage.setViewportSize({ width, height: 900 });
    const baseline = (await apiClient.getUserSettings()).settings;
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Held Done drag");
    try {
      const steps = [];
      for (const [index, name] of ["Backlog", "In Progress", "Review", "Done"].entries()) {
        steps.push(
          await apiClient.createWorkflowStep(workflow.id, name, index, {
            is_start_step: index === 0,
          }),
        );
      }
      const task = await apiClient.createTask(seedData.workspaceId, "Held drag source", {
        workflow_id: workflow.id,
        workflow_step_id: steps[0].id,
      });
      await apiClient.saveUserSettings({
        workflow_filter_id: workflow.id,
        kanban_hidden_step_ids: {},
        workflow_ids_with_auto_hide_empty_steps: [],
      });
      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      const beforeWidths = await renderedColumnWidths(testPage);
      const target = kanban.columnByStepId(steps[3].id);
      const box = await target.boundingBox();
      if (!box) throw new Error("Done has no layout box");
      const x = Math.min(box.x + box.width / 2, width - 70);
      const y = box.y + 140;
      await beginPointerDrag(testPage, kanban.taskCard(task.id));
      const probe = await testPage.evaluateHandle(
        ({ x, y, stepId }) => {
          const state = { lostTarget: false, exceededContent: false, frame: 0 };
          const sample = () => {
            const scroll = document.querySelector<HTMLElement>(
              '[data-testid="desktop-kanban-scroll-window"]',
            )!;
            const grid = scroll.querySelector<HTMLElement>(
              '[data-testid="desktop-kanban-lane-grid"]',
            )!;
            state.lostTarget ||=
              document
                .elementsFromPoint(x, y)
                .find((element) => element.closest("[data-kanban-step-id]"))
                ?.closest("[data-kanban-step-id]")
                ?.getAttribute("data-kanban-step-id") !== stepId;
            state.exceededContent ||=
              scroll.scrollLeft > Math.max(0, grid.scrollWidth - scroll.clientWidth) + 1;
            state.frame = requestAnimationFrame(sample);
          };
          sample();
          return state;
        },
        { x, y, stepId: steps[3].id },
      );
      try {
        await testPage.mouse.move(x, y, { steps: 20 });
        await dwell(
          testPage,
          1200,
          "negative-assertion",
          "A held drag must not scroll Done away from the stationary pointer.",
        );
        await testInfo.attach("held-done", {
          body: await testPage.screenshot({ path: testInfo.outputPath("held-done.png") }),
          contentType: "image/png",
        });
        const observations = await probe.evaluate((state) => ({
          lostTarget: state.lostTarget,
          exceededContent: state.exceededContent,
        }));
        expect(observations).toEqual({ lostTarget: false, exceededContent: false });
        const duringWidths = await renderedColumnWidths(testPage);
        duringWidths.forEach((value, index) =>
          expect(Math.abs(value - beforeWidths[index])).toBeLessThanOrEqual(1),
        );
        await testPage.mouse.up();
      } finally {
        await probe.evaluate((state) => cancelAnimationFrame(state.frame));
        await probe.dispose();
        await testPage.mouse.up();
      }
      await expect
        .poll(async () => (await apiClient.getTask(task.id)).workflow_step_id)
        .toBe(steps[3].id);
      await expect(target.getByTestId(`task-card-${task.id}`)).toBeVisible();
      await testPage.reload();
      await expect(
        kanban.columnByStepId(steps[3].id).getByTestId(`task-card-${task.id}`),
      ).toBeVisible();
      expect(
        await testPage.evaluate(
          () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
        ),
      ).toBeLessThanOrEqual(1);
    } finally {
      await testPage.keyboard.press("Escape");
      await testPage.mouse.up();
      await apiClient.saveUserSettings(baseline);
      await apiClient.deleteWorkflow(workflow.id);
    }
  });
}

async function createOverflowDragBoard(apiClient: ApiClient, workspaceId: string) {
  const baseline = (await apiClient.getUserSettings()).settings;
  const workflow = await apiClient.createWorkflow(workspaceId, "Drag scrolling boundaries");
  const steps = [];
  for (let index = 0; index < 6; index++) {
    steps.push(
      await apiClient.createWorkflowStep(workflow.id, `Step ${index + 1}`, index, {
        is_start_step: index === 0,
      }),
    );
  }
  await apiClient.saveUserSettings({
    workflow_filter_id: workflow.id,
    kanban_hidden_step_ids: {},
    workflow_ids_with_auto_hide_empty_steps: [],
  });
  return {
    workflow,
    steps,
    dispose: async () => {
      await apiClient.saveUserSettings(baseline);
      await apiClient.deleteWorkflow(workflow.id);
    },
  };
}

test("reaches real columns in both directions within one drag", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  // @covers AC-UI-ADAPTIVE-KANBAN-001.3, AC-UI-ADAPTIVE-KANBAN-003.9
  await testPage.setViewportSize({ width: 1440, height: 900 });
  const board = await createOverflowDragBoard(apiClient, seedData.workspaceId);
  try {
    const task = await apiClient.createTask(seedData.workspaceId, "Bidirectional drag", {
      workflow_id: board.workflow.id,
      workflow_step_id: board.steps[0].id,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const scroll = testPage.getByTestId("desktop-kanban-scroll-window");
    const box = await scroll.boundingBox();
    if (!box) throw new Error("scroll window has no layout box");
    await beginPointerDrag(testPage, kanban.taskCard(task.id));
    const realMaximum = await scroll.evaluate(
      (element) =>
        element.querySelector<HTMLElement>('[data-testid="desktop-kanban-lane-grid"]')!
          .scrollWidth - element.clientWidth,
    );
    expect(realMaximum).toBeGreaterThan(0);
    await testPage.mouse.move(box.x + box.width - 4, box.y + 140, { steps: 20 });
    await expect
      .poll(() => scroll.evaluate((element) => element.scrollLeft))
      .toBeCloseTo(realMaximum, 0);
    await testPage.mouse.move(box.x + 4, box.y + 140, { steps: 20 });
    await expect.poll(() => scroll.evaluate((element) => element.scrollLeft)).toBe(0);
    await testPage.mouse.move(box.x + box.width - 4, box.y + 140, { steps: 20 });
    await expect
      .poll(() => scroll.evaluate((element) => element.scrollLeft))
      .toBeCloseTo(realMaximum, 0);
    await testPage.mouse.up();
    await expect
      .poll(async () => (await apiClient.getTask(task.id)).workflow_step_id)
      .toBe(board.steps[5].id);
    await expect(
      kanban.columnByStepId(board.steps[5].id).getByTestId(`task-card-${task.id}`),
    ).toBeVisible();
  } finally {
    await testPage.keyboard.press("Escape");
    await testPage.mouse.up();
    await board.dispose();
  }
});

test("preserves source anchoring when earlier temporary columns appear", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  // @covers AC-UI-ADAPTIVE-KANBAN-003.9
  await testPage.setViewportSize({ width: 1440, height: 900 });
  const board = await createOverflowDragBoard(apiClient, seedData.workspaceId);
  try {
    const task = await apiClient.createTask(seedData.workspaceId, "Anchored source", {
      workflow_id: board.workflow.id,
      workflow_step_id: board.steps[4].id,
    });
    await apiClient.saveUserSettings({
      workflow_ids_with_auto_hide_empty_steps: [board.workflow.id],
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const source = kanban.columnByStepId(board.steps[4].id);
    const before = await source.boundingBox();
    if (!before) throw new Error("source has no layout box");
    await beginPointerDrag(testPage, kanban.taskCard(task.id));
    await expect(testPage.locator("[data-kanban-step-id]")).toHaveCount(6);
    await expect.poll(async () => (await source.boundingBox())?.x).toBeCloseTo(before.x, 0);
    const scroll = testPage.getByTestId("desktop-kanban-scroll-window");
    const position = await scroll.evaluate((element) => element.scrollLeft);
    const realMaximum = await scroll.evaluate(
      (element) =>
        element.querySelector<HTMLElement>('[data-testid="desktop-kanban-lane-grid"]')!
          .scrollWidth - element.clientWidth,
    );
    expect(position).toBeGreaterThan(realMaximum);
    await cancelPointerDrag(testPage, apiClient, task.id, board.steps[4].id);
    await expect(testPage.locator("[data-kanban-step-id]")).toHaveCount(1);
  } finally {
    await testPage.keyboard.press("Escape");
    await testPage.mouse.up();
    await board.dispose();
  }
});

test("retains vertical column auto-scroll during a task drag", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  // @covers AC-UI-ADAPTIVE-KANBAN-003.9
  await testPage.setViewportSize({ width: 1440, height: 900 });
  const board = await createOverflowDragBoard(apiClient, seedData.workspaceId);
  try {
    const tasks = [];
    for (let index = 0; index < 30; index++) {
      tasks.push(
        await apiClient.createTask(seedData.workspaceId, `Vertical drag task ${index}`, {
          workflow_id: board.workflow.id,
          workflow_step_id: board.steps[0].id,
        }),
      );
    }
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const column = kanban.columnByStepId(board.steps[0].id).getByTestId("kanban-column-scroll");
    const box = await column.boundingBox();
    if (!box) throw new Error("column has no layout box");
    await beginPointerDrag(testPage, kanban.taskCard(tasks[0].id));
    await testPage.mouse.move(box.x + box.width / 2, box.y + box.height - 4, { steps: 20 });
    await expect.poll(() => column.evaluate((element) => element.scrollTop)).toBeGreaterThan(100);
    await cancelPointerDrag(testPage, apiClient, tasks[0].id, board.steps[0].id);
  } finally {
    await testPage.keyboard.press("Escape");
    await testPage.mouse.up();
    await board.dispose();
  }
});
