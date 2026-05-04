import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

const START_AGENT_TEST_ID = "submit-start-agent";
const START_ENABLED_TIMEOUT = 30_000;
const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

test.describe("Task creation", () => {
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
  });

  test("dialog remembers selections after creating a task", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // First: create a task so localStorage selections get persisted
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
              display_name: profile.agent_display_name,
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

    // Click "Start task" — the agent starts, the dialog closes, we stay on kanban
    await startBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    // The new task card appears on the kanban board (pushed via WS)
    const card = kanban.taskCardByTitle("Start Agent Task");
    await expect(card).toBeVisible({ timeout: 10_000 });

    // Clicking the card fetches the session and navigates to /t/<taskId>
    await card.click();
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

    // Navigate to the task session
    const card = kanban.taskCardByTitle("Regression Task");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();
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
