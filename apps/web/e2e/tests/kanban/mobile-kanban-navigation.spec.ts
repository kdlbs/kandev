import { test, expect } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import {
  withHeightWorkflows,
  expectCompactColumn,
  expectNoDocumentOverflow,
} from "./swimlane-height-helpers";

test("multiple workflows keep a full-height focused phone board across the tablet boundary", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  // @covers AC-UI-ADAPTIVE-KANBAN-002.6
  await withHeightWorkflows(apiClient, seedData, async (second) => {
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    const phoneSize = testPage.viewportSize()!;
    for (const width of [phoneSize.width, 767]) {
      await testPage.setViewportSize({ width, height: phoneSize.height });
      await expect(mobile.mobileKanbanLayout()).toHaveCount(1);
      await expect(mobile.mobileKanbanLayout()).toHaveCSS("display", "flex");
      await expect(mobile.mobileKanbanLayout()).toHaveCSS("overflow-y", "hidden");
      await expect
        .poll(async () => (await mobile.mobileKanbanLayout().boundingBox())!.height)
        .toBeGreaterThan(400);
      const boardBox = (await mobile.swimlaneContainer.boundingBox())!;
      const layoutBox = (await mobile.mobileKanbanLayout().boundingBox())!;
      expect(layoutBox.y + layoutBox.height).toBeLessThanOrEqual(boardBox.y + boardBox.height);
      await expectNoDocumentOverflow(testPage);
    }
    await testPage.setViewportSize(phoneSize);
    await mobile.boardNavigator.tap();
    await mobile.workflowItem(second.workflowId).tap();
    const drawer = testPage.getByTestId("mobile-board-navigator-drawer");
    await expect(drawer).toBeVisible();
    await drawer.getByTestId(`column-tab-${second.stepIndex}`).tap();
    await expect(drawer).not.toBeVisible();
    await expect(mobile.taskCard(second.taskId)).toBeInViewport();
    await testInfo.attach("focused-phone", {
      body: await testPage.screenshot({ path: testInfo.outputPath("focused-phone.png") }),
      contentType: "image/png",
    });
    await mobile.taskCard(second.taskId).tap();
    await expect(testPage).toHaveURL(new RegExp(`/t/${second.taskId}`));
    await apiClient.saveUserSettings({ workflow_filter_id: "" });
    await testPage.setViewportSize({ width: 768, height: phoneSize.height });
    await testPage.goto("/");
    await expect(testPage.getByTestId("mobile-kanban-layout")).toHaveCount(0);
    await expect(testPage.getByTestId("tablet-kanban-layout")).toHaveCount(2);
    await expectCompactColumn(testPage.getByTestId(`kanban-column-${second.stepId}`));
    await expectNoDocumentOverflow(testPage);
    await testPage.setViewportSize(phoneSize);
    await expect(mobile.mobileKanbanLayout()).toHaveCount(1);
    await expect
      .poll(async () => (await mobile.mobileKanbanLayout().boundingBox())!.height)
      .toBeGreaterThan(400);
    expect((await apiClient.getUserSettings()).settings.kanban_view_mode ?? "").toBe("");
  });
});

test.describe("Mobile Kanban navigation and scrolling", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      app_status_bar_enabled: false,
      system_metrics_display: { show_in_topbar: false },
      workflow_filter_id: "",
      kanban_view_mode: "",
    });
  });

  test("keeps phone overflow cues and final-card navigation inside the focused column", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // @covers AC-UI-ADAPTIVE-KANBAN-002.6, AC-UI-ADAPTIVE-KANBAN-003.7
    const taskIds: string[] = [];
    for (let index = 0; index < 12; index++) {
      const task = await apiClient.createTask(
        seedData.workspaceId,
        `Mobile overflow ${index + 1}`,
        {
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
        },
      );
      taskIds.push(task.id);
    }

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    const column = testPage.getByTestId(`kanban-column-${seedData.startStepId}`);
    const scroll = column.getByTestId("kanban-column-scroll");
    await expect
      .poll(() => scroll.evaluate((element) => element.scrollHeight - element.clientHeight))
      .toBeGreaterThan(1);
    await expect(column.getByTestId("kanban-overflow-top-fade")).toHaveAttribute(
      "data-visible",
      "false",
    );
    await expect(column.getByTestId("kanban-overflow-bottom-fade")).toHaveAttribute(
      "data-visible",
      "true",
    );

    await scroll.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
      element.dispatchEvent(new Event("scroll", { bubbles: true }));
    });
    await expect(column.getByTestId("kanban-overflow-top-fade")).toHaveAttribute(
      "data-visible",
      "true",
    );
    await expect(column.getByTestId("kanban-overflow-bottom-fade")).toHaveAttribute(
      "data-visible",
      "false",
    );
    await expect(mobile.taskCard(taskIds.at(-1)!)).toBeInViewport();
    await expectNoDocumentOverflow(testPage);
    await mobile.taskCard(taskIds.at(-1)!).tap();
    await expect(testPage).toHaveURL(new RegExp(`/t/${taskIds.at(-1)!}`));
  });

  test("restores the selected workflow step after opening a task", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Mobile Restore Column Workflow",
    );
    const todoStep = await apiClient.createWorkflowStep(workflow.id, "Todo", 0, {
      is_start_step: true,
    });
    const planStep = await apiClient.createWorkflowStep(workflow.id, "Plan", 1);
    await apiClient.createTask(seedData.workspaceId, "Restore Column Todo Task", {
      workflow_id: workflow.id,
      workflow_step_id: todoStep.id,
    });
    const planTask = await apiClient.createTask(seedData.workspaceId, "Restore Column Plan Task", {
      workflow_id: workflow.id,
      workflow_step_id: planStep.id,
    });
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
    });

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    await expect(mobile.boardNavigator).toContainText("Mobile Restore Column Workflow");
    await prCapture.startRecording("mobile-kanban-column-restore-after");
    await mobile.boardNavigator.click();
    await expect(testPage.getByTestId("mobile-board-navigator-drawer")).toBeVisible();
    await testPage.getByTestId("column-tab-1").click();
    await expect(testPage.getByTestId("mobile-board-navigator-drawer")).not.toBeVisible();
    await expect(mobile.boardNavigator).toContainText("Plan");
    await expect(mobile.taskCardByTitle("Restore Column Plan Task")).toBeInViewport();

    await mobile.taskCard(planTask.id).click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${planTask.id}`));

    await testPage.getByRole("link", { name: "Task overview" }).click();
    await expect(mobile.mobileKanbanLayout()).toBeVisible();
    await expect(mobile.boardNavigator).toContainText("Plan");
    await expect(mobile.taskCardByTitle("Restore Column Plan Task")).toBeInViewport();
    await expect(mobile.taskCardByTitle("Restore Column Todo Task")).not.toBeInViewport();
    await prCapture.stopRecording({
      caption: "After: returning from a task keeps the Plan column selected",
    });
  });

  test("keeps the initial fallback step stable after a live task update", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Mobile Stable Fallback Workflow",
    );
    const todoStep = await apiClient.createWorkflowStep(workflow.id, "Todo", 0, {
      is_start_step: true,
    });
    const planStep = await apiClient.createWorkflowStep(workflow.id, "Plan", 1);
    await apiClient.createTask(seedData.workspaceId, "Stable Fallback Plan Task", {
      workflow_id: workflow.id,
      workflow_step_id: planStep.id,
    });
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
    });

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    await expect(mobile.boardNavigator).toContainText("Plan");
    await expect(mobile.taskCardByTitle("Stable Fallback Plan Task")).toBeInViewport();

    await apiClient.createTask(seedData.workspaceId, "Live Todo Task", {
      workflow_id: workflow.id,
      workflow_step_id: todoStep.id,
    });

    await expect(mobile.taskCardByTitle("Live Todo Task")).toBeAttached();
    await expect(mobile.boardNavigator).toContainText("Plan");
    await expect(mobile.taskCardByTitle("Stable Fallback Plan Task")).toBeInViewport();
    await expect(mobile.taskCardByTitle("Live Todo Task")).not.toBeInViewport();
  });

  test("step drawer allows switching between workflow steps", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const steps = seedData.steps;
    await apiClient.createTask(seedData.workspaceId, "Task In First Step", {
      workflow_id: seedData.workflowId,
      workflow_step_id: steps[0].id,
    });
    if (steps.length > 1) {
      await apiClient.createTask(seedData.workspaceId, "Task In Second Step", {
        workflow_id: seedData.workflowId,
        workflow_step_id: steps[1].id,
      });
    }

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    await expect(mobile.taskCardByTitle("Task In First Step")).toBeVisible();

    if (steps.length > 1) {
      const navigatorBox = await mobile.boardNavigator.boundingBox();
      if (!navigatorBox) throw new Error("mobile board navigator has no layout box");
      expect(navigatorBox.height).toBeGreaterThanOrEqual(44);
      await mobile.boardNavigator.click();
      await expect(testPage.getByTestId("mobile-board-navigator-drawer")).toBeVisible();
      const firstTab = testPage.getByTestId("column-tab-0");
      const secondTab = testPage.getByTestId("column-tab-1");
      const secondTabBox = await secondTab.boundingBox();
      if (!secondTabBox) throw new Error("mobile step item has no layout box");
      expect(secondTabBox.height).toBeGreaterThanOrEqual(44);

      await expect(firstTab).toContainText("1");
      await expect(secondTab).toContainText("1");
      await expect(firstTab).toHaveAttribute("data-active", "true");

      await secondTab.click();
      await expect(testPage.getByTestId("mobile-board-navigator-drawer")).not.toBeVisible();
      await expect(mobile.boardNavigator).toContainText(steps[1].name);
      await expect(mobile.taskCardByTitle("Task In Second Step")).toBeVisible();
      await expect(mobile.taskCardByTitle("Task In Second Step")).toBeInViewport();
      await expect(mobile.taskCardByTitle("Task In First Step")).not.toBeInViewport();

      await testPage.getByRole("button", { name: "Previous step" }).click();
      await expect(mobile.boardNavigator).toContainText(steps[0].name);
      await expect(mobile.taskCardByTitle("Task In First Step")).toBeInViewport();

      const pageWidth = await testPage.evaluate(() => ({
        scroll: document.documentElement.scrollWidth,
        client: document.documentElement.clientWidth,
      }));
      expect(pageWidth.scroll).toBeLessThanOrEqual(pageWidth.client);
    }
  });

  test("column tabs show WIP occupancy over limit", async ({ testPage, apiClient, seedData }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Mobile WIP Workflow");
    const limitedStep = await apiClient.createWorkflowStep(workflow.id, "Limited", 0, {
      is_start_step: true,
    });
    await apiClient.createWorkflowStep(workflow.id, "Done", 1);
    await apiClient.createTask(seedData.workspaceId, "Mobile WIP One", {
      workflow_id: workflow.id,
      workflow_step_id: limitedStep.id,
    });
    await apiClient.createTask(seedData.workspaceId, "Mobile WIP Two", {
      workflow_id: workflow.id,
      workflow_step_id: limitedStep.id,
    });
    await apiClient.updateWorkflowStep(limitedStep.id, { wip_limit: 1 });
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
    });

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    await mobile.boardNavigator.click();
    await expect(testPage.getByTestId("column-tab-0")).toContainText("2/1");
  });

  test("mobile search bar filters tasks", async ({ testPage, apiClient, seedData }) => {
    await apiClient.createTask(seedData.workspaceId, "Searchable Alpha", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.createTask(seedData.workspaceId, "Hidden Beta", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    await expect(mobile.taskCardByTitle("Searchable Alpha")).toBeVisible();
    await expect(mobile.taskCardByTitle("Hidden Beta")).toBeVisible();
    await mobile.openSearch();
    await mobile.searchInput().fill("Alpha");
    await expect(mobile.taskCardByTitle("Searchable Alpha")).toBeVisible({ timeout: 5000 });
    await expect(mobile.taskCardByTitle("Hidden Beta")).not.toBeVisible({ timeout: 5000 });
  });

  test("tapping a task card navigates directly to the task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Direct Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await mobile.taskCard(task.id).click();

    await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}$`));
    await expect(testPage.getByTestId("mobile-task-sheet")).toHaveCount(0);
  });

  test("FAB opens create task dialog", async ({ testPage }) => {
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await mobile.mobileFab.click();
    await expect(testPage.getByRole("dialog")).toBeVisible({ timeout: 5000 });
  });

  test("does not show desktop preview panel on mobile", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.saveUserSettings({ enable_preview_on_click: true });
    const task = await apiClient.createTask(seedData.workspaceId, "No Preview Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await mobile.taskCardByTitle("No Preview Task").click();

    await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}$`));
    await expect(testPage.getByTestId("mobile-task-sheet")).toHaveCount(0);
    await expect(testPage.getByTestId("preview-panel")).toHaveCount(0);
    await expect(testPage).not.toHaveURL(/taskId=/);
  });

  test("swimlane header is hidden when single workflow on mobile", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.createTask(seedData.workspaceId, "Single Workflow Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.steps[0].id,
    });

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    await expect(mobile.swimlaneContainer).toBeVisible();
    await expect(mobile.taskCardByTitle("Single Workflow Task")).toBeVisible();
    await expect(testPage.getByTestId("swimlane-header")).not.toBeVisible();
    await expect(mobile.boardNavigator).toContainText("E2E Workflow");
    await expect(mobile.boardNavigator).toContainText(seedData.steps[0].name);
  });
});
