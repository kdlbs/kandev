import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

const HINT = /without the leading slash/i;
const MOCK_OUTPUT = "This is a simple mock response for e2e testing.";

/**
 * Seed a task whose auto-started first turn runs `scenario`, then open its chat.
 * The empty-turn notice is live-only, so the triggering turn is the auto-started
 * one (a multi-second `/e2e:empty-turn`) — the client subscribes while it runs
 * and receives the live turn.completed that drives the notice. Returns the page
 * without waiting for idle, so the assertion can observe the notice as it fires.
 */
async function seedAndOpenChat(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  description: string,
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
  return session;
}

test.describe("Empty-turn notice", () => {
  test.describe.configure({ retries: 1 });

  test("shows a slash-command hint when a /command produces no output", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedAndOpenChat(
      testPage,
      apiClient,
      seedData,
      "Empty Turn Hint",
      "/e2e:empty-turn",
    );

    await expect(session.chat).toContainText(HINT, { timeout: 30_000 });
  });

  test("does not show a hint when the turn produces output", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedAndOpenChat(
      testPage,
      apiClient,
      seedData,
      "Empty Turn No Hint",
      "/e2e:simple-message",
    );

    await expect(session.chat).toContainText(MOCK_OUTPUT, { timeout: 30_000 });
    await session.waitForChatIdle();
    await expect(session.chat).not.toContainText(HINT);
  });
});
