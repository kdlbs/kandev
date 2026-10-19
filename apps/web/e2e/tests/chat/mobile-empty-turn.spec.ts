import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

const HINT = /without the leading slash/i;

/** Open a normal completed turn before triggering the live-only empty-turn notice. */
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
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.chat).toContainText("This is a simple mock response for e2e testing.", {
    timeout: 30_000,
  });
  await session.waitForChatIdle();
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

    await session.sendMessageViaButton("/e2e:empty-turn");
    await expect(session.chat).toContainText(HINT, { timeout: 30_000 });
  });
});
