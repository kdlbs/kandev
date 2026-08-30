import { test, expect } from "../../fixtures/test-base";
import { seedIdleSession } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

test.describe("transient provider error (ACP peer disconnected) retry", () => {
  test("shows the yellow retrying card labelled 'Agent connection lost', not the red error banner", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedIdleSession(
      testPage,
      apiClient,
      seedData,
      "Transport Lost Retry Test",
    );

    // /transport-lost:9 keeps failing so the retry loop stays visible until cancel.
    await session.sendMessage("/transport-lost:9");

    // The calm yellow "retrying" card + Cancel button must appear, labelled
    // for the ACP transport-death signature (not the generic provider copy)...
    await expect(session.transientRetryCard()).toBeVisible({ timeout: 30_000 });
    await expect(session.transientRetryCard().getByText("Agent connection lost")).toBeVisible();
    await expect(session.recoveryCancelRetryButton()).toBeVisible();

    // ...and the red recovery banner must NOT be shown yet (retries in flight).
    await expect(session.recoveryResumeButton()).toBeHidden();
  });

  test("Cancel stops the retry loop and surfaces the recovery banner", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedIdleSession(
      testPage,
      apiClient,
      seedData,
      "Transport Lost Cancel Test",
    );

    await session.sendMessage("/transport-lost:9");
    await expect(session.recoveryCancelRetryButton()).toBeVisible({ timeout: 30_000 });

    await session.recoveryCancelRetryButton().click();

    // Cancelling falls through to the red Resume / Start-fresh recovery banner.
    await expect(session.recoveryResumeButton()).toBeVisible({ timeout: 30_000 });
    await expect(session.recoveryFreshButton()).toBeVisible();
    await expect(session.transientRetryCard()).toBeHidden();
  });

  test("a peer-disconnected error on the very first turn retries (launch prompt is cached)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Start the task with the failing prompt as the INITIAL turn. Initial
    // launches go through LaunchPreparedSession, not PromptTask, so the prompt
    // must be cached there too or the retry would park behind a stuck card.
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Initial Transport Lost Test",
      seedData.agentProfileId,
      {
        description: "/transport-lost:9",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.chat.getByText(/attempt 1 of 5/i)).toBeVisible({ timeout: 30_000 });
    await expect(session.chat.getByText(/attempt 2 of 5/i)).toBeVisible({ timeout: 30_000 });
  });
});
