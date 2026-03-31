import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

async function seedTaskAndWaitForIdle(
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

  await testPage.goto(`/t/${task.id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

  return session;
}

// ---------------------------------------------------------------------------
// Plan mode follow-up message tests
// ---------------------------------------------------------------------------

test.describe("Plan mode follow-up messages", () => {
  test.describe.configure({ retries: 1 });

  test("follow-up message in plan mode shows plan mode badge", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const session = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Plan mode follow-up test",
    );

    // Enable plan mode after initial agent turn completes.
    await session.togglePlanMode();
    await expect(session.planModeInput()).toBeVisible({ timeout: 10_000 });

    // Send a follow-up message in plan mode (simulating plan revision request).
    await session.sendMessage("Please add error handling to step 3 of the plan");

    // The follow-up message should have the plan mode badge, confirming
    // plan_mode: true was sent by the frontend and stored in message metadata.
    const planBadges = session.chat.getByText("Plan mode", { exact: true });
    await expect(planBadges.last()).toBeVisible({ timeout: 15_000 });

    // Wait for agent to complete and return to plan mode idle state.
    await expect(session.planModeInput()).toBeVisible({ timeout: 30_000 });
  });

  test("follow-up message without plan mode does not show plan mode badge", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const session = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "No plan mode follow-up test",
    );

    // Send a follow-up message without plan mode.
    await session.sendMessage("implement the feature now");

    // Wait for agent to complete.
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // The follow-up message should NOT have a plan mode badge.
    // Only count badges — the initial message might not have one either.
    const planBadges = session.chat.getByText("Plan mode", { exact: true });
    await expect(planBadges).toHaveCount(0, { timeout: 5_000 });
  });
});
