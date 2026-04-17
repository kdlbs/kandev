import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

/** Mock response text from the simple-message scenario */
const SIMPLE_MOCK_RESPONSE = "This is a simple mock response for e2e testing.";

/**
 * Test that navigating to a task without sessions does NOT show messages
 * from the previously viewed task's session.
 *
 * Reproduces bug: when activeSessionId holds an old session ID and the user
 * navigates to a new task (created with start_agent=false), the chat panel
 * was incorrectly showing messages from the old session.
 */
test.describe("Session isolation", () => {
  test("navigating to session-less task does not show messages from previous task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    // 1. Create a task WITH an agent session that will have messages
    const taskWithSession = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Task With Messages",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 2. Navigate to the task with session and wait for agent to complete
    await testPage.goto(`/t/${taskWithSession.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // 3. Verify there are messages in the chat (agent has responded)
    const chatPanel = testPage.getByTestId("session-chat");
    await expect(chatPanel.getByText(SIMPLE_MOCK_RESPONSE)).toBeVisible({ timeout: 10_000 });

    // 4. Create a task WITHOUT an agent session (start_agent=false)
    const taskWithoutSession = await apiClient.createTask(
      seedData.workspaceId,
      "Task Without Session",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 5. Navigate to the session-less task directly via URL
    await testPage.goto(`/t/${taskWithoutSession.id}`);
    await session.waitForLoad();

    // 6. The chat panel should NOT show the message from the previous task
    // This is the core assertion - we're testing that messages don't leak
    await expect(chatPanel.getByText(SIMPLE_MOCK_RESPONSE)).not.toBeVisible({ timeout: 5_000 });

    // 7. Also verify the task title in the page matches the new task
    // (ensuring we actually navigated to the correct task)
    await expect(testPage.getByRole("link", { name: "Task Without Session" })).toBeVisible({
      timeout: 5_000,
    });
  });

  test("switching tasks via sidebar does not show messages from previous task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    // 1. Create first task with agent session
    const taskA = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "First Task A",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 2. Create second task with a DIFFERENT agent session (different message)
    const taskB = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Second Task B",
      seedData.agentProfileId,
      {
        description: "/e2e:read-and-edit",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 3. Navigate to first task and wait for agent to complete
    await testPage.goto(`/t/${taskA.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // 4. Verify mock response message is visible for first task
    const chatPanel = testPage.getByTestId("session-chat");
    await expect(chatPanel.getByText(SIMPLE_MOCK_RESPONSE)).toBeVisible({ timeout: 10_000 });

    // 5. Click on second task in the sidebar to switch
    await session.clickTaskInSidebar("Second Task B");

    // 6. Wait for navigation to complete
    await expect(testPage).toHaveURL((url) => url.pathname.includes(`/t/${taskB.id}`), {
      timeout: 10_000,
    });
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // 7. The chat should NOT show messages from the first task's session
    // (the "simple-message" text should not appear for task B which uses "read-and-edit")
    await expect(chatPanel.getByText(SIMPLE_MOCK_RESPONSE)).not.toBeVisible({ timeout: 5_000 });

    // 8. Verify we're on the correct task
    await expect(testPage.getByRole("link", { name: "Second Task B" })).toBeVisible({
      timeout: 5_000,
    });
  });
});
