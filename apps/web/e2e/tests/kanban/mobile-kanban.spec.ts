import { test, expect } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

test.describe("Mobile kanban view", () => {
  test("renders mobile layout with column tabs and swipeable columns", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.createTask(seedData.workspaceId, "Mobile Layout Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    // Mobile layout should be rendered (swipeable columns, not CSS grid)
    await expect(mobile.mobileKanbanLayout()).toBeVisible();
    // FAB should be visible for creating tasks
    await expect(mobile.mobileFab).toBeVisible();
    // Search bar should be visible on mobile
    await expect(mobile.mobileSearchBar).toBeVisible();
    // Task card should be visible
    await expect(mobile.taskCardByTitle("Mobile Layout Task")).toBeVisible();
  });

  test("shows mobile menu via hamburger button", async ({ testPage }) => {
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    await expect(mobile.mobileMenuButton).toBeVisible();
    await mobile.mobileMenuButton.click();

    // Menu sheet should open with display options
    await expect(testPage.getByRole("heading", { name: "Menu" })).toBeVisible();
    await expect(testPage.getByText("Display Options")).toBeVisible();
  });

  test("column tabs allow switching between workflow steps", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Create tasks in different steps
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

    // First step's task should be visible
    await expect(mobile.taskCardByTitle("Task In First Step")).toBeVisible();

    // If there are multiple steps, switch to second step via tab
    if (steps.length > 1) {
      const firstTab = testPage.getByTestId("column-tab-0");
      const secondTab = testPage.getByTestId("column-tab-1");

      // Verify tab counts reflect tasks in each step
      await expect(firstTab).toContainText("1");
      await expect(secondTab).toContainText("1");

      // First tab should be active initially
      await expect(firstTab).toHaveAttribute("data-active", "true");

      // Click second tab — active tab should switch
      await secondTab.click();
      await expect(secondTab).toHaveAttribute("data-active", "true", { timeout: 5000 });
      await expect(firstTab).toHaveAttribute("data-active", "false");

      // Second step task should exist in the DOM
      await expect(mobile.taskCardByTitle("Task In Second Step")).toBeVisible();
    }
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

    // Both tasks should be visible initially
    await expect(mobile.taskCardByTitle("Searchable Alpha")).toBeVisible();
    await expect(mobile.taskCardByTitle("Hidden Beta")).toBeVisible();

    // Type in search
    await mobile.searchInput().fill("Alpha");

    // Only matching task should remain visible
    await expect(mobile.taskCardByTitle("Searchable Alpha")).toBeVisible({ timeout: 5000 });
    await expect(mobile.taskCardByTitle("Hidden Beta")).not.toBeVisible({ timeout: 5000 });
  });

  test("tapping a task card opens bottom sheet", async ({ testPage, apiClient, seedData }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Sheet Test Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    // Tap on the task card
    await mobile.taskCard(task.id).click();

    // Bottom sheet should appear with task title and action buttons
    await expect(mobile.mobileTaskSheet).toBeVisible({ timeout: 5000 });
    await expect(mobile.mobileTaskSheet.getByText("Sheet Test Task")).toBeVisible();
    await expect(mobile.sheetGoToSession()).toBeVisible();
    await expect(mobile.sheetEditButton()).toBeVisible();
    await expect(mobile.sheetDeleteButton()).toBeVisible();
  });

  test("FAB opens create task dialog", async ({ testPage }) => {
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    await mobile.mobileFab.click();

    // Create task dialog should open
    await expect(testPage.getByRole("dialog")).toBeVisible({ timeout: 5000 });
  });

  test("does not show desktop preview panel on mobile", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Enable preview-on-click to test that it's still hidden on mobile
    await apiClient.saveUserSettings({ enable_preview_on_click: true });

    await apiClient.createTask(seedData.workspaceId, "No Preview Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    // Tap task card
    await mobile.taskCardByTitle("No Preview Task").click();

    // Bottom sheet should appear, NOT the desktop preview panel
    await expect(mobile.mobileTaskSheet).toBeVisible({ timeout: 5000 });
    // Desktop preview panel should NOT exist in the DOM at all
    await expect(testPage.getByTestId("preview-panel")).toHaveCount(0);
    // No ?taskId= URL param (that's the desktop preview behavior)
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

    // With a single workflow, the swimlane header badge should not be shown
    // (the swimlane section header contains the workflow name in a badge)
    await expect(mobile.swimlaneContainer).toBeVisible();
    await expect(mobile.taskCardByTitle("Single Workflow Task")).toBeVisible();

    // The swimlane header (collapse toggle) should not exist for single workflow
    await expect(testPage.getByTestId("swimlane-header")).not.toBeVisible();
  });
});
