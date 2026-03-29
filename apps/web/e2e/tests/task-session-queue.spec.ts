import { type Page } from "@playwright/test";
import { test, expect } from "../fixtures/test-base";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "../helpers/api-client";
import { SessionPage } from "../pages/session-page";

/**
 * Task session detail page — queued message tests.
 *
 * Verifies that users can queue messages via button click while the agent is
 * busy, including with plan mode enabled. Uses /slow to keep the agent running.
 */

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

/**
 * Type text into the TipTap editor while the agent is busy.
 * fill() silently fails on TipTap when the busy placeholder is shown,
 * so we retry clicking and typing until text appears in the editor.
 */
async function typeWhileBusy(page: Page, editor: import("@playwright/test").Locator, text: string) {
  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await editor.scrollIntoViewIfNeeded();
  for (let attempt = 0; attempt < 3; attempt++) {
    const box = await editor.boundingBox();
    if (!box) throw new Error("Editor bounding box not found");
    await page.mouse.click(box.x + 20, box.y + box.height / 2);
    await page.waitForTimeout(200);
    await page.keyboard.type(text);
    await page.waitForTimeout(100);
    const content = await editor.textContent();
    if (content?.includes(text)) return;
    await page.keyboard.press(`${modifier}+a`);
    await page.keyboard.press("Backspace");
    await page.waitForTimeout(200);
  }
  throw new Error(`Failed to type "${text}" into editor after 3 attempts`);
}

test.describe("Task session queue", () => {
  test.describe.configure({ retries: 1 });

  test("queue message via submit button on task session page", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const session = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Queue button test",
    );

    // Send a slow command to keep the agent busy.
    await session.sendMessage("/slow 5s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
    await testPage.waitForTimeout(500);

    // Type a message while agent is busy.
    const editor = testPage.locator(".tiptap.ProseMirror").first();
    await typeWhileBusy(testPage, editor, "queued via button");

    // Both submit and cancel-agent buttons should be visible.
    const submitBtn = testPage.getByTestId("submit-message-button");
    const cancelAgentBtn = testPage.getByTestId("cancel-agent-button");
    await expect(submitBtn).toBeVisible({ timeout: 5_000 });
    await expect(cancelAgentBtn).toBeVisible();

    // Click the submit button to queue the message.
    await submitBtn.click();

    // Verify the queued message indicator appears.
    await expect(testPage.getByTitle("Cancel queued message")).toBeVisible({ timeout: 10_000 });

    // Wait for the queued message to auto-execute and agent to become idle.
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
  });

  test("queue message with plan mode enabled via submit button", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const session = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Queue plan mode test",
    );

    // Enable plan mode.
    await session.togglePlanMode();

    // Send a slow command to keep the agent busy.
    await session.sendMessage("/slow 5s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
    await testPage.waitForTimeout(500);

    // In plan mode with no typed text, only the cancel button should be visible.
    // The auto-added plan context should NOT cause the send button to appear.
    const submitBtn = testPage.getByTestId("submit-message-button");
    await expect(submitBtn).not.toBeVisible();
    await expect(testPage.getByTestId("cancel-agent-button")).toBeVisible();

    // Type a message while agent is busy — send button should appear.
    const editor = testPage.locator(".tiptap.ProseMirror").first();
    await typeWhileBusy(testPage, editor, "plan queue test");
    await expect(submitBtn).toBeVisible({ timeout: 5_000 });

    // Click the submit button to queue the message.
    await submitBtn.click();

    // Verify the queued message indicator shows clean text (no system tags).
    const queueIndicator = testPage.getByTitle("Cancel queued message").locator("..");
    await expect(queueIndicator).toBeVisible({ timeout: 10_000 });
    await expect(queueIndicator).not.toContainText("kandev-system");

    // Wait for agent to finish processing.
    await expect(session.planModeInput()).toBeVisible({ timeout: 30_000 });
  });
});
