import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("Session tab close guard", () => {
  test("last session tab hides the close button", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(60_000);

    // Create a task with one session
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Single Session Close Guard",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });

    const { sessions } = await apiClient.listTaskSessions(task.id);
    const firstSessionId = sessions[0]?.id;
    expect(firstSessionId).toBeTruthy();
    if (!firstSessionId) return;

    await expect(session.sessionTabBySessionId(firstSessionId)).toBeVisible({ timeout: 10_000 });
    await expect(session.sessionTabCloseButton(firstSessionId)).toBeHidden({ timeout: 5_000 });
  });

  test("closing one of two session panels hides the remaining panel's close button", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Hidden Sibling Close Guard",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });

    const { sessions: initialSessions } = await apiClient.listTaskSessions(task.id);
    const firstSessionId = initialSessions[0]?.id;
    expect(firstSessionId).toBeTruthy();

    await session.openNewSessionDialog();
    await session.newSessionPromptInput().fill("/e2e:simple-message");
    await session.newSessionStartButton().click();

    await expect
      .poll(async () => (await apiClient.listTaskSessions(task.id)).sessions.length, {
        timeout: 30_000,
        message: "Waiting for second session",
      })
      .toBe(2);

    const { sessions } = await apiClient.listTaskSessions(task.id);
    const secondSessionId = sessions.find((candidate) => candidate.id !== firstSessionId)?.id;
    expect(secondSessionId).toBeTruthy();
    if (!firstSessionId || !secondSessionId) return;

    await expect(session.sessionTabCloseButton(firstSessionId)).toBeVisible({ timeout: 10_000 });
    await session.sessionTabBySessionId(firstSessionId).click({ button: "right" });
    await session.contextMenuItem("Hide").click();

    await expect(session.sessionTabBySessionId(firstSessionId)).not.toBeVisible({ timeout: 5_000 });
    await expect(session.sessionTabBySessionId(secondSessionId)).toBeVisible();
    await expect(session.sessionTabCloseButton(secondSessionId)).not.toBeVisible();
    await expect
      .poll(async () => (await apiClient.listTaskSessions(task.id)).sessions.length, {
        timeout: 10_000,
        message:
          "Session count must stay at 2 after tab close (panel close should not delete session)",
      })
      .toBe(2);
  });
});
