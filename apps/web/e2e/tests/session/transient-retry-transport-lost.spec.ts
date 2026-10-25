import { test, expect } from "../../fixtures/test-base";
import { seedIdleSession } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

test.describe("ACP peer-disconnected prompt error (transport lost)", () => {
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

    await session.sendMessage("/transport-lost");

    // The signature that classifies this as transient lives only in the ACP
    // error's Data, which the projection never reads, so the failure is
    // presented as terminal on the first occurrence: the red Resume /
    // Start-fresh recovery banner, not the yellow retrying card.
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
    // Start the task with the failing prompt as the INITIAL turn. Initial
    // launches go through LaunchPreparedSession, not PromptTask, so manual
    // recovery must be reachable from that path too, not only from a prompt
    // sent to an already-idle session.
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Initial Transport Lost Test",
      seedData.agentProfileId,
      {
        description: "/transport-lost",
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
