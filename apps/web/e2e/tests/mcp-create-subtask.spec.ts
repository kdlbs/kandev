import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";

type TaskWithRepos = {
  id: string;
  repositories?: Array<{ repository_id: string }>;
};

const START_AGENT_TEST_ID = "submit-start-agent";
const START_ENABLED_TIMEOUT = 30_000;

test.describe("MCP create_task subtask", () => {
  test("agent creates subtask via MCP create_task with parent_id", async ({ testPage }) => {
    const subtaskTitle = "MCP-subtask-e2e-verify";

    const script = [
      'e2e:thinking("Planning subtasks...")',
      "e2e:delay(100)",
      `e2e:mcp:kandev:create_task({"parent_id":"self","title":"${subtaskTitle}","description":"E2E subtask: verify MCP create_task with parent_id"})`,
      "e2e:delay(100)",
      'e2e:message("Done.")',
    ].join("\n");

    // 1. Create parent task via UI dialog
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    await testPage.getByTestId("task-title-input").fill("MCP Subtask Parent");
    await testPage.getByTestId("task-description-input").fill(script);

    const startBtn = testPage.getByTestId(START_AGENT_TEST_ID);
    await expect(startBtn).toBeEnabled({ timeout: START_ENABLED_TIMEOUT });
    await startBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    // 2. Click the parent task card to navigate to its session
    const parentCard = kanban.taskCardByTitle("MCP Subtask Parent");
    await expect(parentCard).toBeVisible({ timeout: 10_000 });
    await parentCard.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    // 3. Wait for the agent to complete — the MCP create_task call happens during execution
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // 4. Go back to kanban — subtask card should be visible with parent badge
    await kanban.goto();

    const subtaskCard = kanban.taskCardByTitle(subtaskTitle);
    await expect(subtaskCard).toBeVisible({ timeout: 10_000 });
    await expect(subtaskCard.getByText("MCP Subtask Parent")).toBeVisible();
  });

  test("MCP-created subtask inherits parent task repositories", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const subtaskTitle = "Repo-Inherit Subtask E2E";

    const script = [
      'e2e:thinking("Creating subtask...")',
      "e2e:delay(100)",
      `e2e:mcp:kandev:create_task({"parent_id":"self","title":"${subtaskTitle}","description":"E2E subtask: verify repository inheritance"})`,
      "e2e:delay(100)",
      'e2e:message("Done.")',
    ].join("\n");

    // 1. Create parent task via API with an explicit repository. Without the
    //    fix, the MCP handler would not copy repositories to the subtask and
    //    it would run as a repository-less "quick chat" session.
    const parentTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Repo Inherit Parent Task",
      seedData.agentProfileId,
      {
        description: script,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    expect(parentTask.id).toBeTruthy();

    // 2. Navigate to parent task and wait for the agent to complete (which
    //    includes the MCP create_task call that creates the subtask).
    await testPage.goto(`/t/${parentTask.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // 3. Locate the subtask created by the MCP call.
    const allTasks = await apiClient.listTasks(seedData.workspaceId);
    const subtask = allTasks.tasks.find((t) => t.title === subtaskTitle);
    expect(subtask).toBeDefined();

    // 4. Verify the subtask inherited the parent's repository.
    const subtaskRaw = await apiClient.rawRequest("GET", `/api/v1/tasks/${subtask!.id}`);
    const subtaskData = (await subtaskRaw.json()) as TaskWithRepos;
    expect(
      subtaskData.repositories?.length,
      `subtask should have at least 1 repository; got: ${JSON.stringify(subtaskData.repositories)}`,
    ).toBeGreaterThanOrEqual(1);
    expect(
      subtaskData.repositories?.[0]?.repository_id,
      `subtask repository_id should match parent's (${seedData.repositoryId})`,
    ).toBe(seedData.repositoryId);
  });

  test("user creates subtask via sidebar button", async ({ testPage }) => {
    // 1. Create parent task via UI dialog with a simple agent script
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    await testPage.getByTestId("task-title-input").fill("Subtask Button Parent");
    await testPage.getByTestId("task-description-input").fill("/e2e:simple-message");

    const startBtn = testPage.getByTestId(START_AGENT_TEST_ID);
    await expect(startBtn).toBeEnabled({ timeout: START_ENABLED_TIMEOUT });
    await startBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    // 2. Click the parent card to navigate to its session
    const parentCard = kanban.taskCardByTitle("Subtask Button Parent");
    await expect(parentCard).toBeVisible({ timeout: 10_000 });
    await parentCard.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    // 3. Wait for the agent to finish
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // 4. Open the Task split-button chevron and click "New Subtask"
    await testPage.getByTestId("new-task-chevron").click();
    await testPage.getByTestId("new-subtask-button").click();
    const subtaskTitleInput = testPage.getByTestId("subtask-title-input");
    await expect(subtaskTitleInput).toBeVisible();

    // Title should be pre-filled with "Parent / Subtask N" pattern
    await expect(subtaskTitleInput).toHaveValue(/Subtask Button Parent \/ Subtask \d+/);

    // 5. Fill the prompt and submit
    const parentUrl = testPage.url();
    await testPage.getByTestId("subtask-prompt-input").fill("/e2e:simple-message");
    await testPage.getByRole("button", { name: "Create Subtask" }).click();

    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });
    await expect(testPage).not.toHaveURL(parentUrl);

    // 6. Go back to kanban — subtask card should be visible with parent badge
    await kanban.goto();
    const subtaskCard = kanban.taskCardByTitle(/Subtask Button Parent \/ Subtask \d+/);
    await expect(subtaskCard).toBeVisible({ timeout: 10_000 });
  });
});
