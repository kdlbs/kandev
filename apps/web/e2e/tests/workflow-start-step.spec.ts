import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";

test.describe("Workflow start step placement", () => {
  test.describe("without explicit start step (Todo -> In Progress -> Done)", () => {
    test("task lands in first step by position (Todo)", async ({
      testPage,
      apiClient,
      seedData,
    }) => {
      const workflow = await apiClient.createWorkflow(
        seedData.workspaceId,
        "No Start Step Workflow",
      );
      const todoStep = await apiClient.createWorkflowStep(workflow.id, "Todo", 0);
      await apiClient.createWorkflowStep(workflow.id, "In Progress", 1);
      await apiClient.createWorkflowStep(workflow.id, "Done", 2);

      await apiClient.saveUserSettings({
        workspace_id: seedData.workspaceId,
        workflow_filter_id: workflow.id,
        enable_preview_on_click: false,
      });

      // Create task without explicit step — should resolve to position 0 (Todo)
      await apiClient.createTask(seedData.workspaceId, "No Start Step Task", {
        workflow_id: workflow.id,
      });

      const kanban = new KanbanPage(testPage);
      await kanban.goto();

      const card = kanban.taskCardInColumn("No Start Step Task", todoStep.id);
      await expect(card).toBeVisible({ timeout: 10_000 });
    });

    test("plan mode task also lands in first step (Todo)", async ({
      testPage,
      apiClient,
      seedData,
    }) => {
      const workflow = await apiClient.createWorkflow(
        seedData.workspaceId,
        "No Start Step Plan Workflow",
      );
      const todoStep = await apiClient.createWorkflowStep(workflow.id, "Todo", 0);
      await apiClient.createWorkflowStep(workflow.id, "In Progress", 1);
      await apiClient.createWorkflowStep(workflow.id, "Done", 2);

      await apiClient.saveUserSettings({
        workspace_id: seedData.workspaceId,
        workflow_filter_id: workflow.id,
        enable_preview_on_click: false,
      });

      // plan_mode task with no explicit step — both modes resolve to position 0
      await apiClient.createTask(seedData.workspaceId, "Plan No Start Step Task", {
        workflow_id: workflow.id,
        plan_mode: true,
      });

      const kanban = new KanbanPage(testPage);
      await kanban.goto();

      const card = kanban.taskCardInColumn("Plan No Start Step Task", todoStep.id);
      await expect(card).toBeVisible({ timeout: 10_000 });
    });
  });

  test.describe("with explicit start step (Todo -> In Progress [start] -> Done)", () => {
    test("task lands on start step (In Progress)", async ({ testPage, apiClient, seedData }) => {
      const workflow = await apiClient.createWorkflow(
        seedData.workspaceId,
        "Explicit Start Step Workflow",
      );
      await apiClient.createWorkflowStep(workflow.id, "Todo", 0);
      const inProgressStep = await apiClient.createWorkflowStep(workflow.id, "In Progress", 1, {
        is_start_step: true,
      });
      await apiClient.createWorkflowStep(workflow.id, "Done", 2);

      await apiClient.saveUserSettings({
        workspace_id: seedData.workspaceId,
        workflow_filter_id: workflow.id,
        enable_preview_on_click: false,
      });

      // No plan_mode, no explicit step — should resolve via is_start_step (In Progress)
      await apiClient.createTask(seedData.workspaceId, "Start Step Task", {
        workflow_id: workflow.id,
      });

      const kanban = new KanbanPage(testPage);
      await kanban.goto();

      const card = kanban.taskCardInColumn("Start Step Task", inProgressStep.id);
      await expect(card).toBeVisible({ timeout: 10_000 });
    });

    test("plan mode task lands in first step (Todo), ignoring start step", async ({
      testPage,
      apiClient,
      seedData,
    }) => {
      const workflow = await apiClient.createWorkflow(
        seedData.workspaceId,
        "Plan Mode Start Step Workflow",
      );
      const todoStep = await apiClient.createWorkflowStep(workflow.id, "Todo", 0);
      await apiClient.createWorkflowStep(workflow.id, "In Progress", 1, {
        is_start_step: true,
      });
      await apiClient.createWorkflowStep(workflow.id, "Done", 2);

      await apiClient.saveUserSettings({
        workspace_id: seedData.workspaceId,
        workflow_filter_id: workflow.id,
        enable_preview_on_click: false,
      });

      // plan_mode: true — should ignore is_start_step and use position 0 (Todo)
      await apiClient.createTask(seedData.workspaceId, "Plan Mode Start Step Task", {
        workflow_id: workflow.id,
        plan_mode: true,
      });

      const kanban = new KanbanPage(testPage);
      await kanban.goto();

      const card = kanban.taskCardInColumn("Plan Mode Start Step Task", todoStep.id);
      await expect(card).toBeVisible({ timeout: 10_000 });
    });
  });

  /**
   * Full plan-mode-to-agent lifecycle:
   *
   * Workflow: Todo (on_turn_start: move_to_next) → In Progress (on_turn_complete: move_to_next) → Done
   *
   * 1. Create task via dialog in plan mode (title only, no description)
   * 2. Session page opens with plan panel visible
   * 3. Toggle off plan mode → plan panel disappears
   * 4. Send a delayed mock script to start the agent
   * 5. on_turn_start fires on Todo → task moves to In Progress (assert before response)
   * 6. Agent responds after 3s delay → on_turn_complete fires → task moves to Done
   */
  test("plan mode to agent: create in plan mode, toggle off, send message, cascades through steps", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Plan To Agent Workflow");

    const todoStep = await apiClient.createWorkflowStep(workflow.id, "Todo", 0);
    const inProgressStep = await apiClient.createWorkflowStep(workflow.id, "In Progress", 1);
    await apiClient.createWorkflowStep(workflow.id, "Done", 2);

    // Todo: move to next step when agent turn starts
    await apiClient.updateWorkflowStep(todoStep.id, {
      events: {
        on_turn_start: [{ type: "move_to_next" }],
      },
    });

    // In Progress: move to next step when agent turn completes
    await apiClient.updateWorkflowStep(inProgressStep.id, {
      events: {
        on_turn_complete: [{ type: "move_to_next" }],
      },
    });

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      enable_preview_on_click: false,
    });

    // --- Create task via dialog in plan mode ---
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    // Fill title only — no description triggers plan mode as the primary submit action
    await testPage.getByTestId("task-title-input").fill("Plan To Agent Task");

    // The default submit button shows "Start Plan Mode" when there's no description.
    // It's a type="submit" button so clicking it submits the form.
    const submitBtn = dialog.getByRole("button", { name: /Start Plan Mode/ });
    await expect(submitBtn).toBeEnabled({ timeout: 10_000 });
    await submitBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    // Navigates to session page with plan layout
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Plan panel is visible (plan layout was activated)
    await expect(session.planPanel).toBeVisible({ timeout: 10_000 });

    // Stepper shows Todo as current step (task starts at position 0)
    await expect(session.stepperStep("Todo")).toHaveAttribute("aria-current", "step", {
      timeout: 10_000,
    });

    // --- Toggle off plan mode ---
    await session.togglePlanMode();
    await expect(session.planPanel).not.toBeVisible({ timeout: 10_000 });

    // --- Send message to start the agent ---
    // Use script mode with a delay so we can observe the In Progress intermediate state.
    // on_turn_start fires immediately → task moves to In Progress.
    // The 3s delay gives us time to assert before on_turn_complete fires.
    await session.sendMessage('e2e:delay(3000)\ne2e:message("delayed mock response")');

    // on_turn_start fires on Todo → task moves to In Progress
    await expect(session.stepperStep("In Progress")).toHaveAttribute("aria-current", "step", {
      timeout: 15_000,
    });

    // Agent responds after the delay
    await expect(session.chat.getByText("delayed mock response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });

    // on_turn_complete fires on In Progress → task moves to Done
    await expect(session.stepperStep("Done")).toHaveAttribute("aria-current", "step", {
      timeout: 15_000,
    });

    // Session transitions to idle
    await expect(session.idleInput()).toBeVisible({ timeout: 15_000 });
  });

  /**
   * Plan mode bypasses is_start_step — task lands in Backlog (position 0):
   *
   * Workflow: Backlog → In Progress [start] (auto_start_agent) → Review → Done
   *
   * 1. Create task via dialog in plan mode (title only, no description)
   * 2. Session page opens with plan panel visible
   * 3. Task lands in Backlog, not In Progress — plan mode ignores is_start_step
   */
  test("plan mode with start step: task lands in Backlog, not In Progress", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Plan Mode Dev Workflow");

    const backlogStep = await apiClient.createWorkflowStep(workflow.id, "Backlog", 0);
    await apiClient.createWorkflowStep(workflow.id, "In Progress", 1, { is_start_step: true });
    await apiClient.createWorkflowStep(workflow.id, "Review", 2);
    await apiClient.createWorkflowStep(workflow.id, "Done", 3);

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      enable_preview_on_click: false,
    });

    // --- Create task via dialog in plan mode ---
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    // Fill title only — "Start Plan Mode" becomes the default submit action
    await testPage.getByTestId("task-title-input").fill("Plan Dev Task");

    const submitBtn = dialog.getByRole("button", { name: /Start Plan Mode/ });
    await expect(submitBtn).toBeEnabled({ timeout: 10_000 });
    await submitBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    // Navigates to session page with plan layout
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Plan panel is visible
    await expect(session.planPanel).toBeVisible({ timeout: 10_000 });

    // Stepper shows Backlog as current step — plan mode ignores is_start_step
    await expect(session.stepperStep("Backlog")).toHaveAttribute("aria-current", "step", {
      timeout: 10_000,
    });

    // Task card is in the Backlog column on the kanban board
    await testPage.goto("/");
    await kanban.board.waitFor({ state: "visible" });
    const card = kanban.taskCardInColumn("Plan Dev Task", backlogStep.id);
    await expect(card).toBeVisible({ timeout: 10_000 });
  });
});
