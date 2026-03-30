import { type Locator, type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";

/**
 * Quick Chat E2E tests: basic flow, enhance prompt, queued messages, multi-tab.
 */

async function openQuickChatWithAgent(page: Page): Promise<Locator> {
  await page.goto("/");
  await page.waitForLoadState("networkidle");

  // Open Quick Chat via keyboard shortcut (Cmd+Shift+Q / Ctrl+Shift+Q).
  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await page.keyboard.press(`${modifier}+Shift+q`);

  // Wait for Quick Chat dialog.
  const dialog = page.getByRole("dialog", { name: "Quick Chat" });
  await expect(dialog).toBeVisible({ timeout: 10_000 });

  // If a stale session tab is showing, click "+" to start a fresh agent picker.
  const agentPicker = dialog.getByText("Choose an agent to start chatting");
  if (!(await agentPicker.isVisible({ timeout: 1_000 }).catch(() => false))) {
    await dialog.getByLabel("Start new chat").click();
  }
  await expect(agentPicker).toBeVisible({ timeout: 5_000 });

  // Click the first agent profile card.
  const agentCard = dialog
    .locator("button")
    .filter({ has: page.locator(".rounded-md.border") })
    .first();
  await expect(agentCard).toBeVisible({ timeout: 5_000 });
  await agentCard.click();

  // Wait for chat input to appear (session created and content rendered).
  await expect(dialog.locator(".tiptap.ProseMirror")).toBeVisible({ timeout: 15_000 });

  return dialog;
}

async function sendQuickChatMessage(dialog: Locator, page: Page, text: string) {
  const editor = dialog.locator(".tiptap.ProseMirror");
  await editor.click();
  await editor.fill(text);
  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await editor.press(`${modifier}+Enter`);
}

test.describe("Quick Chat", () => {
  test("opens quick chat, selects agent, sends message and receives response", async ({
    testPage,
  }) => {
    const dialog = await openQuickChatWithAgent(testPage);

    await sendQuickChatMessage(dialog, testPage, "/e2e:simple-message");

    // Mock agent scenario "simple-message" responds with this text.
    await expect(
      dialog.getByText("simple mock response for e2e testing", { exact: false }),
    ).toBeVisible({ timeout: 30_000 });
  });

  test("enhance prompt replaces input text with AI-enhanced version", async ({
    testPage,
    apiClient,
  }) => {
    // Configure utility agent so the enhance button is enabled.
    await apiClient.saveUserSettings({
      default_utility_agent_id: "mock",
      default_utility_model: "mock-fast",
    });

    // Intercept utility execute API to return mock enhanced text.
    await testPage.route("**/api/v1/utility/execute", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          success: true,
          response: "Enhanced: please fix the null pointer bug in the user service",
          model: "mock-fast",
          prompt_tokens: 50,
          response_tokens: 20,
          duration_ms: 100,
        }),
      });
    });

    const dialog = await openQuickChatWithAgent(testPage);

    // Type initial text.
    const editor = dialog.locator(".tiptap.ProseMirror");
    await editor.click();
    await editor.fill("fix the bug");

    // Click the enhance prompt button.
    const enhanceBtn = dialog.getByLabel("Enhance prompt with AI");
    await expect(enhanceBtn).toBeVisible({ timeout: 5_000 });
    await expect(enhanceBtn).toBeEnabled();
    await enhanceBtn.click();

    // Wait for enhanced text to replace input.
    await expect(editor).toHaveText(
      "Enhanced: please fix the null pointer bug in the user service",
      { timeout: 10_000 },
    );
  });

  test("supports multiple chat tabs and switching between them", async ({ testPage }) => {
    test.setTimeout(90_000);

    const dialog = await openQuickChatWithAgent(testPage);

    // Send a message in the first tab.
    await sendQuickChatMessage(dialog, testPage, "/e2e:simple-message");
    await expect(
      dialog.getByText("simple mock response for e2e testing", { exact: false }),
    ).toBeVisible({ timeout: 30_000 });

    // Create a new tab.
    const newChatBtn = dialog.getByLabel("Start new chat");
    await newChatBtn.click();

    // Agent picker should appear.
    await expect(dialog.getByText("Choose an agent to start chatting")).toBeVisible({
      timeout: 5_000,
    });

    // Select agent for the new tab.
    const agentCard = dialog
      .locator("button")
      .filter({ has: testPage.locator(".rounded-md.border") })
      .first();
    await agentCard.click();

    // Wait for chat input in new tab.
    await expect(dialog.locator(".tiptap.ProseMirror")).toBeVisible({ timeout: 15_000 });

    // Send a message in the second tab using script mode.
    await sendQuickChatMessage(dialog, testPage, 'e2e:message("second tab response")');
    await expect(dialog.getByText("second tab response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });

    // Switch back to the first tab by clicking its tab button.
    const tabBar = dialog.locator(".scrollbar-hide").first();
    const firstTab = tabBar.locator("button").first();
    await firstTab.click();

    // First tab content should still be visible.
    await expect(
      dialog.getByText("simple mock response for e2e testing", { exact: false }),
    ).toBeVisible({ timeout: 10_000 });
  });
});
