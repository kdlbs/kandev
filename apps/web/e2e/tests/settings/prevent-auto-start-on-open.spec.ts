import { test, expect, type Page } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

// ---------------------------------------------------------------------------
// Prevent agent auto-start on open (prevent_auto_start_agent_on_open).
//
// The final-step cases add a custom terminal step (on_enter auto_start_agent)
// to the SEEDED (active) workflow: the task page hydrates state.kanban.steps
// for the active workflow via SSR, so the frontend gate resolves the step
// list without board-side multi-workflow snapshots. Each test deletes its
// task and step in teardown so the seeded workflow stays pristine for the
// next test in the same worker.
// ---------------------------------------------------------------------------

async function openTaskSession(page: Page, title: string): Promise<SessionPage> {
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const { KanbanPage } = await import("../../pages/kanban-page");
  const kanban = new KanbanPage(page);
  await kanban.goto();

  const card = kanban.taskCardByTitle(title);
  await expect(card).toBeVisible({ timeout: 15_000 });
  await card.click();
  await expect(page).toHaveURL(/\/t\//, { timeout: 15_000 });

  const session = new SessionPage(page);
  await session.waitForLoad();
  return session;
}

test.describe("Prevent auto-start on open", () => {
  test("final-step task with no session opens with the Start agent button when the preference is on", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const finalStep = await apiClient.createWorkflowStep(
      seedData.workflowId,
      "Prevent AutoStart Done",
      7,
      { events: { on_enter: [{ type: "auto_start_agent" }] } },
    );
    let taskId: string | undefined;
    try {
      await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: true });
      const task = await apiClient.createTask(
        seedData.workspaceId,
        "Prevent AutoStart Final Step Task",
        {
          description: "Prevent auto-start on open final step task",
          workflow_id: seedData.workflowId,
          workflow_step_id: finalStep.id,
          agent_profile_id: seedData.agentProfileId,
          repository_ids: [seedData.repositoryId],
        },
      );
      taskId = task.id;

      await testPage.goto(`/t/${task.id}`);

      // The preference keeps the agent stopped and renders the Start agent
      // button (session created in the prepared/CREATED state).
      const startButton = testPage.getByTestId("task-description-start-button");
      await expect(startButton).toBeVisible({ timeout: 30_000 });
      await expect(testPage.getByText("Started agent", { exact: false })).toHaveCount(0);

      // Clicking the button starts the agent, which then answers prompts.
      await startButton.click();
      const session = new SessionPage(testPage);
      await expect(testPage.getByText("Started agent", { exact: false })).toBeVisible({
        timeout: 30_000,
      });
      await session.sendMessage("/e2e:simple-message");
      await session.expectChatResponseVisible("simple mock response", 0, { timeout: 30_000 });
    } finally {
      await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: false });
      if (taskId) await apiClient.deleteTask(taskId);
      await apiClient.deleteWorkflowStep(finalStep.id);
    }
  });

  test("final-step task auto-starts on open when the preference is off (control)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const finalStep = await apiClient.createWorkflowStep(
      seedData.workflowId,
      "Prevent AutoStart Ctrl Done",
      8,
      { events: { on_enter: [{ type: "auto_start_agent" }] } },
    );
    let taskId: string | undefined;
    try {
      const task = await apiClient.createTask(
        seedData.workspaceId,
        "Prevent AutoStart Control Task",
        {
          description: "Prevent auto-start on open final step task",
          workflow_id: seedData.workflowId,
          workflow_step_id: finalStep.id,
          agent_profile_id: seedData.agentProfileId,
          repository_ids: [seedData.repositoryId],
        },
      );
      taskId = task.id;

      await testPage.goto(`/t/${task.id}`);

      // Default behavior: the agent auto-starts and no Start button is shown.
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await expect(testPage.getByText("Started agent", { exact: false })).toBeVisible({
        timeout: 30_000,
      });
      await expect(testPage.getByTestId("task-description-start-button")).toHaveCount(0);
    } finally {
      if (taskId) await apiClient.deleteTask(taskId);
      await apiClient.deleteWorkflowStep(finalStep.id);
    }
  });

  test("post-restart session does not auto-resume when the preference is on", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(180_000);
    await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: true });
    try {
      await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Prevent AutoStart Resume Task",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
        },
      );

      // Let the agent finish its first turn so the session ends idle with a
      // resume token (the post-restart needs_resume shape).
      const session = await openTaskSession(testPage, "Prevent AutoStart Resume Task");
      await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
        timeout: 30_000,
      });
      await session.waitForChatIdle({ timeout: 15_000 });

      // Restart the backend and reload — the resume-skipped gate must keep
      // the agent stopped. The composer hint replaces the footer Start agent
      // button: it says a message will auto-start the agent, and it is
      // always visible (the composer sits outside the transcript scroll
      // container, so the clipped-button failure cannot occur).
      await backend.restart();
      await testPage.reload();
      await session.waitForLoad();

      await expect(testPage.getByTestId("composer-agent-start-hint")).toBeVisible({
        timeout: 60_000,
      });
      await expect(testPage.getByTestId("session-resume-start-button")).toHaveCount(0);
      await expect(testPage.getByText("Resumed agent", { exact: false })).toHaveCount(0);

      // Sending a message is the explicit start: the agent resumes and
      // keeps working. Index 1: the first "simple mock response" from the
      // pre-restart turn is still in the transcript, so the post-send reply
      // must be the SECOND match — asserting index 0 would pass even if the
      // resumed launch dropped the prompt.
      await session.sendMessage("/e2e:simple-message");
      await expect(testPage.getByText("Resumed agent", { exact: false })).toBeVisible({
        timeout: 30_000,
      });
      await session.expectChatResponseVisible("simple mock response", 1, { timeout: 30_000 });
    } finally {
      await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: false });
    }
  });
});
