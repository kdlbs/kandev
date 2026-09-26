import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { typeWhileBusy, waitForComposerQueueMode } from "../../helpers/type-while-busy";
import { SessionPage } from "../../pages/session-page";

/**
 * Creates a task whose agent stays busy for 30s (via `/slow 30s`) and queues
 * one message, returning the open task route. The long busy window keeps the
 * queued entry alive across the navigation below.
 */
async function seedPinnedQueueTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<{ taskId: string }> {
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

  await session.sendMessage("/slow 30s");
  await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
  await waitForComposerQueueMode(testPage);

  const editor = testPage.locator(".tiptap.ProseMirror").first();
  await typeWhileBusy(testPage, editor, "queued while pinned");
  await testPage.getByTestId("submit-message-button").click();

  await expect(testPage.getByTestId("queue-chip")).toBeVisible({ timeout: 10_000 });
  return { taskId: task.id };
}

test.describe("Message queue pin", () => {
  test.describe.configure({ retries: 1 });

  test("pinned queue stays expanded across navigation", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const { taskId } = await seedPinnedQueueTask(
      testPage,
      apiClient,
      seedData,
      "Pinned queue navigation",
    );

    // Expand the panel and pin it.
    await testPage.getByTestId("queue-chip").click();
    await expect(testPage.getByTestId("queued-ghost-list")).toBeVisible({ timeout: 5_000 });
    await testPage.getByTestId("queue-pin").click();
    await expect(testPage.getByTestId("queue-pin")).toHaveAttribute("aria-pressed", "true");

    // Navigate away and back — the panel must already be expanded, with no
    // chip click required.
    await testPage.goto("/");
    await testPage.waitForLoadState("networkidle");
    await testPage.goto(`/t/${taskId}`);

    await expect(testPage.getByTestId("queued-ghost-list")).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("queue-chip")).not.toBeVisible();
  });

  test("unpinned queue stays collapsed across navigation", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const { taskId } = await seedPinnedQueueTask(
      testPage,
      apiClient,
      seedData,
      "Unpinned queue navigation",
    );

    // Open the panel (without pinning), then navigate away and back.
    await testPage.getByTestId("queue-chip").click();
    await expect(testPage.getByTestId("queued-ghost-list")).toBeVisible({ timeout: 5_000 });
    await testPage.goto("/");
    await testPage.waitForLoadState("networkidle");
    await testPage.goto(`/t/${taskId}`);

    // Without a pin the queue starts collapsed: chip visible, panel hidden.
    await expect(testPage.getByTestId("queue-chip")).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("queued-ghost-list")).not.toBeVisible();
  });
});
