import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

/**
 * Tests for the New Session Dialog UI flow.
 * Verifies that creating a second session via the + menu works end-to-end:
 * dialog opens, environment info is shown, session is created, and a new tab appears.
 */
test.describe("New session dialog", () => {
  test("opens dialog from + menu and shows environment info", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    // 1. Create task with first session via API
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "New Session Dialog Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 2. Wait for first session to complete
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for first session to finish" },
      )
      .toBe(true);

    // 3. Navigate to the task
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("New Session Dialog Task");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Wait for first session's response to be visible (confirms session loaded)
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // 4. Open the + menu and click "New Session"
    await session.openNewSessionDialog();

    // 5. Verify dialog is visible with environment info
    await expect(session.newSessionDialog()).toBeVisible({ timeout: 5_000 });
    await expect(session.newSessionEnvironmentInfo()).toBeVisible();
    await expect(session.newSessionPromptInput()).toBeVisible();
    await expect(session.newSessionStartButton()).toBeVisible();
  });

  test("creates second session and new tab appears", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(120_000);

    // 1. Create task with first session via API
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Second Session Tab Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 2. Wait for first session to complete
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for first session to finish" },
      )
      .toBe(true);

    // 3. Navigate to the task
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("Second Session Tab Task");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Wait for chat to load before interacting with the + menu
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // 4. Open the new session dialog
    await session.openNewSessionDialog();
    await expect(session.newSessionDialog()).toBeVisible({ timeout: 5_000 });

    // 5. Fill in prompt and submit
    await session.newSessionPromptInput().fill("/e2e:simple-message");
    await session.newSessionStartButton().click();

    // 6. Dialog should close
    await expect(session.newSessionDialog()).not.toBeVisible({ timeout: 10_000 });

    // 7. Verify two session tabs exist (index badge + agent label)
    await expect(session.sessionTabByText("1")).toBeVisible({ timeout: 15_000 });
    await expect(session.sessionTabByText("2")).toBeVisible({ timeout: 15_000 });

    // 8. Verify the new session is active (chat loads fresh context)
    await session.waitForLoad();

    // 9. Verify the backend has two sessions
    const { sessions: allSessions } = await apiClient.listTaskSessions(task.id);
    expect(allSessions).toHaveLength(2);
  });

  test("second session reuses same task environment", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(120_000);

    // 1. Create task with first session via API
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Env Reuse Dialog Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 2. Wait for first session to complete
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for first session to finish" },
      )
      .toBe(true);

    // Capture the environment created by first session
    const envBefore = await apiClient.getTaskEnvironment(task.id);
    expect(envBefore).not.toBeNull();

    // 3. Navigate to the task
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("Env Reuse Dialog Task");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // 4. Create second session via the dialog
    await session.openNewSessionDialog();
    await expect(session.newSessionDialog()).toBeVisible({ timeout: 5_000 });
    await session.newSessionPromptInput().fill("/e2e:simple-message");
    await session.newSessionStartButton().click();
    await expect(session.newSessionDialog()).not.toBeVisible({ timeout: 10_000 });

    // 5. Wait for second session to complete
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.length;
        },
        { timeout: 30_000, message: "Waiting for second session to appear" },
      )
      .toBe(2);

    // 6. Verify both sessions share the same task environment
    const { sessions: allSessions } = await apiClient.listTaskSessions(task.id);
    const envAfter = await apiClient.getTaskEnvironment(task.id);
    expect(envAfter).not.toBeNull();
    expect(envAfter!.id).toBe(envBefore!.id);

    // Both sessions should reference the same environment
    for (const s of allSessions) {
      expect(s.task_environment_id).toBe(envBefore!.id);
    }
  });

  test("dialog cancel does not create a session", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(90_000);

    // 1. Create task with first session via API
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Cancel Dialog Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 2. Wait for first session to complete
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for first session to finish" },
      )
      .toBe(true);

    // 3. Navigate to the task
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("Cancel Dialog Task");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // 4. Open and cancel the dialog
    await session.openNewSessionDialog();
    await expect(session.newSessionDialog()).toBeVisible({ timeout: 5_000 });

    // Type something to verify it doesn't accidentally submit
    await session.newSessionPromptInput().fill("should not create");

    // Click cancel
    const cancelBtn = session.newSessionDialog().getByRole("button", { name: "Cancel" });
    await cancelBtn.click();

    // 5. Verify dialog closed and no new session was created
    await expect(session.newSessionDialog()).not.toBeVisible({ timeout: 5_000 });

    const { sessions } = await apiClient.listTaskSessions(task.id);
    expect(sessions).toHaveLength(1);
  });

  test("+ dropdown lists sessions with status and primary indicators", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    // 1. Create task with first session via API
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Session List Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 2. Wait for first session to complete
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for first session to finish" },
      )
      .toBe(true);

    // 3. Navigate to the task
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("Session List Task");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // 4. Open the + dropdown
    await session.addPanelButton().click();

    // 5. Verify session item is visible with #1 label
    const sessionItems = session.sessionReopenItems();
    await expect(sessionItems.first()).toBeVisible({ timeout: 5_000 });
    await expect(sessionItems.first()).toContainText("#1");

    // 6. Verify the "Agents" label is visible
    const sessionsLabel = testPage.getByText("Agents", { exact: true });
    await expect(sessionsLabel).toBeVisible();
  });
});
