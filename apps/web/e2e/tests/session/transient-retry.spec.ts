import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

/**
 * Seed a task + session and navigate to it, waiting for the first (normal)
 * turn to complete. Follow-up `/overloaded` prompts then exercise the
 * transient-retry flow from a clean idle state.
 */
async function seedIdleSession(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
  return session;
}

test.describe("transient provider error (529 Overloaded) retry", () => {
  test("shows the yellow retrying card, not the red error banner", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedIdleSession(testPage, apiClient, seedData, "Overloaded Retry Test");

    // /overloaded:9 keeps failing so the retry loop stays visible until cancel.
    await session.sendMessage("/overloaded:9");

    // The calm yellow "retrying" card + Cancel button must appear...
    await expect(session.transientRetryCard()).toBeVisible({ timeout: 30_000 });
    await expect(session.recoveryCancelRetryButton()).toBeVisible();

    // ...and the red recovery banner must NOT be shown yet (retries in flight).
    await expect(session.recoveryResumeButton()).toBeHidden();
  });

  test("Cancel stops the retry loop and surfaces the recovery banner", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedIdleSession(testPage, apiClient, seedData, "Overloaded Cancel Test");

    await session.sendMessage("/overloaded:9");
    await expect(session.recoveryCancelRetryButton()).toBeVisible({ timeout: 30_000 });

    await session.recoveryCancelRetryButton().click();

    // Cancelling falls through to the red Resume / Start-fresh recovery banner.
    await expect(session.recoveryResumeButton()).toBeVisible({ timeout: 30_000 });
    await expect(session.recoveryFreshButton()).toBeVisible();
  });

  test("retries are paced — the attempt counter advances across the backoff", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedIdleSession(testPage, apiClient, seedData, "Overloaded Backoff Test");

    // /overloaded:9 keeps failing, so each backoff retry re-drives the prompt
    // and the orchestrator advances the attempt counter. The yellow card must
    // progress from attempt 1/3 to attempt 2/3 — proving the retry loop is live
    // and paced rather than an instant, silent resume.
    await session.sendMessage("/overloaded:9");

    await expect(session.chat.getByText(/retrying in 5s \(attempt 1\/3\)/i)).toBeVisible({
      timeout: 30_000,
    });
    await expect(session.chat.getByText(/attempt 2\/3/i)).toBeVisible({ timeout: 30_000 });
  });

  test("a 529 on the very first turn retries (launch prompt is cached)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Start the task with the failing prompt as the INITIAL turn. Initial
    // launches go through LaunchPreparedSession, not PromptTask, so the prompt
    // must be cached there too or the retry would park behind a stuck card.
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Initial Overload Test",
      seedData.agentProfileId,
      {
        description: "/overloaded:9",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.chat.getByText(/retrying in 5s \(attempt 1\/3\)/i)).toBeVisible({
      timeout: 30_000,
    });
    await expect(session.chat.getByText(/attempt 2\/3/i)).toBeVisible({ timeout: 30_000 });
  });
});
