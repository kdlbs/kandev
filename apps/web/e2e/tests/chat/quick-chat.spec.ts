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

  // Wait for chat input to appear AND become editable. Eager init means the
  // agent starts during the HTTP request, so the input is briefly disabled
  // while the FE store catches up to the RUNNING session state.
  const editor = dialog.locator(".tiptap.ProseMirror");
  await expect(editor).toBeVisible({ timeout: 15_000 });
  await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 30_000 });

  return dialog;
}

async function sendQuickChatMessage(dialog: Locator, page: Page, text: string) {
  const editor = dialog.locator(".tiptap.ProseMirror");
  // Wait for the editor to be editable. With eager init, the agent boots
  // during the picker → tab transition and the input is briefly disabled
  // while the FE store catches up to RUNNING. Multi-tab scenarios share an
  // agent slot so the second boot can take longer.
  await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 30_000 });
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

  test("slash command menu populates before first message (eager agent init)", async ({
    testPage,
  }) => {
    // Picking an agent in quick chat should boot the agent process eagerly,
    // so available_commands_update fires from session/new — the slash menu is
    // populated before the user sends their first prompt. Mock-agent emits
    // /slow, /error, /thinking, etc. on session/new (parity with real ACP
    // agents like OpenCode and Claude).
    // Hook WebSocket BEFORE navigating to capture all session.available_commands
    // frames into window.__wsAvailableCommands for assertion.
    await testPage.addInitScript(() => {
      const w = window as unknown as {
        __wsAvailableCommands: unknown[];
        __wsAll: unknown[];
      };
      w.__wsAvailableCommands = [];
      w.__wsAll = [];
      const OrigWS = window.WebSocket;
      const Hooked = function (this: WebSocket, url: string | URL, protocols?: string | string[]) {
        const ws = new OrigWS(url, protocols);
        w.__wsAll.push({ event: "open", url: String(url) });
        ws.addEventListener("message", (ev: MessageEvent) => {
          try {
            const m = JSON.parse(String(ev.data));
            w.__wsAll.push({ event: "msg", action: m.action });
            if (m.action === "session.available_commands") {
              w.__wsAvailableCommands.push({
                session_id: m.payload?.session_id,
                count: m.payload?.available_commands?.length ?? 0,
              });
            }
          } catch {
            // ignore
          }
        });
        return ws;
      } as unknown as typeof WebSocket;
      Hooked.prototype = OrigWS.prototype;
      Object.assign(Hooked, OrigWS);
      window.WebSocket = Hooked;
    });

    const dialog = await openQuickChatWithAgent(testPage);

    // Wait for the available_commands WS frame to land. Eager init kicks off
    // session/new during the HTTP request, but the agent emits commands
    // asynchronously after the response flushes — so the frame can arrive
    // moments after openQuickChatWithAgent resolves.
    await testPage.waitForFunction(
      () => {
        const w = window as unknown as { __wsAvailableCommands?: unknown[] };
        return (w.__wsAvailableCommands?.length ?? 0) > 0;
      },
      { timeout: 15_000 },
    );

    const editor = dialog.locator(".tiptap.ProseMirror");
    await editor.click();
    await editor.pressSequentially("/");

    // SlashCommandMenu renders into a portal at document root, so query at page level.
    await expect(testPage.getByText("Commands").first()).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText("/slow")).toBeVisible({ timeout: 5_000 });
    await expect(testPage.getByText("/error")).toBeVisible({ timeout: 5_000 });
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
    // The user message bubble also contains "second tab response" — match only
    // the agent reply (the rendered text without the surrounding script call).
    await expect(dialog.getByText("second tab response", { exact: true })).toBeVisible({
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
