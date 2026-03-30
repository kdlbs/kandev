import { type Locator, type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

/**
 * Type text into the TipTap editor while the agent is busy.
 * fill() silently fails on TipTap when the busy placeholder is shown,
 * so we retry clicking and typing until text appears in the editor.
 */
async function typeWhileBusy(page: Page, editor: Locator, text: string): Promise<void> {
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
    // Text wasn't entered; select all and clear for retry
    await page.keyboard.press(`${modifier}+a`);
    await page.keyboard.press("Backspace");
    await page.waitForTimeout(200);
  }
  throw new Error(`Failed to type "${text}" into editor after 3 attempts`);
}

// ---------------------------------------------------------------------------
// Quick Chat queue tests
// ---------------------------------------------------------------------------

async function openQuickChatWithAgent(page: Page): Promise<Locator> {
  await page.goto("/");
  await page.waitForLoadState("networkidle");

  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await page.keyboard.press(`${modifier}+Shift+q`);

  const dialog = page.getByRole("dialog", { name: "Quick Chat" });
  await expect(dialog).toBeVisible({ timeout: 10_000 });

  const agentPicker = dialog.getByText("Choose an agent to start chatting");
  if (!(await agentPicker.isVisible({ timeout: 1_000 }).catch(() => false))) {
    await dialog.getByLabel("Start new chat").click();
  }
  await expect(agentPicker).toBeVisible({ timeout: 5_000 });

  const agentCard = dialog
    .locator("button")
    .filter({ has: page.locator(".rounded-md.border") })
    .first();
  await expect(agentCard).toBeVisible({ timeout: 5_000 });
  await agentCard.click();

  await expect(dialog.locator(".tiptap.ProseMirror")).toBeVisible({ timeout: 15_000 });
  return dialog;
}

test.describe("Quick chat queue", () => {
  // Allow 1 retry: the test can be flaky when a previous test cycle's agent process hasn't
  // fully shut down, causing the new session to conflict with a stale execution.
  test.describe.configure({ retries: 1 });

  test("queued message indicator appears and message executes after agent turn", async ({
    testPage,
  }) => {
    test.setTimeout(60_000);

    const dialog = await openQuickChatWithAgent(testPage);

    // Send a slow command so the agent stays busy for 10 seconds.
    const editor = dialog.locator(".tiptap.ProseMirror");
    await editor.click();
    await editor.fill("/slow 10s");
    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await editor.press(`${modifier}+Enter`);

    // Wait for agent to become busy.
    await expect(testPage.getByRole("status", { name: /Agent is (starting|running)/ })).toBeVisible(
      {
        timeout: 15_000,
      },
    );
    await testPage.waitForTimeout(500);

    await typeWhileBusy(testPage, editor, "hello world");
    await testPage.keyboard.press(`${modifier}+Enter`);

    // Verify the queued message indicator with cancel button appears.
    const cancelBtn = dialog.getByTitle("Cancel queued message");
    await expect(cancelBtn).toBeVisible({ timeout: 10_000 });

    // Wait for the first (slow) response to complete.
    await expect(dialog.getByText("Slow response complete", { exact: false })).toBeVisible({
      timeout: 30_000,
    });

    // The queued message should auto-execute — wait for the agent turn to finish.
    await expect(
      dialog.locator('[data-placeholder="Continue working on the task..."]'),
    ).toBeVisible({
      timeout: 30_000,
    });
  });

  test("queue message via submit button click", async ({ testPage }) => {
    test.setTimeout(90_000);

    const dialog = await openQuickChatWithAgent(testPage);

    // Send a slow command so the agent stays busy for 10 seconds.
    const editor = dialog.locator(".tiptap.ProseMirror");
    await editor.click();
    await editor.fill("/slow 10s");
    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await editor.press(`${modifier}+Enter`);

    // Wait for agent to become busy.
    await expect(testPage.getByRole("status", { name: /Agent is (starting|running)/ })).toBeVisible(
      {
        timeout: 15_000,
      },
    );
    await testPage.waitForTimeout(500);

    // Before typing, only the cancel button should be visible (no send button).
    const submitBtn = dialog.getByTestId("submit-message-button");
    await expect(submitBtn).not.toBeVisible();
    await expect(dialog.getByTestId("cancel-agent-button")).toBeVisible();

    // Type a queued message — the submit button should appear.
    await typeWhileBusy(testPage, editor, "queued via button");
    await expect(submitBtn).toBeVisible({ timeout: 5_000 });

    // Click the submit button (not keyboard shortcut) to queue the message.
    await submitBtn.click();

    // Verify the queued message indicator with cancel button appears.
    const cancelBtn = dialog.getByTitle("Cancel queued message");
    await expect(cancelBtn).toBeVisible({ timeout: 10_000 });

    // Verify the cancel-agent button is also visible alongside submit.
    const cancelAgentBtn = dialog.getByTestId("cancel-agent-button");
    await expect(cancelAgentBtn).toBeVisible();

    // Wait for the first (slow) response to complete and queued message to auto-execute.
    await expect(
      dialog.locator('[data-placeholder="Continue working on the task..."]'),
    ).toBeVisible({
      timeout: 60_000,
    });
  });
});

// ---------------------------------------------------------------------------
// Task session queue tests
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
