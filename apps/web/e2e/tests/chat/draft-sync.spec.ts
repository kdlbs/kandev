import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

/**
 * E2E tests for cross-device draft persistence.
 * Verifies that chat input drafts are saved to the server and restored
 * when the same session is opened in a new browser context (simulating a different device).
 */

const DRAFT_TEXT = "cross-device draft test message";
const DEBOUNCE_WAIT_MS = 3_000;

async function seedTaskAndWaitForIdle(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
) {
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
  await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

  return { session, taskId: task.id, sessionId: task.session_id! };
}

async function typeInEditor(page: Page, text: string) {
  const editor = page.locator(".tiptap.ProseMirror").first();
  await editor.click();
  await editor.fill(text);
}

async function getEditorText(page: Page): Promise<string> {
  const editor = page.locator(".tiptap.ProseMirror").first();
  return (await editor.textContent()) ?? "";
}

test.describe("Draft sync", () => {
  test.describe.configure({ retries: 1 });

  test("draft auto-saves to server and restores in new browser context", async ({
    testPage,
    apiClient,
    seedData,
    browser,
    backend,
  }) => {
    test.setTimeout(90_000);

    // Seed a task with a completed session so the input is idle.
    const { taskId } = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Draft Sync Test",
    );

    // Type a draft message.
    await typeInEditor(testPage, DRAFT_TEXT);

    // Wait for debounce + save to complete.
    await testPage.waitForTimeout(DEBOUNCE_WAIT_MS);

    // Open the same session in a new browser context (simulates a different device — fresh sessionStorage).
    const context2 = await browser.newContext({ baseURL: backend.frontendUrl });
    const page2 = await context2.newPage();

    // Set up page2 with the same init scripts as testPage.
    await page2.addInitScript(
      ({ backendPort }: { backendPort: string }) => {
        localStorage.setItem("kandev.onboarding.completed", "true");
        window.__KANDEV_API_PORT = backendPort;
      },
      { backendPort: String(backend.port) },
    );

    try {
      await page2.goto(`/t/${taskId}`);
      const session2 = new SessionPage(page2);
      await session2.waitForLoad();

      // The draft should be restored from the server.
      await expect(page2.locator(".tiptap.ProseMirror").first()).toHaveText(DRAFT_TEXT, {
        timeout: 15_000,
      });
    } finally {
      await context2.close();
    }
  });

  test("draft is cleared after sending a message", async ({
    testPage,
    apiClient,
    seedData,
    browser,
    backend,
  }) => {
    test.setTimeout(90_000);

    const { taskId } = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Draft Clear Test",
    );

    // Type a draft and wait for server save.
    await typeInEditor(testPage, "message to send and clear");
    await testPage.waitForTimeout(DEBOUNCE_WAIT_MS);

    // Submit the message.
    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await testPage.locator(".tiptap.ProseMirror").first().press(`${modifier}+Enter`);

    // Wait for the message to appear in chat (agent processes it).
    await expect(testPage.getByText("simple mock response for e2e testing")).toBeVisible({
      timeout: 30_000,
    });

    // Open the same session in a new context — draft should be empty.
    const context2 = await browser.newContext({ baseURL: backend.frontendUrl });
    const page2 = await context2.newPage();
    await page2.addInitScript(
      ({ backendPort }: { backendPort: string }) => {
        localStorage.setItem("kandev.onboarding.completed", "true");
        window.__KANDEV_API_PORT = backendPort;
      },
      { backendPort: String(backend.port) },
    );

    try {
      await page2.goto(`/t/${taskId}`);
      const session2 = new SessionPage(page2);
      await session2.waitForLoad();
      await expect(session2.idleInput()).toBeVisible({ timeout: 30_000 });

      // Verify the editor is empty (no stale draft).
      const text = await getEditorText(page2);
      expect(text.trim()).toBe("");
    } finally {
      await context2.close();
    }
  });
});
