import { test, expect } from "../../fixtures/test-base";
import { seedIdleSession } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

test.describe("genuine ACP process loss (crash)", () => {
  test("shows the manual recovery banner, not the yellow retrying card", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedIdleSession(
      testPage,
      apiClient,
      seedData,
      "Transport Lost Recovery Test",
    );

    await session.sendMessage("/crash");

    // A real subprocess exit is local runtime loss, not a remote error response,
    // so the session falls to the red Resume / Start-fresh recovery banner
    // rather than the retained-turn inline error or a retry card.
    await expect(session.recoveryResumeButton()).toBeVisible({ timeout: 30_000 });
    await expect(session.recoveryFreshButton()).toBeVisible();
    await expect(session.transientRetryCard()).toBeHidden();
    await expect(session.recoveryCancelRetryButton()).toBeHidden();
  });

  test("a peer-disconnected error on the very first turn falls to manual recovery", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Start the task with a real crash as the INITIAL turn. Initial launches
    // go through LaunchPreparedSession, not PromptTask, so manual recovery must
    // be reachable from that path too, not only from a prompt sent to an
    // already-idle session.
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Initial Runtime Loss Test",
      seedData.agentProfileId,
      {
        description: "/crash",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.recoveryResumeButton()).toBeVisible({ timeout: 30_000 });
    await expect(session.recoveryFreshButton()).toBeVisible();
    await expect(session.transientRetryCard()).toBeHidden();
  });
});
