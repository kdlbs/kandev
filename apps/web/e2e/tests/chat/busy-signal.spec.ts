import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForActiveSessionForegroundActivity } from "../../helpers/session-store";
import { typeWhileBusy } from "../../helpers/type-while-busy";
import { SessionPage } from "../../pages/session-page";

async function seedTaskAndWaitForIdle(
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
  await session.waitForChatIdle({ timeout: 30_000 });
  return session;
}

test.describe("Coarse RUNNING busy signal", () => {
  test.describe.configure({ retries: 1 });

  test("held-open background turn remains busy across input and reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const session = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Coarse busy signal",
    );

    await session.sendMessage("/background 30s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByText("Kicking off background work")).toBeVisible({
      timeout: 15_000,
    });
    // The foreground-idle frame follows this text in the mock's ordered ACP
    // stream. Allow the subsequent WS publication to settle before asserting
    // the stable composer contract.
    await testPage.waitForTimeout(500);

    // The private tracker may identify background work, but the public
    // contract remains coarse for the entire RUNNING turn.
    await waitForActiveSessionForegroundActivity(testPage, "generating");
    await expect(session.idleInput()).not.toBeVisible();
    await expect(testPage.locator('[data-placeholder^="Queue"]')).toBeVisible();

    const editor = testPage.locator(".tiptap.ProseMirror").first();
    await typeWhileBusy(testPage, editor, "queue this follow-up");
    await testPage.getByTestId("submit-message-button").click();
    await expect(testPage.getByTestId("queue-chip")).toBeVisible({ timeout: 10_000 });

    await testPage.reload();
    await session.waitForLoad();
    await expect(session.agentStatus()).toBeVisible();
    await waitForActiveSessionForegroundActivity(testPage, "generating");
    await expect(session.idleInput()).not.toBeVisible();
    await expect(testPage.locator('[data-placeholder^="Queue"]')).toBeVisible();
  });

  test("foreground generation continues to queue input", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    const session = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Coarse busy foreground",
    );

    await session.sendMessage("/slow 10s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
    await waitForActiveSessionForegroundActivity(testPage, "generating");

    const editor = testPage.locator(".tiptap.ProseMirror").first();
    await typeWhileBusy(testPage, editor, "queue foreground follow-up");
    await testPage.getByTestId("submit-message-button").click();
    await expect(testPage.getByTestId("queue-chip")).toBeVisible({ timeout: 10_000 });
  });
});
