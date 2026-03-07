import { type Page } from "@playwright/test";
import { test, expect } from "../fixtures/test-base";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "../helpers/api-client";
import { SessionPage } from "../pages/session-page";

/**
 * Seed a task + session via the API and navigate directly to the session page.
 * Waits for the mock agent to complete its turn (idle input visible).
 */
async function seedTaskWithSession(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  description = "/e2e:simple-message",
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await testPage.goto(`/s/${task.session_id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

  return session;
}

test.describe("Session recovery", () => {
  test.describe.configure({ retries: 1 });

  test("reset context shows divider and agent responds fresh", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedTaskWithSession(testPage, apiClient, seedData, "Reset Context Test");

    // Click reset context button — confirmation dialog should appear
    await session.resetContextButton().click();
    await expect(session.resetContextConfirm()).toBeVisible();

    // Confirm the reset
    await session.resetContextConfirm().click();

    // Divider should appear in chat
    await expect(session.contextResetDivider()).toBeVisible({ timeout: 30_000 });

    // Agent should restart and become idle again
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // Verify agent works after reset by sending a new message
    await session.sendMessage("/e2e:simple-message");
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
  });

  test("agent crash — start fresh session recovers", async ({ testPage, apiClient, seedData }) => {
    const session = await seedTaskWithSession(
      testPage,
      apiClient,
      seedData,
      "Crash Recovery Fresh Test",
    );

    // Send /crash to make the agent exit with code 1
    await session.sendMessage("/crash");

    // Recovery buttons should appear
    await expect(session.recoveryFreshButton()).toBeVisible({ timeout: 30_000 });
    await expect(session.recoveryResumeButton()).toBeVisible();

    // Click "Start fresh session"
    await session.recoveryFreshButton().click();

    // Agent should recover and become idle
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // Verify agent works after recovery
    await session.sendMessage("/e2e:simple-message");
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
  });

  test("agent crash — resume session recovers", async ({ testPage, apiClient, seedData }) => {
    const session = await seedTaskWithSession(
      testPage,
      apiClient,
      seedData,
      "Crash Recovery Resume Test",
    );

    // Send /crash to make the agent exit with code 1
    await session.sendMessage("/crash");

    // Recovery buttons should appear
    await expect(session.recoveryResumeButton()).toBeVisible({ timeout: 30_000 });

    // Click "Resume session"
    await session.recoveryResumeButton().click();

    // Agent should recover and become idle
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // Verify agent works after recovery
    await session.sendMessage("/e2e:simple-message");
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
  });
});
