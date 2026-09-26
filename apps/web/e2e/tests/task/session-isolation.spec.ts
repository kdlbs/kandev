import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

/** Mock response text from the simple-message scenario */
const SIMPLE_MOCK_RESPONSE = "This is a simple mock response for e2e testing.";

async function waitForAgentMessage(
  apiClient: {
    listSessionMessages: (
      sessionId: string,
    ) => Promise<{ messages: Array<{ content: string; author_type: string }> }>;
  },
  sessionId: string,
  content: string,
) {
  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        return messages.some(
          (message) => message.author_type === "agent" && message.content.includes(content),
        );
      },
      { timeout: 45_000 },
    )
    .toBe(true);
}

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
    test.setTimeout(90_000);

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
    // The isolation assertion only needs the response to be persisted. The
    // composer can remain busy while the mock response is already available,
    // and waiting for its idle placeholder makes this test depend on a
    // separate terminal-state transition.
    await waitForAgentMessage(apiClient, taskWithSession.session_id!, SIMPLE_MOCK_RESPONSE);
    await testPage.reload();
    await session.waitForLoad();

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
    await expect(testPage.getByTestId("task-topbar-title")).toHaveText("Task Without Session", {
      timeout: 5_000,
    });

    // 6. The chat panel should NOT show the message from the previous task
    // This is the core assertion - we're testing that messages don't leak
    await expect(chatPanel.getByText(SIMPLE_MOCK_RESPONSE)).not.toBeVisible({ timeout: 5_000 });
  });

  test("switching tasks via sidebar does not show messages from previous task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

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

    // 2. Navigate to first task and wait for agent to complete
    await testPage.goto(`/t/${taskA.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });

    // 3. Verify mock response message is visible for first task
    const chatPanel = testPage.getByTestId("session-chat");
    await expect(chatPanel.getByText(SIMPLE_MOCK_RESPONSE)).toBeVisible({ timeout: 10_000 });

    // 4. Create second task AFTER first agent completes to avoid concurrent
    //    git operations on the same repo (both agents do git checkout during
    //    environment preparation, causing index.lock conflicts).
    //    Task B uses `multi-turn` (a single quick text emission) rather than a
    //    permission-gated scenario. The isolation assertion below only needs
    //    task B's response to differ from task A's SIMPLE_MOCK_RESPONSE — it
    //    never checks any tool/edit output — and a permission-gated turn (e.g.
    //    read-and-edit) can leave the backend session stuck in RUNNING when its
    //    terminal transition races, so the composer never goes idle here.
    const taskB = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Second Task B",
      seedData.agentProfileId,
      {
        description: "/e2e:multi-turn",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 5. Click on second task in the sidebar to switch
    await session.clickTaskInSidebar("Second Task B");

    // 6. Wait for navigation to complete
    await expect(testPage).toHaveURL((url) => url.pathname.includes(`/t/${taskB.id}`), {
      timeout: 10_000,
    });
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });

    // 7. The chat should NOT show messages from the first task's session
    // (the "simple-message" text should not appear for task B which uses "read-and-edit")
    await expect(chatPanel.getByText(SIMPLE_MOCK_RESPONSE)).not.toBeVisible({ timeout: 5_000 });

    // 8. Verify we're on the correct task
    await expect(testPage.getByTestId("task-topbar-title")).toHaveText("Second Task B", {
      timeout: 5_000,
    });
  });
});
