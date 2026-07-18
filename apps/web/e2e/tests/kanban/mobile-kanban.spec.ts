import { test, expect } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { missingGitHealth } from "./health-fixtures";

test.describe("Mobile kanban view", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      system_metrics_display: { show_in_topbar: false },
      workflow_filter_id: "",
      kanban_view_mode: "",
    });
  });

  test("metrics match the height of the mobile topbar actions", async ({ testPage, apiClient }) => {
    await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      system_metrics_display: { show_in_topbar: true },
    });
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    const metrics = testPage.getByTestId("mobile-topbar-metrics");
    await expect(metrics).toBeVisible();
    await expect(mobile.mobileSearchToggle).toBeVisible();
    const metricsBox = await metrics.boundingBox();
    const actionBox = await mobile.mobileSearchToggle.boundingBox();
    if (!metricsBox || !actionBox) throw new Error("topbar action has no bounding box");

    expect(metricsBox.height).toBe(actionBox.height);
  });

  test("renders focused mobile layout with step navigation", async ({
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
    // Search is collapsed behind a topbar icon by default
    await expect(mobile.mobileSearchToggle).toBeVisible();
    await expect(mobile.mobileSearchBar).not.toBeVisible();
    await expect(mobile.boardNavigator).toBeVisible();
    await expect(mobile.boardNavigator).toContainText("E2E Workflow");
    const startStep = seedData.steps.find((step) => step.id === seedData.startStepId);
    if (!startStep) throw new Error("seeded start step not found");
    await expect(mobile.boardNavigator).toContainText(startStep.name);
    // Task card should be visible
    await expect(mobile.taskCardByTitle("Mobile Layout Task")).toBeVisible();
  });

  test("keeps workflow navigation visible when every mobile board is empty", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const secondWorkflow = await apiClient.createWorkflow(seedData.workspaceId, "Empty Workflow");
    await apiClient.createWorkflowStep(secondWorkflow.id, "Empty queue", 0, {
      is_start_step: true,
    });
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: "",
    });

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    await expect(mobile.boardNavigator).toContainText("E2E Workflow");
    await mobile.boardNavigator.click();
    await expect(mobile.workflowItem(seedData.workflowId)).toBeVisible();
    await expect(mobile.workflowItem(secondWorkflow.id)).toBeVisible();

    await testPage.keyboard.press("Escape");
    await mobile.mobileFab.click();
    await expect(testPage.getByTestId("workflow-selector-trigger")).toContainText("E2E Workflow");
  });

  test("does not expose hidden workflows in the mobile navigator", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const hiddenWorkflow = await apiClient.e2eCreateHiddenWorkflow(
      seedData.workspaceId,
      "Hidden system workflow",
    );
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: "",
    });

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    await expect(mobile.boardNavigator).toContainText("E2E Workflow");
    await mobile.boardNavigator.click();
    await expect(mobile.workflowItem(hiddenWorkflow.id)).toHaveCount(0);
  });

  test("keeps the workflow navigator usable for a workflow without steps", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const steplessWorkflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Stepless Workflow",
    );
    await apiClient.createTask(seedData.workspaceId, "Visible workflow task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: "",
    });

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    await mobile.boardNavigator.click();
    const navigatorDrawer = testPage.getByTestId("mobile-board-navigator-drawer");
    await mobile.workflowItem(steplessWorkflow.id).click();
    await expect(navigatorDrawer).toBeVisible();
    await expect(mobile.boardNavigator).toBeVisible();
    await expect(mobile.boardNavigator).toContainText("Stepless Workflow");
    await expect(mobile.boardNavigator).toContainText("No steps configured");
    await expect(testPage.getByTestId("mobile-kanban-no-steps")).toBeVisible();

    await mobile.workflowItem(seedData.workflowId).click();
    await expect(navigatorDrawer).toBeVisible();
    await navigatorDrawer.getByTestId("column-tab-0").click();
    await expect(navigatorDrawer).not.toBeVisible();
    await expect(mobile.boardNavigator).toContainText("E2E Workflow");
  });

  test("renders kanban card dropdown as a mobile bottom sheet", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Mobile Dropdown Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    await mobile.taskCard(task.id).getByRole("button", { name: "More options" }).click();
    const menu = testPage.locator('[data-slot="dropdown-menu-content"]:visible');
    const editItem = menu.getByRole("menuitem", { name: "Edit", exact: true });
    await expect(editItem).toBeVisible();
    await menu.evaluate((element) =>
      Promise.all(element.getAnimations({ subtree: true }).map((animation) => animation.finished)),
    );

    const [menuBox, itemBox, viewport] = await Promise.all([
      menu.boundingBox(),
      editItem.boundingBox(),
      testPage.evaluate(() => ({ width: window.innerWidth, height: window.innerHeight })),
    ]);
    if (!menuBox || !itemBox) throw new Error("mobile dropdown has no layout box");
    expect(menuBox.x).toBeGreaterThanOrEqual(7);
    expect(menuBox.x).toBeLessThanOrEqual(10);
    expect(menuBox.width).toBeGreaterThanOrEqual(viewport.width - 20);
    expect(viewport.height - (menuBox.y + menuBox.height)).toBeGreaterThanOrEqual(7);
    expect(viewport.height - (menuBox.y + menuBox.height)).toBeLessThanOrEqual(10);
    expect(itemBox.height).toBeGreaterThanOrEqual(44);
  });

  test("edits a started task title while keeping its prompt locked", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Started mobile task", {
      description: "Prompt must stay unchanged",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.updateTaskState(task.id, "IN_PROGRESS");
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    await mobile.taskCard(task.id).getByRole("button", { name: "More options" }).click();
    await testPage
      .locator('[data-slot="dropdown-menu-content"]:visible')
      .getByRole("menuitem", { name: "Edit", exact: true })
      .click();

    const dialog = testPage.getByRole("dialog");
    const titleInput = dialog.getByTestId("task-title-input");
    const promptInput = dialog.getByTestId("task-description-input");
    await expect(titleInput).toBeVisible();
    await expect(titleInput).toBeEnabled();
    await expect(promptInput).toBeDisabled();
    await expect(promptInput).toHaveValue("Prompt must stay unchanged");
    await titleInput.fill("Renamed started mobile task");
    await dialog.getByRole("button", { name: "Update", exact: true }).click();

    await expect(dialog).toHaveCount(0);
    await expect
      .poll(async () => (await apiClient.getTask(task.id)).title)
      .toBe("Renamed started mobile task");
  });

  test("shows workflow and step together and switches active workflow from one drawer", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.createTask(seedData.workspaceId, "Default workflow mobile task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const secondWorkflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Mobile Product Workflow",
    );
    const secondStep = await apiClient.createWorkflowStep(secondWorkflow.id, "Product queue", 0, {
      is_start_step: true,
    });
    const thirdWorkflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Mobile Operations Workflow",
    );
    const thirdStep = await apiClient.createWorkflowStep(thirdWorkflow.id, "Operations queue", 0, {
      is_start_step: true,
    });
    await apiClient.createTask(seedData.workspaceId, "Product workflow mobile task", {
      workflow_id: secondWorkflow.id,
      workflow_step_id: secondStep.id,
    });
    await apiClient.createTask(seedData.workspaceId, "Operations workflow mobile task", {
      workflow_id: thirdWorkflow.id,
      workflow_step_id: thirdStep.id,
    });
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: seedData.workflowId,
    });

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    await expect(mobile.boardNavigator).toContainText("E2E Workflow");
    const startStep = seedData.steps.find((step) => step.id === seedData.startStepId);
    if (!startStep) throw new Error("seeded start step not found");
    await expect(mobile.boardNavigator).toContainText(startStep.name);
    const navigatorBox = await mobile.boardNavigator.boundingBox();
    if (!navigatorBox) throw new Error("mobile board navigator has no layout box");
    expect(navigatorBox.height).toBeGreaterThanOrEqual(44);
    await expect(mobile.mobileKanbanLayout()).toHaveCount(1);
    await mobile.boardNavigator.click();
    const navigatorDrawer = testPage.getByTestId("mobile-board-navigator-drawer");
    await expect(navigatorDrawer).toBeVisible();
    await navigatorDrawer.evaluate((element) => {
      element.setAttribute("data-test-instance", "workflow-selection");
    });
    await expect(testPage.getByTestId("column-tab-0")).toBeVisible();
    const secondWorkflowItem = mobile.workflowItem(secondWorkflow.id);
    const workflowItemBox = await secondWorkflowItem.boundingBox();
    if (!workflowItemBox) throw new Error("mobile workflow item has no layout box");
    expect(workflowItemBox.height).toBeGreaterThanOrEqual(44);
    await secondWorkflowItem.click();
    await expect(navigatorDrawer).toBeVisible();
    await expect(navigatorDrawer).toHaveAttribute("data-test-instance", "workflow-selection");
    await expect(secondWorkflowItem).toBeFocused();
    const productStepItem = navigatorDrawer.getByTestId("column-tab-0");
    await expect(productStepItem).toContainText("Product queue");
    await productStepItem.click();
    await expect(navigatorDrawer).not.toBeVisible();

    await mobile.mobileFab.click();
    const createDialog = testPage.getByRole("dialog");
    await expect(createDialog.getByTestId("workflow-selector-trigger")).toContainText(
      "Mobile Product Workflow",
    );
    await createDialog.getByRole("button", { name: "Cancel", exact: true }).click();

    await expect(mobile.taskCardByTitle("Product workflow mobile task")).toBeVisible();
    await expect(mobile.taskCardByTitle("Operations workflow mobile task")).toHaveCount(0);
    await expect(mobile.mobileKanbanLayout()).toHaveCount(1);
    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.workflow_filter_id)
      .toBe(secondWorkflow.id);
    const pageWidth = await testPage.evaluate(() => ({
      scroll: document.documentElement.scrollWidth,
      client: document.documentElement.clientWidth,
    }));
    expect(pageWidth.scroll).toBeLessThanOrEqual(pageWidth.client);

    await mobile.boardNavigator.click();
    await expect(secondWorkflowItem).toBeVisible();
    await expect(mobile.workflowItem(seedData.workflowId)).toHaveAttribute("data-active", "false");
    await expect(secondWorkflowItem).toHaveAttribute("data-active", "true");
    await testPage.keyboard.press("Escape");
    await expect(mobile.boardNavigator).toContainText("Mobile Product Workflow");
    await expect(mobile.taskCardByTitle("Product workflow mobile task")).toBeVisible();
    await expect(mobile.taskCardByTitle("Default workflow mobile task")).toHaveCount(0);
  });

  test("uses Kanban and removes Pipeline choices on mobile without changing desktop preference", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.createTask(seedData.workspaceId, "Pipeline preference mobile task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.saveUserSettings({ kanban_view_mode: "graph2" });

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    await expect(mobile.mobileKanbanLayout()).toBeVisible();
    await mobile.mobileMenuButton.click();
    const menuCard = testPage.getByTestId("mobile-home-menu-card");
    await expect(menuCard).toBeVisible();
    await expect(menuCard.getByText("Pipeline", { exact: true })).toHaveCount(0);
    await expect(menuCard.getByText("Workflow", { exact: true })).toHaveCount(0);
    await expect(menuCard.getByText("Kanban", { exact: true })).toBeVisible();
    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.kanban_view_mode)
      .toBe("graph2");
    await menuCard.evaluate((element) => {
      const surface = element.closest('[data-slot="drawer-content"]') ?? element;
      return Promise.all(
        surface.getAnimations({ subtree: true }).map((animation) => animation.finished),
      );
    });

    const [cardBox, viewport] = await Promise.all([
      menuCard.boundingBox(),
      testPage.evaluate(() => ({ width: window.innerWidth, height: window.innerHeight })),
    ]);
    if (!cardBox) throw new Error("mobile Home menu card has no layout box");
    expect(cardBox.x).toBeGreaterThanOrEqual(7);
    expect(cardBox.x + cardBox.width).toBeLessThanOrEqual(viewport.width - 7);
    expect(viewport.height - (cardBox.y + cardBox.height)).toBeGreaterThanOrEqual(7);
  });

  test("keeps the desktop Pipeline preference when returning from mobile List", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      kanban_view_mode: "graph2",
    });
    await testPage.goto("/tasks");
    await testPage.getByRole("button", { name: "Open menu" }).click();

    const menuCard = testPage.getByTestId("mobile-home-menu-card");
    await menuCard.getByText("Kanban", { exact: true }).click();

    await expect(testPage).toHaveURL(/\/$/);
    await expect(testPage.getByTestId("mobile-kanban-layout")).toBeVisible();
    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.kanban_view_mode)
      .toBe("graph2");
  });

  test("search toggle reveals and hides the search input", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.createTask(seedData.workspaceId, "Toggle Visible Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    // Hidden by default, revealed when the topbar search icon is tapped
    await expect(mobile.mobileSearchBar).not.toBeVisible();
    await mobile.openSearch();
    await expect(mobile.mobileSearchBar).toBeVisible();
    // Input is focused on reveal so the keyboard opens immediately
    await expect(mobile.searchInput()).toBeFocused();

    // Tapping the icon again collapses the search bar
    await mobile.mobileSearchToggle.click();
    await expect(mobile.mobileSearchBar).not.toBeVisible();
  });

  test("collapsing search clears an active query", async ({ testPage, apiClient, seedData }) => {
    await apiClient.createTask(seedData.workspaceId, "Clearable Alpha", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.createTask(seedData.workspaceId, "Other Beta", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    await mobile.openSearch();
    await mobile.searchInput().fill("Alpha");
    await expect(mobile.taskCardByTitle("Other Beta")).not.toBeVisible({ timeout: 5000 });

    // Collapsing clears the query so the full list is shown again
    await mobile.mobileSearchToggle.click();
    await expect(mobile.mobileSearchBar).not.toBeVisible();
    await expect(mobile.taskCardByTitle("Clearable Alpha")).toBeVisible({ timeout: 5000 });
    await expect(mobile.taskCardByTitle("Other Beta")).toBeVisible({ timeout: 5000 });
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

  test("switches workspaces from the mobile menu", async ({ testPage, apiClient }) => {
    const otherWorkspace = await apiClient.createWorkspace("Mobile Alternate Workspace");
    await apiClient.createWorkflow(otherWorkspace.id, "Mobile Alternate Workflow", "simple");

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    await mobile.mobileMenuButton.click();
    const dialog = testPage.getByRole("dialog", { name: "Menu" });
    await expect(dialog.getByText("Workspace", { exact: true })).toBeVisible();
    await expect(testPage.getByTestId("mobile-workspace-trigger")).toContainText("E2E Workspace");

    await testPage.getByTestId("mobile-workspace-trigger").click();
    await testPage.getByTestId(`mobile-workspace-item-${otherWorkspace.id}`).click();

    await expect(dialog).not.toBeVisible();

    await mobile.mobileMenuButton.click();
    await expect(testPage.getByTestId("mobile-workspace-trigger")).toContainText(
      "Mobile Alternate Workspace",
    );
  });

  test("mobile menu exposes settings navigation", async ({ testPage }) => {
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    await mobile.mobileMenuButton.click();
    await testPage.getByRole("link", { name: "Settings" }).click();

    await expect(testPage).toHaveURL(/\/settings(?:\/general)?$/);
    await expect(testPage.getByRole("link", { name: /Appearance/ })).toBeVisible();
  });

  test("opening mobile menu does not focus task search", async ({ testPage }) => {
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    await mobile.mobileMenuButton.click();
    const dialog = testPage.getByRole("dialog", { name: "Menu" });
    const searchInput = dialog.getByPlaceholder("Search tasks...");

    await expect(searchInput).toBeVisible();
    await expect(searchInput).not.toBeFocused();
  });

  test("opens missing git health issue from mobile menu", async ({ testPage, backend }) => {
    await testPage.route(`${backend.baseUrl}/api/v1/system/health`, (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(missingGitHealth),
      }),
    );

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    await mobile.mobileMenuButton.click();
    await testPage.getByRole("button", { name: "Health issues" }).click();

    const dialog = testPage.getByRole("dialog", { name: "Setup Issues" });
    await expect(dialog.getByText("Git executable is required")).toBeVisible();
    await expect(
      dialog.getByText("Install Git and ensure the git executable is available on PATH."),
    ).toBeVisible();
    await expect(dialog.getByRole("button", { name: "View system status" })).toBeVisible();
  });

  test("step drawer allows switching between workflow steps", async ({
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

    // If there are multiple steps, switch to the second step through the drawer.
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

      // Verify tab counts reflect tasks in each step
      await expect(firstTab).toContainText("1");
      await expect(secondTab).toContainText("1");

      // First tab should be active initially
      await expect(firstTab).toHaveAttribute("data-active", "true");

      // Click second tab — active tab should switch
      await secondTab.click();
      await expect(testPage.getByTestId("mobile-board-navigator-drawer")).not.toBeVisible();
      await expect(mobile.boardNavigator).toContainText(steps[1].name);

      // Second step task should be the visible active column
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
    await apiClient.updateWorkflowStep(limitedStep.id, { wip_limit: 1 });
    await apiClient.createTask(seedData.workspaceId, "Mobile WIP One", {
      workflow_id: workflow.id,
      workflow_step_id: limitedStep.id,
    });
    await apiClient.createTask(seedData.workspaceId, "Mobile WIP Two", {
      workflow_id: workflow.id,
      workflow_step_id: limitedStep.id,
    });
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

    // Both tasks should be visible initially
    await expect(mobile.taskCardByTitle("Searchable Alpha")).toBeVisible();
    await expect(mobile.taskCardByTitle("Hidden Beta")).toBeVisible();

    // Reveal the search input from the topbar, then type in it
    await mobile.openSearch();
    await mobile.searchInput().fill("Alpha");

    // Only matching task should remain visible
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

    // With a single workflow, the workflow is still evident in the board navigator.
    await expect(mobile.swimlaneContainer).toBeVisible();
    await expect(mobile.taskCardByTitle("Single Workflow Task")).toBeVisible();

    // The swimlane header (collapse toggle) should not exist for single workflow
    await expect(testPage.getByTestId("swimlane-header")).not.toBeVisible();
    await expect(mobile.boardNavigator).toContainText("E2E Workflow");
    await expect(mobile.boardNavigator).toContainText(seedData.steps[0].name);
  });
});
