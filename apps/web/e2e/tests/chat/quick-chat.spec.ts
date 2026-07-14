import { type Locator, type Page } from "@playwright/test";
import { execFileSync } from "node:child_process";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { attachAvailableCommandsCapture } from "../../helpers/ws-capture";

/**
 * Quick Chat E2E tests: basic flow, enhance prompt, queued messages, multi-tab.
 */

async function openQuickChatSetup(page: Page): Promise<Locator> {
  await page.goto("/");
  await page.waitForLoadState("networkidle");

  // Open Quick Chat via keyboard shortcut (Cmd+Shift+Q / Ctrl+Shift+Q).
  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await page.keyboard.press(`${modifier}+Shift+q`);

  // Wait for Quick Chat dialog.
  const dialog = page.getByRole("dialog", { name: "Quick Chat" });
  await expect(dialog).toBeVisible({ timeout: 10_000 });

  // If a stale session tab is showing, click "+" to start a fresh setup form.
  const setup = dialog.getByTestId("quick-chat-setup");
  if (!(await setup.isVisible({ timeout: 1_000 }).catch(() => false))) {
    await dialog.getByLabel("Start new chat").click();
  }
  await expect(setup).toBeVisible({ timeout: 5_000 });
  return dialog;
}

async function selectAgentIfNeeded(dialog: Locator, page: Page) {
  const selector = dialog.getByTestId("agent-profile-selector");
  if (
    await selector
      .getByText("Select agent", { exact: false })
      .isVisible()
      .catch(() => false)
  ) {
    await selector.click();
    await page.getByRole("option").first().click();
  }
}

async function startQuickChatFromSetup(dialog: Locator, page: Page) {
  await selectAgentIfNeeded(dialog, page);
  await dialog.getByTestId("quick-chat-start").click();

  // Wait for chat input to appear AND become editable. Eager init means the
  // agent starts during the HTTP request, so the input is briefly disabled
  // while the FE store catches up to the RUNNING session state.
  const editor = dialog.locator(".tiptap.ProseMirror");
  await expect(editor).toBeVisible({ timeout: 15_000 });
  await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 30_000 });
}

async function openQuickChatWithAgent(page: Page): Promise<Locator> {
  const dialog = await openQuickChatSetup(page);
  await startQuickChatFromSetup(dialog, page);
  return dialog;
}

async function sendQuickChatMessage(dialog: Locator, page: Page, text: string) {
  const editor = dialog.locator(".tiptap.ProseMirror");
  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  // With eager init, the agent boots during picker -> tab transition and the
  // input can briefly toggle disabled while the FE store catches up. Retry the
  // full edit action so fill() cannot race a contenteditable=false flip.
  await expect(async () => {
    await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 1_000 });
    await editor.click({ timeout: 1_000 });
    await editor.fill(text, { timeout: 1_000 });
    await expect(editor).toHaveText(text, { timeout: 1_000 });
    await editor.press(`${modifier}+Enter`, { timeout: 1_000 });
    await expect(editor).toHaveText("", { timeout: 2_000 });
  }).toPass({ timeout: 30_000, intervals: [250, 500, 1_000] });
}

async function waitForQuickChatWidth(dialog: Locator) {
  await expect
    .poll(() =>
      dialog.evaluate((element) => {
        const preferred = Number.parseFloat(
          getComputedStyle(element).getPropertyValue("--quick-chat-width"),
        );
        return Math.abs(element.getBoundingClientRect().width - preferred);
      }),
    )
    .toBeLessThan(2);
}

test.describe("Quick Chat", () => {
  test("resizes from either edge, restores width, and keeps tab actions adjacent", async ({
    testPage,
  }) => {
    const dialog = await openQuickChatSetup(testPage);
    await waitForQuickChatWidth(dialog);
    const initialBox = await dialog.boundingBox();
    expect(initialBox).not.toBeNull();

    const rightHandle = dialog.getByTestId("quick-chat-resize-right");
    await expect(rightHandle).toBeVisible();
    const rightBox = await rightHandle.boundingBox();
    expect(rightBox).not.toBeNull();
    await testPage.mouse.move(
      rightBox!.x + rightBox!.width / 2,
      rightBox!.y + rightBox!.height / 2,
    );
    await testPage.mouse.down();
    await expect.poll(() => testPage.evaluate(() => document.body.style.cursor)).toBe("ew-resize");
    await testPage.mouse.move(
      rightBox!.x + rightBox!.width / 2 + 50,
      rightBox!.y + rightBox!.height / 2,
    );
    await testPage.mouse.up();
    await waitForQuickChatWidth(dialog);

    const rightResizedBox = await dialog.boundingBox();
    expect(rightResizedBox!.width).toBeGreaterThan(initialBox!.width + 80);

    const leftHandle = dialog.getByTestId("quick-chat-resize-left");
    const leftBox = await leftHandle.boundingBox();
    expect(leftBox).not.toBeNull();
    await testPage.mouse.move(leftBox!.x + leftBox!.width / 2, leftBox!.y + leftBox!.height / 2);
    await testPage.mouse.down();
    await testPage.mouse.move(
      leftBox!.x + leftBox!.width / 2 - 40,
      leftBox!.y + leftBox!.height / 2,
    );
    await testPage.mouse.up();
    await waitForQuickChatWidth(dialog);

    const finalBox = await dialog.boundingBox();
    expect(finalBox!.width).toBeGreaterThan(rightResizedBox!.width + 50);
    expect(finalBox!.x + finalBox!.width / 2).toBeCloseTo(
      (await testPage.evaluate(() => window.innerWidth)) / 2,
      0,
    );

    const tab = dialog.getByTestId("quick-chat-tab").last();
    const newChat = dialog.getByLabel("Start new chat");
    const tabBox = await tab.boundingBox();
    const newChatBox = await newChat.boundingBox();
    expect(newChatBox!.x - (tabBox!.x + tabBox!.width)).toBeLessThanOrEqual(8);

    await startQuickChatFromSetup(dialog, testPage);
    const surfaces = await dialog.evaluate((element) => {
      const messages = element.querySelector<HTMLElement>('[data-testid="quick-chat-messages"]');
      const input = element.querySelector<HTMLElement>('[data-testid="chat-input-area"]');
      return {
        messages: messages ? getComputedStyle(messages).backgroundColor : null,
        input: input ? getComputedStyle(input).backgroundColor : null,
      };
    });
    expect(surfaces.input).toBe(surfaces.messages);

    await testPage.keyboard.press("Escape");
    await expect(dialog).not.toBeVisible();
    await testPage.reload();
    await testPage.waitForLoadState("networkidle");
    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await testPage.keyboard.press(`${modifier}+Shift+q`);
    await expect(dialog).toBeVisible();
    await waitForQuickChatWidth(dialog);
    const restoredBox = await dialog.boundingBox();
    expect(restoredBox!.width).toBeCloseTo(finalBox!.width, 0);
  });

  test("explains quick chat and starts with repository context", async ({
    testPage,
    seedData,
    backend,
  }) => {
    const sourceRepo = path.join(backend.tmpDir, "repos", "e2e-repo");
    execFileSync("git", ["branch", "-f", "context-branch"], { cwd: sourceRepo });
    const dialog = await openQuickChatSetup(testPage);
    await expect(dialog.getByTestId("quick-chat-introduction")).toContainText(
      "Chat with an agent about an idea, question, or codebase.",
    );
    await expect(
      dialog.getByText("Add repository context to focus on specific code and branches."),
    ).toBeVisible();
    await selectAgentIfNeeded(dialog, testPage);

    await dialog.getByTestId("add-repository").click();
    await dialog.getByTestId("repo-chip-trigger").click();
    await testPage.getByRole("option").first().click();
    await dialog.getByTestId("branch-chip-trigger").click();
    await testPage.getByRole("option", { name: "context-branch" }).click();

    const startRequest = testPage.waitForRequest(
      (request) => request.url().includes("/quick-chat") && request.method() === "POST",
    );
    await dialog.getByTestId("quick-chat-start").click();
    const payload = (await startRequest).postDataJSON() as {
      repositories?: Array<{ repository_id: string; base_branch: string }>;
    };
    expect(payload.repositories).toEqual([
      { repository_id: seedData.repositoryId, base_branch: "context-branch" },
    ]);
    await expect(dialog.locator(".tiptap.ProseMirror")).toBeVisible({ timeout: 30_000 });
    expect(
      execFileSync("git", ["branch", "--show-current"], {
        cwd: sourceRepo,
        encoding: "utf8",
      }).trim(),
    ).toBe("main");
  });

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

    // Type initial text. Re-gate on the editor being editable: eager init can
    // flip it back to contenteditable=false (agent briefly RUNNING) after the
    // open helper's initial check, and fill() requires an editable element.
    const editor = dialog.locator(".tiptap.ProseMirror");
    await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 30_000 });
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
    const availableCommands = attachAvailableCommandsCapture(testPage);

    const dialog = await openQuickChatWithAgent(testPage);

    // Wait for the available_commands WS frame to land. Eager init kicks off
    // session/new during the HTTP request, but the agent emits commands
    // asynchronously after the response flushes — so the frame can arrive
    // moments after openQuickChatWithAgent resolves.
    await expect
      .poll(() => availableCommands.frames.some((frame) => frame.count > 0), { timeout: 15_000 })
      .toBe(true);

    const editor = dialog.locator(".tiptap.ProseMirror");
    await editor.click();
    await editor.pressSequentially("/");

    // SlashCommandMenu renders into a portal at document root, so query at page level.
    await expect(testPage.getByText("Commands").first()).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText("/slow")).toBeVisible({ timeout: 5_000 });
    await expect(testPage.getByText("/error")).toBeVisible({ timeout: 5_000 });
  });

  test("model selector shows dynamic session options before first message", async ({
    testPage,
  }) => {
    const dialog = await openQuickChatWithAgent(testPage);

    const trigger = dialog.getByRole("button", { name: "Session model settings" });
    await expect(trigger).toContainText("Mock Fast", { timeout: 15_000 });
    await trigger.click();

    const effortTrigger = testPage.getByTestId("config-option-trigger-effort");
    await expect(effortTrigger).toBeVisible({
      timeout: 10_000,
    });
    await effortTrigger.click();
    await expect(testPage.getByTestId("config-option-section-effort")).toBeVisible({
      timeout: 10_000,
    });
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

    // Setup should appear without the first-use introduction once a chat exists.
    await expect(dialog.getByTestId("quick-chat-setup")).toBeVisible({ timeout: 5_000 });
    await expect(dialog.getByTestId("quick-chat-introduction")).not.toBeVisible();
    await startQuickChatFromSetup(dialog, testPage);

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
