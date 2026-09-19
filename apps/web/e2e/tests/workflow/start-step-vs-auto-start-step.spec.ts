import { test, expect } from "../../fixtures/test-base";
import { waitForSessionDone } from "../../helpers/session";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

/**
 * `is_start_step` and `auto_start_agent` are independent settings: the first
 * says where new tasks are created, the second says which steps run agents.
 * Task creation used to resolve both create actions through the start step,
 * which silently made them synonymous — marking Backlog as the start step
 * parked the task there and started an agent in it anyway.
 *
 * The seeded "simple" workflow ships with In Progress as both, so the two
 * rules agree and nothing can be observed. Moving the start step onto Backlog
 * (which automates nothing) separates them, which is the only configuration
 * where the routing is visible at all.
 */
test.describe("start step vs auto-start step", () => {
  test("routes immediate agent starts to the automated step in either agent mode", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const backlog = seedData.steps[0];
    const inProgress = seedData.steps[1];
    expect(backlog.name).toBe("Backlog");
    expect(inProgress.name).toBe("In Progress");

    // Backlog becomes the start step, a parking column that automates nothing;
    // In Progress is the first step that runs agents. seedData is worker-scoped,
    // so both settings are stated explicitly instead of inherited.
    await apiClient.updateWorkflowStep(inProgress.id, {
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });
    await apiClient.updateWorkflowStep(backlog.id, { is_start_step: true });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const dialog = testPage.getByTestId("create-task-dialog");

    // --- Create without starting agent -> the start step ---
    await kanban.createTaskButton.click();
    await expect(dialog).toBeVisible();
    await testPage.getByTestId("task-title-input").fill("Parked for later");
    await testPage.getByTestId("task-description-input").fill("/e2e:simple-message");
    const chevron = testPage.getByTestId("submit-start-agent-chevron");
    await expect(chevron).toBeEnabled({ timeout: 30_000 });
    await chevron.click();
    await testPage.getByTestId("submit-create-without-agent").click();
    await expect(dialog).not.toBeVisible({ timeout: 15_000 });

    // The no-agent create opens the prepared task; come back to the board.
    await kanban.goto();
    await expect(kanban.taskCardInColumn("Parked for later", backlog.id)).toBeVisible({
      timeout: 15_000,
    });

    // --- Start task -> the first step carrying auto_start_agent ---
    await kanban.createTaskButton.click();
    await expect(dialog).toBeVisible();
    await testPage.getByTestId("task-title-input").fill("Runs right now");
    await testPage.getByTestId("task-description-input").fill("/e2e:simple-message");
    const start = testPage.getByTestId("submit-start-agent");
    await expect(start).toBeEnabled({ timeout: 30_000 });
    await start.click();
    await expect(dialog).not.toBeVisible({ timeout: 15_000 });

    await kanban.goto();
    await expect(kanban.taskCardInColumn("Runs right now", inProgress.id)).toBeVisible({
      timeout: 30_000,
    });
    // The parked card is untouched by the second create.
    await expect(kanban.taskCardInColumn("Parked for later", backlog.id)).toBeVisible();

    // --- Start task in plan mode -> the same auto-start destination ---
    await kanban.createTaskButton.click();
    await expect(dialog).toBeVisible();
    await testPage.getByTestId("task-title-input").fill("Plans right now");
    await testPage.getByTestId("task-description-input").fill("/e2e:simple-message");
    await expect(start).toBeEnabled({ timeout: 30_000 });
    await testPage.getByTestId("submit-start-agent-chevron").click();
    await testPage.getByTestId("submit-plan-mode").click();
    await expect(dialog).not.toBeVisible({ timeout: 15_000 });

    await kanban.goto();
    await expect(kanban.taskCardInColumn("Plans right now", inProgress.id)).toBeVisible({
      timeout: 30_000,
    });
    await expect(kanban.taskCardInColumn("Plans right now", backlog.id)).toHaveCount(0);
  });

  // The dialog used to send its own `workflow_step_id`, taken from whatever the
  // caller passed as `defaultStepId` — uniformly "first step by position", a
  // leftover from before `is_start_step` existed. On the stock template that
  // pinned a no-agent create to Backlog while the workflow's start step was
  // In Progress. The destination belongs to the backend.
  test("a no-agent create honors a start step that is not the first column", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const backlog = seedData.steps[0];
    const inProgress = seedData.steps[1];
    // Stock "simple" shape: In Progress is the start step, Backlog is column 0.
    // Drop the automation so this case isolates placement from launching.
    await apiClient.updateWorkflowStep(backlog.id, { is_start_step: false });
    await apiClient.updateWorkflowStep(inProgress.id, { is_start_step: true, events: {} });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const dialog = testPage.getByTestId("create-task-dialog");
    await kanban.createTaskButton.click();
    await expect(dialog).toBeVisible();
    await testPage.getByTestId("task-title-input").fill("Lands on the start step");
    await testPage.getByTestId("task-description-input").fill("/e2e:simple-message");
    const chevron = testPage.getByTestId("submit-start-agent-chevron");
    await expect(chevron).toBeEnabled({ timeout: 30_000 });
    await chevron.click();
    await testPage.getByTestId("submit-create-without-agent").click();
    await expect(dialog).not.toBeVisible({ timeout: 15_000 });

    await kanban.goto();
    await expect(kanban.taskCardInColumn("Lands on the start step", inProgress.id)).toBeVisible({
      timeout: 15_000,
    });
    await expect(kanban.taskCardInColumn("Lands on the start step", backlog.id)).toHaveCount(0);
  });

  test("does not repeat the plan-mode task description when entering an empty auto-start step", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Plan Prompt Dedup Workflow",
    );
    const planStep = await apiClient.createWorkflowStep(workflow.id, "Plan", 0, {
      is_start_step: true,
    });
    const autoStartStep = await apiClient.createWorkflowStep(workflow.id, "Auto Start", 1);
    // The first step is also the first auto-start destination. This keeps the
    // task on Plan under immediate-start routing, then exercises suppression
    // when the existing session enters a second empty auto-start step.
    await apiClient.updateWorkflowStep(planStep.id, {
      prompt: "",
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });
    await apiClient.updateWorkflowStep(autoStartStep.id, {
      prompt: "",
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      task_create_last_used: {
        repository_id: seedData.repositoryId,
        branch: "main",
        agent_profile_id: seedData.agentProfileId,
        workflow_ids_by_workspace: { [seedData.workspaceId]: workflow.id },
      },
      enable_preview_on_click: false,
    });

    const description = "/e2e:simple-message";
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    await testPage.getByTestId("task-title-input").fill("Plan prompt deduplication");
    await testPage.getByTestId("task-description-input").fill(description);

    const startButton = testPage.getByTestId("submit-start-agent");
    await expect(startButton).toBeEnabled({ timeout: 30_000 });
    await testPage.getByTestId("submit-start-agent-chevron").click();
    await expect(testPage.getByTestId("submit-plan-mode")).toBeVisible({ timeout: 5_000 });
    await testPage.getByTestId("submit-plan-mode").click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });
    await expect(testPage).toHaveURL(/\/t\/.*layout=plan/, { timeout: 15_000 });

    const taskId = testPage.url().match(/\/t\/([^/?#]+)/)?.[1];
    expect(taskId).toBeTruthy();
    if (!taskId) throw new Error("Plan-mode task ID was missing from the session URL");

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.planPanel).toBeVisible({ timeout: 10_000 });

    let sessionId: string | undefined;
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(taskId);
          sessionId = sessions[0]?.id;
          return sessionId ?? null;
        },
        { timeout: 30_000, message: "Waiting for the plan-mode session to be created" },
      )
      .not.toBeNull();
    if (!sessionId) throw new Error("Plan-mode session ID was not created");

    await expect(
      session.activeChat().getByText("simple mock response", { exact: false }),
    ).toBeVisible({
      timeout: 30_000,
    });
    await waitForSessionDone(
      apiClient,
      taskId,
      sessionId,
      "Waiting for the first plan-mode turn to finish",
    );
    await session.waitForChatIdle({ timeout: 30_000 });

    await expect
      .poll(
        async () => {
          const { tasks } = await apiClient.listTasks(seedData.workspaceId);
          return tasks.find((task) => task.id === taskId)?.workflow_step_id ?? null;
        },
        { timeout: 15_000, message: "Waiting for the plan-mode task to remain on its first step" },
      )
      .toBe(planStep.id);

    const beforeMoveMessages = await apiClient.listSessionMessages(sessionId);
    const beforeMoveTurns = await apiClient.listSessionTurns(sessionId);
    const beforeMoveSession = (await apiClient.listTaskSessions(taskId)).sessions.find(
      (candidate) => candidate.id === sessionId,
    );
    expect(beforeMoveSession).toBeTruthy();
    const beforeMoveUpdatedAt = beforeMoveSession?.updated_at;

    await apiClient.moveTask(taskId, workflow.id, autoStartStep.id);
    await expect
      .poll(
        async () => {
          const { tasks } = await apiClient.listTasks(seedData.workspaceId);
          return tasks.find((task) => task.id === taskId)?.workflow_step_id ?? null;
        },
        { timeout: 15_000, message: "Waiting for the task to enter the empty auto-start step" },
      )
      .toBe(autoStartStep.id);

    // The move response only confirms task placement. The session review reset
    // is the first durable signal from the asynchronous on_enter path, so wait
    // for that post-move write before inspecting the transcript.
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(taskId);
          const current = sessions.find((candidate) => candidate.id === sessionId);
          return current?.updated_at && current.updated_at !== beforeMoveUpdatedAt
            ? current.updated_at
            : null;
        },
        {
          timeout: 30_000,
          message: "Waiting for asynchronous workflow on_enter processing to write session state",
        },
      )
      .not.toBeNull();

    // Suppression is intentionally silent, so there is no new idle transition
    // for waitForChatIdle to observe. Require a stable server-side snapshot
    // after the causal write, which also catches a late duplicate turn.
    let previousSnapshot = "";
    let stableReads = 0;
    await expect
      .poll(
        async () => {
          const [{ messages }, { turns }, { sessions }] = await Promise.all([
            apiClient.listSessionMessages(sessionId),
            apiClient.listSessionTurns(sessionId),
            apiClient.listTaskSessions(taskId),
          ]);
          const current = sessions.find((candidate) => candidate.id === sessionId);
          const snapshot = JSON.stringify({
            state: current?.state,
            messages: messages.map((message) => ({
              id: message.id,
              author_type: message.author_type,
              content: message.content,
            })),
            turns: turns.map((turn) => ({ id: turn.id, completed_at: turn.completed_at })),
          });
          if (snapshot === previousSnapshot) {
            stableReads += 1;
          } else {
            previousSnapshot = snapshot;
            stableReads = 0;
          }
          return stableReads;
        },
        {
          timeout: 30_000,
          intervals: [250, 500, 750, 1_000],
          message: "Waiting for workflow on_enter prompt admission to settle",
        },
      )
      .toBeGreaterThanOrEqual(3);

    const afterMoveMessages = await apiClient.listSessionMessages(sessionId);
    const afterMoveTurns = await apiClient.listSessionTurns(sessionId);
    const userMessages = afterMoveMessages.messages.filter(
      (message) => message.author_type === "user",
    );
    await session.showSessionContext();

    const userDescriptionMessages = userMessages.filter((message) =>
      message.content.includes(description),
    );
    const emptyUserMessages = userMessages.filter((message) => message.content.trim() === "");
    expect(userDescriptionMessages).toHaveLength(1);
    expect(emptyUserMessages).toHaveLength(0);
    expect(afterMoveTurns.turns).toHaveLength(beforeMoveTurns.turns.length);
    expect(
      beforeMoveMessages.messages.filter((message) => message.author_type === "user"),
    ).toHaveLength(1);
    await expect(session.activeChat().getByText(description, { exact: true })).toHaveCount(1);
  });
});
