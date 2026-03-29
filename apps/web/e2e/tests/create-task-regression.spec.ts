import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";

const START_AGENT_TEST_ID = "submit-start-agent";
const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];
const START_ENABLED_TIMEOUT = 30_000;

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
    // Extract task ID from the URL: /t/<taskId>
    const { sessions } = await apiClient.listTaskSessions(await getTaskIdFromPage(testPage));
    expect(sessions.length).toBe(1);
    expect(DONE_STATES).toContain(sessions[0].state);

    // Verify task environment was created
    const env = await apiClient.getTaskEnvironment(await getTaskIdFromPage(testPage));
    expect(env).not.toBeNull();
    expect(env!.task_id).toBeTruthy();
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
