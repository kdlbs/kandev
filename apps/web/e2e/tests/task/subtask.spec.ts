import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

const START_AGENT_TEST_ID = "submit-start-agent";
const START_ENABLED_TIMEOUT = 30_000;
const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

type TaskWithRepos = {
  id: string;
  repositories?: Array<{ repository_id: string }>;
};

type SessionInfo = {
  id: string;
  agent_profile_id: string;
  executor_profile_id: string;
  state: string;
};

test.describe("Subtask basics", () => {
  test("subtask badge visible on kanban card", async ({ testPage, apiClient, seedData }) => {
    // Create parent task via API
    const parent = await apiClient.createTask(seedData.workspaceId, "Parent Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    // Create subtask via API with parent_id
    await apiClient.createTask(seedData.workspaceId, "Child Subtask", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      parent_id: parent.id,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // Parent card is visible but does NOT have subtask badge
    const parentCard = kanban.taskCardByTitle("Parent Task");
    await expect(parentCard).toBeVisible({ timeout: 10_000 });
    await expect(parentCard.getByText("Parent Task", { exact: true }).first()).toBeVisible();

    // Subtask card is visible and HAS badge showing parent title
    const subtaskCard = kanban.taskCardByTitle("Child Subtask");
    await expect(subtaskCard).toBeVisible({ timeout: 10_000 });
    // Badge now shows parent task title instead of generic "Subtask"
    await expect(subtaskCard.getByText("Parent Task")).toBeVisible();
  });

  test("create subtask from sidebar header button", async ({ testPage, apiClient, seedData }) => {
    // Create a task with an agent so we have a session to navigate to
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Subtask Parent",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // Navigate to the session page
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Wait for agent to complete
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // Open the Task split-button chevron and click "New Subtask"
    const chevron = testPage.getByTestId("new-task-chevron");
    await expect(chevron).toBeVisible({ timeout: 5_000 });
    await chevron.click();
    await testPage.getByTestId("new-subtask-button").click();

    // The compact NewSubtaskDialog should open with pre-filled title containing numeric suffix
    const titleInput = testPage.getByTestId("subtask-title-input");
    await expect(titleInput).toBeVisible({ timeout: 5_000 });
    await expect(titleInput).toHaveValue(/Subtask Parent \/ Subtask \d+/);

    // Fill prompt and submit
    const promptInput = testPage.getByTestId("subtask-prompt-input");
    await expect(promptInput).toBeVisible();
    await promptInput.fill("/e2e:simple-message");

    const submitBtn = testPage.getByRole("button", { name: "Create Subtask" });
    await submitBtn.click();
    await expect(titleInput).not.toBeVisible({ timeout: 10_000 });

    // After creation, we navigate to the new subtask's session
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    // Verify the subtask card appears on the kanban board with "Subtask" badge
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const subtaskCard = kanban.taskCardByTitle(/Subtask Parent \/ Subtask \d+/);
    await expect(subtaskCard).toBeVisible({ timeout: 10_000 });
  });
});

test.describe("MCP subtask creation", () => {
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

/**
 * Verifies that when an agent creates a subtask via the MCP create_task tool
 * with parent_id="self", the subtask session inherits BOTH the parent session's
 * agent_profile_id AND executor_profile_id.
 */
test.describe("Subtask inheritance", () => {
  test("MCP-created subtask inherits agent profile and executor profile", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const subtaskTitle = "Inherited Profile Subtask E2E";

    const script = [
      'e2e:thinking("Creating subtask...")',
      "e2e:delay(100)",
      `e2e:mcp:kandev:create_task({"parent_id":"self","title":"${subtaskTitle}","description":"E2E subtask: verify executor and agent profile inheritance"})`,
      "e2e:delay(100)",
      'e2e:message("Done.")',
    ].join("\n");

    // 1. Create an executor + profile so executor_profile_id is non-empty on
    //    the parent session — this makes the inheritance assertion meaningful.
    const executor = await apiClient.createExecutor("E2E Inherit Executor", "local_pc");
    const executorProfile = await apiClient.createExecutorProfile(
      executor.id,
      "E2E Inherit Profile",
    );

    // 2. Create parent task via API with explicit executor_profile_id so the
    //    parent session records it. The mock agent runs the MCP script which
    //    creates the subtask with parent_id="self".
    const parentTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Executor Inherit Parent Task",
      seedData.agentProfileId,
      {
        description: script,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        executor_profile_id: executorProfile.id,
      },
    );
    expect(parentTask.id).toBeTruthy();

    // 3. Navigate to the parent task page and wait for the agent to complete.
    await testPage.goto(`/t/${parentTask.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // 4. Confirm the parent session has both profiles set as expected.
    const parentRaw = await apiClient.rawRequest("GET", `/api/v1/tasks/${parentTask.id}/sessions`);
    const parentData = (await parentRaw.json()) as { sessions: SessionInfo[] };
    const parentSession = parentData.sessions[0];
    expect(
      parentSession?.agent_profile_id,
      `agent_profile_id: got ${parentSession?.agent_profile_id}, want ${seedData.agentProfileId}`,
    ).toBe(seedData.agentProfileId);
    expect(
      parentSession?.executor_profile_id,
      `executor_profile_id: got ${parentSession?.executor_profile_id}, want ${executorProfile.id}; full session: ${JSON.stringify(parentSession)}`,
    ).toBe(executorProfile.id);

    // 5. Locate the subtask created by the MCP call.
    const allTasks = await apiClient.listTasks(seedData.workspaceId);
    const subtask = allTasks.tasks.find((t) => t.title === subtaskTitle);
    expect(subtask).toBeDefined();

    // 6. Verify subtask was auto-started: must have at least one session.
    //    Without the fix, autoStartTask finds no agent profile and skips launch →
    //    the subtask remains sessionless.
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(subtask!.id);
          return sessions.length;
        },
        { timeout: 30_000, message: "Subtask should be auto-started with an inherited session" },
      )
      .toBeGreaterThanOrEqual(1);

    // 7. Wait for the subtask session to reach a terminal state.
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(subtask!.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for subtask session to complete" },
      )
      .toBe(true);

    // 8. Verify the subtask session inherited BOTH profiles from the parent —
    //    proving executor/agent profile inheritance worked correctly.
    const subtaskRaw = await apiClient.rawRequest("GET", `/api/v1/tasks/${subtask!.id}/sessions`);
    const subtaskData = (await subtaskRaw.json()) as { sessions: SessionInfo[] };
    const subtaskSession = subtaskData.sessions[0];

    expect(subtaskSession?.agent_profile_id).toBe(parentSession?.agent_profile_id);
    expect(subtaskSession?.executor_profile_id).toBe(parentSession?.executor_profile_id);
    // Sanity: both are non-empty (so the assertions above are not vacuously true)
    expect(subtaskSession?.agent_profile_id).toBeTruthy();
    expect(subtaskSession?.executor_profile_id).toBeTruthy();
  });
});
