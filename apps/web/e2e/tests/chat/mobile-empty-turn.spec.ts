import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

const HINT = /without the leading slash/i;

/**
 * Mobile parity for the empty-turn notice: the notice is a regular chat message,
 * so it must render in the mobile session chat too. The triggering turn is the
 * auto-started first turn (a multi-second `/e2e:empty-turn`) rather than a
 * `sendMessage` follow-up — the live turn.completed reliably reaches the mobile
 * client while the turn is still running.
 */
async function seedAndOpenChat(
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
      description: "/e2e:empty-turn",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  return session;
}

test.describe("Mobile empty-turn notice", () => {
  test.describe.configure({ retries: 1, timeout: 120_000 });

  test("shows the slash-command hint in the mobile chat", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedAndOpenChat(testPage, apiClient, seedData, "Mobile Empty Turn Hint");

    await expect(session.chat).toContainText(HINT, { timeout: 30_000 });
  });
});
