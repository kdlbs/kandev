import { expect, test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { useRegularMode } from "../../helpers/regular-mode";
import { expectNoDocumentOverflow, withHeightWorkflows } from "./swimlane-height-helpers";

useRegularMode();

test("shows directional vertical cues and chains wheel input at a column boundary", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  // @covers AC-UI-ADAPTIVE-KANBAN-003.2, AC-UI-ADAPTIVE-KANBAN-003.5, AC-UI-ADAPTIVE-KANBAN-003.6
  test.setTimeout(120_000);
  await testPage.setViewportSize({ width: 1440, height: 500 });

  await withHeightWorkflows(apiClient, seedData, async (second) => {
    const denseTasks: string[] = [];
    for (let index = 0; index < 12; index++) {
      const task = await apiClient.createTask(
        seedData.workspaceId,
        `Overflow vertical ${index + 1}`,
        { workflow_id: seedData.workflowId, workflow_step_id: seedData.startStepId },
      );
      denseTasks.push(task.id);
    }
    for (let index = 0; index < 8; index++) {
      await apiClient.createTask(seedData.workspaceId, `Overflow second ${index + 1}`, {
        workflow_id: second.workflowId,
        workflow_step_id: second.stepId,
      });
    }

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const column = kanban.columnByStepId(seedData.startStepId);
    const scroll = column.getByTestId("kanban-column-scroll");
    await expect
      .poll(() => scroll.evaluate((element) => element.scrollHeight - element.clientHeight))
      .toBeGreaterThan(1);

    const topFade = column.getByTestId("kanban-overflow-top-fade");
    const bottomFade = column.getByTestId("kanban-overflow-bottom-fade");
    await expect(topFade).toHaveAttribute("data-visible", "false");
    await expect(bottomFade).toHaveAttribute("data-visible", "true");

    await expect(bottomFade).toHaveCSS("height", "48px");
    await expect(bottomFade).toHaveCSS("pointer-events", "none");
    await expect(bottomFade.locator("svg")).toBeVisible();
    await expect(topFade.locator("svg")).toBeHidden();
    // The cue stays visible even when its edge falls on empty column background.
    await expect(bottomFade.locator("svg")).toHaveCSS("opacity", "0.75");

    await scroll.focus();
    await testPage.keyboard.press("End");
    await expect(topFade).toHaveAttribute("data-visible", "true");
    await expect(bottomFade).toHaveAttribute("data-visible", "false");
    await expect(topFade.locator("svg")).toBeVisible();
    await expect(bottomFade.locator("svg")).toBeHidden();
    await testPage.emulateMedia({ reducedMotion: "reduce" });
    await expect(topFade).toHaveCSS("transition-duration", "0s");
    await expect(kanban.taskCard(denseTasks.at(-1)!)).toBeInViewport();

    const board = testPage.getByTestId("swimlane-container");
    const boardBefore = await board.evaluate((element) => element.scrollTop);
    await scroll.hover();
    await testPage.mouse.wheel(0, 500);
    await expect
      .poll(() => board.evaluate((element) => element.scrollTop))
      .toBeGreaterThan(boardBefore);

    await expectNoDocumentOverflow(testPage);
    await testInfo.attach("vertical-scroll-cues", {
      body: await testPage.screenshot({ path: testInfo.outputPath("vertical-scroll-cues.png") }),
      contentType: "image/png",
    });
  });
});

test("keeps horizontal cues and column geometry stable while the board scrolls", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  // @covers AC-UI-ADAPTIVE-KANBAN-003.1, AC-UI-ADAPTIVE-KANBAN-003.3, AC-UI-ADAPTIVE-KANBAN-003.4
  await testPage.setViewportSize({ width: 1440, height: 900 });
  const workflow = await apiClient.createWorkflow(
    seedData.workspaceId,
    "Horizontal cues",
    "simple",
  );
  try {
    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    for (let index = steps.length; index < 9; index++) {
      await apiClient.createWorkflowStep(workflow.id, `Overflow step ${index}`, index);
    }
    await apiClient.createTask(seedData.workspaceId, "Horizontal cue task", {
      workflow_id: workflow.id,
      workflow_step_id: steps[0]!.id,
    });
    await apiClient.saveUserSettings({ workflow_filter_id: workflow.id, repository_ids: [] });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const scroll = testPage.getByTestId("desktop-kanban-scroll-window");
    await expect
      .poll(() => scroll.evaluate((element) => element.scrollWidth - element.clientWidth))
      .toBeGreaterThan(1);

    const firstColumn = kanban.columnByStepId(steps[0]!.id);
    const initialGeometry = await firstColumn.evaluate((element) => {
      const bounds = element.getBoundingClientRect();
      const header = element.querySelector("h2")?.getBoundingClientRect();
      return {
        columnHeight: bounds.height,
        columnWidth: bounds.width,
        headerHeight: header?.height,
      };
    });
    const leftFade = testPage.getByTestId("kanban-overflow-left-fade");
    const rightFade = testPage.getByTestId("kanban-overflow-right-fade");
    await expect(leftFade).toHaveAttribute("data-visible", "false");
    await expect(rightFade).toHaveAttribute("data-visible", "true");

    await scroll.focus();
    await scroll.evaluate((element) => {
      element.scrollLeft = element.scrollWidth - element.clientWidth;
      element.dispatchEvent(new Event("scroll", { bubbles: true }));
    });
    await expect.poll(() => scroll.evaluate((element) => element.scrollLeft)).toBeGreaterThan(0);
    await expect(leftFade).toHaveAttribute("data-visible", "true");
    await expect(rightFade).toHaveAttribute("data-visible", "false");

    const finalSteps = (await apiClient.listWorkflowSteps(workflow.id)).steps.sort(
      (left, right) => left.position - right.position,
    );
    const finalTask = await apiClient.createTask(seedData.workspaceId, "Final real column task", {
      workflow_id: workflow.id,
      workflow_step_id: finalSteps.at(-1)!.id,
    });
    await expect(kanban.taskCard(finalTask.id)).toBeVisible();
    await scroll.evaluate((element) => {
      element.scrollLeft = element.scrollWidth - element.clientWidth;
      element.dispatchEvent(new Event("scroll", { bubbles: true }));
    });
    const finalCard = kanban.taskCard(finalTask.id);
    const finalCardBox = await finalCard.boundingBox();
    expect(finalCardBox).not.toBeNull();
    await testPage.mouse.move(finalCardBox!.x + 80, finalCardBox!.y + 30);
    await testPage.mouse.down();
    await testPage.mouse.move(finalCardBox!.x + 100, finalCardBox!.y + 30, { steps: 4 });
    await expect(testPage.getByTestId("desktop-kanban-drag-end-reserve")).toHaveCount(1);
    await expect(rightFade).toHaveAttribute("data-visible", "false");
    await testPage.mouse.up();

    const finalGeometry = await firstColumn.evaluate((element) => {
      const bounds = element.getBoundingClientRect();
      const header = element.querySelector("h2")?.getBoundingClientRect();
      return {
        columnHeight: bounds.height,
        columnWidth: bounds.width,
        headerHeight: header?.height,
      };
    });
    expect(finalGeometry.columnWidth).toBeCloseTo(initialGeometry.columnWidth, 0);
    expect(finalGeometry.columnHeight).toBeCloseTo(initialGeometry.columnHeight, 0);
    expect(finalGeometry.headerHeight).toBeCloseTo(initialGeometry.headerHeight ?? 0, 0);
    await expectNoDocumentOverflow(testPage);
    await testInfo.attach("horizontal-scroll-cues", {
      body: await testPage.screenshot({ path: testInfo.outputPath("horizontal-scroll-cues.png") }),
      contentType: "image/png",
    });
  } finally {
    await apiClient.deleteWorkflow(workflow.id);
  }
});

test("shows tablet cues for the full column strip", async ({
  tabletTestPage,
  apiClient,
  seedData,
}) => {
  // @covers AC-UI-ADAPTIVE-KANBAN-003.1, AC-UI-ADAPTIVE-KANBAN-003.3
  const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Tablet cues", "simple");
  try {
    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    for (let index = steps.length; index < 8; index++) {
      await apiClient.createWorkflowStep(workflow.id, `Tablet cue step ${index}`, index);
    }
    await apiClient.saveUserSettings({ workflow_filter_id: workflow.id, repository_ids: [] });

    await tabletTestPage.goto("/");
    const scroll = tabletTestPage.getByTestId("tablet-kanban-scroll-window");
    const leftFade = tabletTestPage.getByTestId("kanban-overflow-left-fade");
    const rightFade = tabletTestPage.getByTestId("kanban-overflow-right-fade");
    await expect(scroll).toBeVisible();
    await expect
      .poll(() => scroll.evaluate((element) => element.scrollWidth - element.clientWidth))
      .toBeGreaterThan(1);
    await expect(leftFade).toHaveAttribute("data-visible", "false");
    await expect(rightFade).toHaveAttribute("data-visible", "true");

    await scroll.evaluate((element) => {
      element.scrollLeft = element.scrollWidth - element.clientWidth;
      element.dispatchEvent(new Event("scroll", { bubbles: true }));
    });
    await expect(leftFade).toHaveAttribute("data-visible", "true");
    await expect(rightFade).toHaveAttribute("data-visible", "false");
  } finally {
    await apiClient.deleteWorkflow(workflow.id);
  }
});

test("does not treat a fitting board's drag reserve as hidden columns", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  // @covers AC-UI-ADAPTIVE-KANBAN-003.1, AC-UI-ADAPTIVE-KANBAN-003.4
  await testPage.setViewportSize({ width: 1920, height: 900 });
  const workflow = await apiClient.createWorkflow(
    seedData.workspaceId,
    "Fitting drag board",
    "simple",
  );
  try {
    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    const task = await apiClient.createTask(seedData.workspaceId, "Fitting drag task", {
      workflow_id: workflow.id,
      workflow_step_id: steps[0]!.id,
    });
    await apiClient.saveUserSettings({ workflow_filter_id: workflow.id, repository_ids: [] });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const scroll = testPage.getByTestId("desktop-kanban-scroll-window");
    const rightFade = testPage.getByTestId("kanban-overflow-right-fade");
    await expect
      .poll(() => scroll.evaluate((element) => element.scrollWidth - element.clientWidth))
      .toBeLessThanOrEqual(1);
    await expect(rightFade).toHaveAttribute("data-visible", "false");

    const cardBox = await kanban.taskCard(task.id).boundingBox();
    expect(cardBox).not.toBeNull();
    await testPage.mouse.move(cardBox!.x + 80, cardBox!.y + 30);
    await testPage.mouse.down();
    await testPage.mouse.move(cardBox!.x + 100, cardBox!.y + 30, { steps: 4 });
    await expect(testPage.getByTestId("desktop-kanban-drag-end-reserve")).toHaveCount(1);
    await expect(rightFade).toHaveAttribute("data-visible", "false");
    await testPage.mouse.up();
  } finally {
    await apiClient.deleteWorkflow(workflow.id);
  }
});

test("keeps tablet horizontal access available in forced colors", async ({
  tabletTestPage,
  apiClient,
  seedData,
}) => {
  // @covers AC-UI-ADAPTIVE-KANBAN-003.7, AC-UI-ADAPTIVE-KANBAN-003.8
  await tabletTestPage.emulateMedia({ forcedColors: "active" });
  const workflow = await apiClient.createWorkflow(
    seedData.workspaceId,
    "Forced colors tablet",
    "simple",
  );
  try {
    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    for (let index = steps.length; index < 8; index++) {
      await apiClient.createWorkflowStep(workflow.id, `Forced color step ${index}`, index);
    }
    await apiClient.createTask(seedData.workspaceId, "Forced colors task", {
      workflow_id: workflow.id,
      workflow_step_id: steps[0]!.id,
    });
    await apiClient.saveUserSettings({ workflow_filter_id: workflow.id, repository_ids: [] });

    await tabletTestPage.goto("/");
    const scroll = tabletTestPage.getByTestId("tablet-kanban-scroll-window");
    await expect(scroll).toBeVisible();
    await expect
      .poll(() => scroll.evaluate((element) => element.scrollWidth - element.clientWidth))
      .toBeGreaterThan(1);
    const styles = await scroll.evaluate((element) => ({
      forcedColors: matchMedia("(forced-colors: active)").matches,
      scrollbarSuppressed: element.classList.contains("scrollbar-hide"),
      scrollbarWidth: getComputedStyle(element).scrollbarWidth,
    }));
    expect(styles.forcedColors).toBe(true);
    expect(styles.scrollbarSuppressed).toBe(false);
    expect(styles.scrollbarWidth).not.toBe("none");

    await scroll.evaluate((element) => {
      element.scrollLeft = element.scrollWidth - element.clientWidth;
      element.dispatchEvent(new Event("scroll", { bubbles: true }));
    });
    await expect.poll(() => scroll.evaluate((element) => element.scrollLeft)).toBeGreaterThan(0);
  } finally {
    await apiClient.deleteWorkflow(workflow.id);
  }
});
