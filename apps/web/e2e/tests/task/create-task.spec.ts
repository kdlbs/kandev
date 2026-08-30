import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

// Exercises the regular task-create dialog (New Task in the sidebar), so run
// with the office feature disabled.
useRegularMode();

const START_AGENT_TEST_ID = "submit-start-agent";
const START_ENABLED_TIMEOUT = 30_000;
const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

test.describe("Task creation", () => {
  test("hidden workflow task detail does not leak into standard task creation", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const hidden = await apiClient.e2eCreateHiddenWorkflow(
      seedData.workspaceId,
      "Hidden task-detail workflow",
    );
    const hiddenStart = await apiClient.createWorkflowStep(hidden.id, "Improve", 0, {
      is_start_step: true,
    });
    const sourceTask = await apiClient.createTask(seedData.workspaceId, "Hidden source task", {
      description: "Source task in the hidden workflow",
      workflow_id: hidden.id,
      workflow_step_id: hiddenStart.id,
    });

    await testPage.goto(`/t/${sourceTask.id}`);
    await testPage.getByTestId("create-task-button").first().click();

    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    await dialog.getByTestId("task-title-input").fill("Visible workflow task");
    await dialog.getByTestId("task-description-input").fill("Created from hidden task detail");
    await expect(dialog.getByTestId(START_AGENT_TEST_ID)).toBeEnabled({
      timeout: START_ENABLED_TIMEOUT,
    });
    await dialog.getByTestId("submit-start-agent-chevron").click();
    await testPage.getByTestId("submit-create-without-agent").click();

    await expect
      .poll(() => getTaskIdFromPage(testPage), { timeout: 15_000 })
      .not.toBe(sourceTask.id);
    const createdTaskId = await getTaskIdFromPage(testPage);
    const createdTask = await apiClient.getTask(createdTaskId);
    expect(createdTask.workflow_step_id).toBe(seedData.startStepId);
  });

  test("selects the single visible workflow when hidden workflows are loaded", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.e2eCreateHiddenWorkflow(seedData.workspaceId, "Hidden system workflow");
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: "",
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();

    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.getByTestId("workflow-selector-trigger")).toHaveCount(0);
  });

  test("dialog pre-selects repository, branch, and agent profile from seed data", async ({
    testPage,
  }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    // Fill title + description to trigger the "Start task" split button (showStartTask)
    await testPage.getByTestId("task-title-input").fill("Pre-select Test");
    await testPage.getByTestId("task-description-input").fill("testing pre-selections");

    // Wait for the submit button to be enabled — confirms all selections resolved
    const startBtn = testPage.getByTestId(START_AGENT_TEST_ID);
    await expect(startBtn).toBeEnabled({ timeout: START_ENABLED_TIMEOUT });

    // Verify the pre-seeded selections are displayed in the dialog selectors.
    // The dialog uses chip-style repo/branch pills since the multi-repo refactor.
    await expect(testPage.getByTestId("repo-chip-trigger").first()).toContainText("E2E Repo");
    await expect(testPage.getByTestId("branch-chip-trigger").first()).toContainText("main");
    // Agent profile name varies — just verify it's not the empty placeholder
    await expect(testPage.getByTestId("agent-profile-selector")).not.toContainText(
      "Select agent...",
    );

    const repositorySelector = testPage.getByTestId("repo-chip-trigger").first();
    await repositorySelector.click();
    const repositoryListbox = testPage.getByRole("listbox");
    const repositoryOptions = repositoryListbox.getByRole("option");
    await expect(repositoryOptions.first()).toHaveAttribute("aria-selected", "true");
    await expect(repositoryOptions.first()).toHaveClass(/bg-card/);
    await testPage.keyboard.press("Escape");

    const agentSelector = testPage.getByTestId("agent-profile-selector");
    await agentSelector.click();
    const agentListbox = testPage.getByRole("listbox");
    const agentOptions = agentListbox.getByRole("option");
    await expect(agentOptions.first()).toHaveAttribute("aria-selected", "true");
    await expect(agentOptions.first()).toHaveClass(/bg-card/);
    await testPage.keyboard.press("Escape");

    const branchSelector = testPage.getByTestId("branch-chip-trigger").first();
    await branchSelector.click();
    const branchListbox = testPage.getByRole("listbox");
    const branchOptions = branchListbox.getByRole("option");
    await expect(branchOptions.first()).toHaveAttribute("aria-selected", "true");
    await expect(branchOptions.first()).toHaveClass(/bg-card/);
  });

  test("dialog remembers selections after creating a task", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // First: create a task so its selections are persisted to user settings.
    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    await testPage.getByTestId("task-title-input").fill("First Task");
    await testPage.getByTestId("task-description-input").fill("/e2e:simple-message");

    const startBtn = testPage.getByTestId(START_AGENT_TEST_ID);
    await expect(startBtn).toBeEnabled({ timeout: START_ENABLED_TIMEOUT });
    await startBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    // Second: open the dialog again and verify selections persist
    await kanban.createTaskButton.first().click();
    await expect(dialog).toBeVisible();

    await testPage.getByTestId("task-title-input").fill("Second Task");
    await testPage.getByTestId("task-description-input").fill("checking persistence");

    const startBtn2 = testPage.getByTestId(START_AGENT_TEST_ID);
    await expect(startBtn2).toBeEnabled({ timeout: START_ENABLED_TIMEOUT });

    // The same repo, branch, and agent profile should still be selected
    await expect(testPage.getByTestId("repo-chip-trigger").first()).toContainText("E2E Repo");
    await expect(testPage.getByTestId("branch-chip-trigger").first()).toContainText("main");
    await expect(testPage.getByTestId("agent-profile-selector")).not.toContainText(
      "Select agent...",
    );
  });

  test("uses the remembered workflow instead of the board filter", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const devWorkflow = await apiClient.createWorkflow(seedData.workspaceId, "Dev", "simple");
    const devSteps = await apiClient.listWorkflowSteps(devWorkflow.id);
    const devStartStep = devSteps.steps.find((step) => step.is_start_step) ?? devSteps.steps[0];
    if (!devStartStep) throw new Error("Dev workflow has no start step");

    try {
      await apiClient.createTask(seedData.workspaceId, "Remembered Dev task", {
        workflow_id: devWorkflow.id,
        workflow_step_id: devStartStep.id,
      });
      const { settings } = await apiClient.getUserSettings();
      expect(
        settings.task_create_last_used?.workflow_ids_by_workspace?.[seedData.workspaceId],
      ).toBe(devWorkflow.id);

      // Keep the board filtered to the seeded workflow while remembering a
      // different workflow for task creation.
      await apiClient.saveUserSettings({
        workspace_id: seedData.workspaceId,
        workflow_filter_id: seedData.workflowId,
      });

      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      await kanban.createTaskButton.first().click();

      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();
      await expect(dialog.getByTestId("workflow-selector-trigger")).toContainText("Dev");
    } finally {
      await apiClient.deleteWorkflow(devWorkflow.id).catch(() => {});
    }
  });

  test("keeps remembered workflows isolated by workspace after reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    let devWorkflowId = "";
    let workspaceBId = "";

    try {
      const devWorkflow = await apiClient.createWorkflow(seedData.workspaceId, "Dev", "simple");
      devWorkflowId = devWorkflow.id;
      const devSteps = await apiClient.listWorkflowSteps(devWorkflow.id);
      const devStartStep = devSteps.steps.find((step) => step.is_start_step) ?? devSteps.steps[0];
      if (!devStartStep) throw new Error("Dev workflow has no start step");

      const workspaceB = await apiClient.createWorkspace("Workflow Memory B");
      workspaceBId = workspaceB.id;
      const supportWorkflow = await apiClient.createWorkflow(workspaceB.id, "Support", "simple");
      const otherWorkflow = await apiClient.createWorkflow(workspaceB.id, "Other", "simple");
      const supportSteps = await apiClient.listWorkflowSteps(supportWorkflow.id);
      const supportStartStep =
        supportSteps.steps.find((step) => step.is_start_step) ?? supportSteps.steps[0];
      if (!supportStartStep) throw new Error("Support workflow has no start step");

      await apiClient.createTask(seedData.workspaceId, "Remembered Dev task", {
        workflow_id: devWorkflow.id,
        workflow_step_id: devStartStep.id,
      });
      await apiClient.createTask(workspaceB.id, "Remembered Support task", {
        workflow_id: supportWorkflow.id,
        workflow_step_id: supportStartStep.id,
      });
      const { settings } = await apiClient.getUserSettings();
      expect(settings.task_create_last_used?.workflow_ids_by_workspace).toMatchObject({
        [seedData.workspaceId]: devWorkflow.id,
        [workspaceB.id]: supportWorkflow.id,
      });

      await apiClient.saveUserSettings({
        workspace_id: seedData.workspaceId,
        workflow_filter_id: seedData.workflowId,
      });

      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      await kanban.createTaskButton.first().click();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();
      await expect(dialog.getByTestId("workflow-selector-trigger")).toContainText("Dev");
      await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
      await expect(dialog).not.toBeVisible();

      await testPage.getByTestId("sidebar-workspace-trigger").click();
      await testPage.getByTestId(`sidebar-workspace-item-${workspaceB.id}`).click();
      await expect(testPage).toHaveURL(
        (url) => url.pathname === "/" && url.searchParams.get("workspaceId") === workspaceB.id,
      );

      // Give workspace B a conflicting board filter so the remembered Support
      // workflow is the only source that can select it in the dialog.
      await apiClient.saveUserSettings({
        workspace_id: workspaceB.id,
        workflow_filter_id: otherWorkflow.id,
      });
      await testPage.reload();
      await kanban.board.waitFor({ state: "visible" });
      await kanban.createTaskButton.first().click();
      await expect(dialog).toBeVisible();
      await expect(dialog.getByTestId("workflow-selector-trigger")).toContainText("Support");
      await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
      await expect(dialog).not.toBeVisible();

      // Reload workspace A again to prove the two memories do not overwrite
      // each other when the active workspace changes.
      await testPage.getByTestId("sidebar-workspace-trigger").click();
      await testPage.getByTestId(`sidebar-workspace-item-${seedData.workspaceId}`).click();
      await expect(testPage).toHaveURL(
        (url) =>
          url.pathname === "/" && url.searchParams.get("workspaceId") === seedData.workspaceId,
      );
      await apiClient.saveUserSettings({
        workspace_id: seedData.workspaceId,
        workflow_filter_id: seedData.workflowId,
      });
      await testPage.reload();
      await kanban.board.waitFor({ state: "visible" });
      await kanban.createTaskButton.first().click();
      await expect(dialog).toBeVisible();
      await expect(dialog.getByTestId("workflow-selector-trigger")).toContainText("Dev");
    } finally {
      if (devWorkflowId) await apiClient.deleteWorkflow(devWorkflowId).catch(() => {});
      if (workspaceBId)
        await apiClient.deleteWorkspace(workspaceBId, "Workflow Memory B").catch(() => {});
      await apiClient.saveUserSettings({
        workspace_id: seedData.workspaceId,
        workflow_filter_id: seedData.workflowId,
      });
    }
  });

  test("opens create task dialog from kanban header", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();
  });

  test("explains when a Docker executor has no compatible agent credentials", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const profile = await apiClient.getAgentProfile(seedData.agentProfileId);
    const { agents } = await apiClient.listAgents();
    const agent = agents.find((item) => item.profiles?.some((p) => p.id === profile.id));
    if (!agent) {
      throw new Error(`agent for profile ${profile.id} not found`);
    }
    const dockerExec = await apiClient.createExecutor("E2E Docker Empty Agent", "local_docker");
    const dockerProfile = await apiClient.createExecutorProfile(dockerExec.id, {
      name: "Docker Missing Auth",
      config: {
        image_tag: "kandev-mock-agent:test",
        dockerfile: "FROM busybox\nWORKDIR /workspace\n",
      },
    });

    await testPage.route("**/api/v1/remote-credentials", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          auth_specs: [
            {
              id: agent.name,
              display_name: profile.agentDisplayName,
              methods: [{ method_id: "test-token", type: "env", env_var: "TEST_TOKEN" }],
            },
          ],
        }),
      });
    });

    try {
      const kanban = new KanbanPage(testPage);
      await kanban.goto();

      await kanban.createTaskButton.first().click();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();

      await testPage.getByTestId("task-title-input").fill("Docker Missing Auth Task");
      await testPage.getByTestId("task-description-input").fill("testing docker auth empty state");

      await testPage.getByTestId("executor-profile-selector").click();
      await testPage.getByRole("option", { name: /Docker Missing Auth/i }).click();

      await expect(testPage.getByTestId("agent-profile-empty-state")).toContainText(
        "No compatible agent profiles",
      );
      await expect(testPage.getByRole("link", { name: "Configure credentials" })).toHaveAttribute(
        "href",
        `/settings/executors/${dockerProfile.id}`,
      );
      await expect(dialog).toContainText(
        "A Docker container will be created from the selected base branch and checked out on a task branch.",
      );
      await expect(testPage.getByTestId(START_AGENT_TEST_ID)).toBeDisabled();
    } finally {
      await apiClient.deleteExecutor(dockerExec.id).catch(() => {});
    }
  });

  test("ignores a stale browser profile and selects a valid backend profile", async ({
    testPage,
  }) => {
    // A stale value from an older browser build must not participate in
    // selection. The dialog uses backend settings and then falls back through
    // the workspace default / first-profile chain.
    await testPage.addInitScript(() => {
      localStorage.setItem(
        "kandev.dialog.lastAgentProfileId",
        JSON.stringify("00000000-0000-0000-0000-000000000000"),
      );
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    await testPage.getByTestId("task-title-input").fill("Stale LS Task");
    await testPage.getByTestId("task-description-input").fill("stale localStorage repro");

    // The bug surfaces as the "No compatible agent profiles" empty state.
    await expect(testPage.getByTestId("agent-profile-empty-state")).toHaveCount(0);

    // Auto-select must resolve to a real profile, and the start button must
    // become enabled - these are the user-visible symptoms of the fix.
    await expect(testPage.getByTestId(START_AGENT_TEST_ID)).toBeEnabled({
      timeout: START_ENABLED_TIMEOUT,
    });
    await expect(testPage.getByTestId("agent-profile-selector")).not.toContainText("Select agent");
  });

  test("can fill in task title and description", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    const titleInput = testPage.getByTestId("task-title-input");
    await titleInput.fill("My E2E Test Task");
    await expect(titleInput).toHaveValue("My E2E Test Task");

    const descInput = testPage.getByTestId("task-description-input");
    await descInput.fill("This is a test description");
    await expect(descInput).toHaveValue("This is a test description");
  });

  test("start agent: creates task, starts session, navigates to session", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    // Fill in title and description — description enables the "Start task" button
    await testPage.getByTestId("task-title-input").fill("Start Agent Task");
    await testPage.getByTestId("task-description-input").fill("/e2e:simple-message");

    // The dialog auto-selects the E2E Repo (first available repository) and the main branch.
    // Wait for the button to become enabled (repo + branch + agent profile all resolved).
    // Under load the branch/profile resolution can take a moment.
    const startBtn = testPage.getByTestId(START_AGENT_TEST_ID);
    await expect(startBtn).toBeEnabled({ timeout: START_ENABLED_TIMEOUT });

    // Click "Start task" — the agent starts, the dialog closes, and sidebar creation
    // focuses the newly created task immediately.
    await startBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Default layout: sidebar, terminal, and file tree are all visible
    await expect(session.sidebar).toBeVisible();
    await expect(session.terminal).toBeVisible();
    await expect(session.files).toBeVisible();
    await expect(session.chat).toBeVisible();

    // Task title appears in the sidebar
    await expect(session.taskInSidebar("Start Agent Task")).toBeVisible();

    // The mock agent's simple-message scenario emits this response text —
    // waiting for it confirms the agent ran and completed its turn.
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });

    // The first user message (task description) is visible in the chat
    await expect(session.chat.getByText("/e2e:simple-message")).toBeVisible();

    // Session transitions to idle — input placeholder changes from "Queue instructions..."
    await expect(session.idleInput()).toBeVisible({ timeout: 15_000 });

    // Sidebar shows the task under "Turn Finished" section — this uses kanban tasks which update
    // via task.updated WS event. Check this first to give more time for the stepper data
    // (session.state_changed) to propagate through the store.
    await expect(session.sidebarSection("Turn Finished")).toBeVisible({ timeout: 15_000 });

    // After the agent completes, the workflow step transitions from "In Progress" to "Review".
    // The step update arrives via WS (task.updated and session.state_changed events) which may
    // take a moment to propagate through the store.
    await expect(session.stepperStep("Review")).toHaveAttribute("aria-current", "step", {
      timeout: 15_000,
    });
  });

  test("plan mode with MCP tool: creates plan via create_task_plan", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    // Multi-line script: thinking → MCP create plan → text message
    const script = [
      'e2e:thinking("Analyzing task and creating plan...")',
      "e2e:delay(100)",
      'e2e:mcp:kandev:create_task_plan_kandev({"task_id":"{task_id}","content":"## Plan\\n\\n1. Analyze requirements\\n2. Implement solution\\n3. Write tests","title":"Implementation Plan"})',
      "e2e:delay(100)",
      'e2e:message("I\'ve created an implementation plan for this task.")',
    ].join("\n");

    await testPage.getByTestId("task-title-input").fill("Plan MCP Task");
    await testPage.getByTestId("task-description-input").fill(script);

    const startBtn = testPage.getByTestId(START_AGENT_TEST_ID);
    await expect(startBtn).toBeEnabled({ timeout: START_ENABLED_TIMEOUT });

    await testPage.getByTestId("submit-start-agent-chevron").click();
    await expect(testPage.getByTestId("submit-plan-mode")).toBeVisible({ timeout: 5_000 });
    await testPage.getByTestId("submit-plan-mode").click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    await expect(testPage).toHaveURL(/\/t\/.*layout=plan/, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Plan panel visible (plan layout)
    await expect(session.planPanel).toBeVisible({ timeout: 10_000 });

    // The first user message (script content) is visible in the chat with plan mode badge
    await expect(session.chat.getByText("e2e:thinking", { exact: false })).toBeVisible({
      timeout: 15_000,
    });
    await expect(session.planModeBadge()).toBeVisible();

    // Agent completion text visible in chat (use .last() because the user message
    // paragraph also contains this text inside the e2e:message(...) script line)
    await expect(
      session.chat.getByText("I've created an implementation plan for this task.").last(),
    ).toBeVisible({ timeout: 30_000 });

    // Plan content appears in plan panel (created via real MCP call to create_task_plan)
    await expect(session.planPanel.getByText("Analyze requirements", { exact: false })).toBeVisible(
      {
        timeout: 15_000,
      },
    );
    await expect(session.planPanel.getByText("Implement solution", { exact: false })).toBeVisible();
    await expect(session.planPanel.getByText("Write tests", { exact: false })).toBeVisible();

    // Session transitions to idle — plan mode input placeholder visible
    await expect(session.planModeInput()).toBeVisible({ timeout: 15_000 });

    // After the agent completes, the workflow step transitions to "Review"
    await expect(session.stepperStep("Review")).toHaveAttribute("aria-current", "step", {
      timeout: 10_000,
    });

    // Sidebar shows the task under "Turn Finished" section
    await expect(session.sidebarSection("Turn Finished")).toBeVisible();
  });

  test("start task in plan mode: opens session with plan panel visible", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    await testPage.getByTestId("task-title-input").fill("Plan Mode Task");
    await testPage.getByTestId("task-description-input").fill("/e2e:simple-message");

    // Wait for the main submit button to be enabled (repo + agent profile resolved),
    // then open the dropdown chevron to reveal the "Start task in plan mode" option.
    const startBtn = testPage.getByTestId(START_AGENT_TEST_ID);
    await expect(startBtn).toBeEnabled({ timeout: START_ENABLED_TIMEOUT });

    // The split-button group wraps "Start task" + a chevron-only dropdown trigger.
    // Click the chevron to open the dropdown.
    await testPage.getByTestId("submit-start-agent-chevron").click();

    const planModeBtn = testPage.getByTestId("submit-plan-mode");
    await expect(planModeBtn).toBeVisible({ timeout: 5_000 });
    await planModeBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    // activatePlanMode navigates to /t/<id>?layout=plan which applies the plan preset
    await expect(testPage).toHaveURL(/\/t\/.*layout=plan/, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Plan panel is visible — the layout preset shows it instead of the default file tree
    await expect(session.planPanel).toBeVisible({ timeout: 10_000 });

    // Chat is still accessible in plan mode
    await expect(session.chat).toBeVisible();

    // Wait for the agent to complete so messages are loaded
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });

    // The first user message (task description) is visible with plan mode badge
    await expect(session.chat.getByText("/e2e:simple-message")).toBeVisible();
    await expect(session.planModeBadge()).toBeVisible();

    // Session transitions to idle — plan mode input placeholder visible
    await expect(session.planModeInput()).toBeVisible({ timeout: 15_000 });

    // After the agent completes, the workflow step transitions to "Review"
    await expect(session.stepperStep("Review")).toHaveAttribute("aria-current", "step", {
      timeout: 10_000,
    });

    // Sidebar shows the task under "Turn Finished" section
    await expect(session.sidebarSection("Turn Finished")).toBeVisible();
  });
});

/**
 * Regression tests for the create-task flow after TaskEnvironment model changes.
 * Ensures that task creation, agent launch, and session navigation still work
 * correctly with the new per-task environment layer.
 */
test.describe("Create task regression", () => {
  test("create and start agent: session completes with task environment created", async ({
    testPage,
    apiClient,
  }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    await testPage.getByTestId("task-title-input").fill("Regression Task");
    await testPage.getByTestId("task-description-input").fill("/e2e:simple-message");

    const startBtn = testPage.getByTestId(START_AGENT_TEST_ID);
    await expect(startBtn).toBeEnabled({ timeout: START_ENABLED_TIMEOUT });
    await startBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Wait for mock agent to complete its turn
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });
    await expect(session.idleInput()).toBeVisible({ timeout: 15_000 });

    // Verify task has exactly one session
    const taskId = await getTaskIdFromPage(testPage);
    const { sessions } = await apiClient.listTaskSessions(taskId);
    expect(sessions.length).toBe(1);
    expect(DONE_STATES).toContain(sessions[0].state);

    // Verify task environment was created
    const env = await apiClient.getTaskEnvironment(taskId);
    expect(env).not.toBeNull();
    expect(env!.task_id).toBe(taskId);
    expect(env!.status).toBe("ready");
  });

  test("create task via API: session and environment link correctly", async ({
    apiClient,
    seedData,
  }) => {
    // Create task with agent via REST API (bypasses UI)
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "API Regression Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // Wait for agent to finish (poll session state)
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for session to finish" },
      )
      .toBe(true);

    // Verify task environment was created and linked
    const env = await apiClient.getTaskEnvironment(task.id);
    expect(env).not.toBeNull();
    expect(env!.task_id).toBe(task.id);

    // Verify session has task_environment_id set
    const { sessions } = await apiClient.listTaskSessions(task.id);
    expect(sessions[0].task_environment_id).toBe(env!.id);
  });
});

/** Extract task ID from the current page URL (/t/<taskId>). */
async function getTaskIdFromPage(page: import("@playwright/test").Page): Promise<string> {
  const url = page.url();
  const match = url.match(/\/t\/([^/?]+)/);
  if (!match) throw new Error(`Cannot extract task ID from URL: ${url}`);
  return match[1];
}
