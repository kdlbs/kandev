import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";

test.describe("Task creation", () => {
  test("opens create task dialog from kanban header", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();
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
    const startBtn = testPage.getByTestId("submit-start-agent");
    await expect(startBtn).toBeEnabled({ timeout: 15_000 });

    // Click "Start task" — the agent starts, the dialog closes, we stay on kanban
    await startBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    // The new task card appears on the kanban board (pushed via WS)
    const card = kanban.taskCardByTitle("Start Agent Task");
    await expect(card).toBeVisible({ timeout: 10_000 });

    // Clicking the card fetches the session and navigates to /s/<sessionId>
    await card.click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

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

    // Sidebar shows the task under "Review" section — this uses kanban tasks which update
    // via task.updated WS event. Check this first to give more time for the stepper data
    // (session.state_changed) to propagate through the store.
    await expect(session.sidebarSection("Review")).toBeVisible({ timeout: 15_000 });

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
      'e2e:mcp:kandev:create_task_plan({"task_id":"{task_id}","content":"## Plan\\n\\n1. Analyze requirements\\n2. Implement solution\\n3. Write tests","title":"Implementation Plan"})',
      "e2e:delay(100)",
      'e2e:message("I\'ve created an implementation plan for this task.")',
    ].join("\n");

    await testPage.getByTestId("task-title-input").fill("Plan MCP Task");
    await testPage.getByTestId("task-description-input").fill(script);

    const startBtn = testPage.getByTestId("submit-start-agent");
    await expect(startBtn).toBeEnabled({ timeout: 15_000 });

    await testPage.getByTestId("submit-start-agent-chevron").click();
    await expect(testPage.getByTestId("submit-plan-mode")).toBeVisible({ timeout: 5_000 });
    await testPage.getByTestId("submit-plan-mode").click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    await expect(testPage).toHaveURL(/\/s\/.*layout=plan/, { timeout: 15_000 });

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

    // Sidebar shows the task under "Review" section
    await expect(session.sidebarSection("Review")).toBeVisible();
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
    const startBtn = testPage.getByTestId("submit-start-agent");
    await expect(startBtn).toBeEnabled({ timeout: 15_000 });

    // The split-button group wraps "Start task" + a chevron-only dropdown trigger.
    // Click the chevron to open the dropdown.
    await testPage.getByTestId("submit-start-agent-chevron").click();

    const planModeBtn = testPage.getByTestId("submit-plan-mode");
    await expect(planModeBtn).toBeVisible({ timeout: 5_000 });
    await planModeBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    // activatePlanMode navigates to /s/<id>?layout=plan which applies the plan preset
    await expect(testPage).toHaveURL(/\/s\/.*layout=plan/, { timeout: 15_000 });

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

    // Sidebar shows the task under "Review" section
    await expect(session.sidebarSection("Review")).toBeVisible();
  });
});
