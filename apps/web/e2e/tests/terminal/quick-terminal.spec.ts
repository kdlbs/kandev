import { type Locator, type Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { assertLocatorWithinViewportX } from "../../helpers/layout-assertions";

const QUICK_CHAT_TITLE = "Quick Chat";

async function readQuickTerminalBuffer(page: Page): Promise<string> {
  return page.evaluate(() => {
    const container = document.querySelector('[data-testid="quick-terminal-terminal"]') as
      | (HTMLDivElement & { __xtermReadBuffer?: () => string })
      | null;
    return container?.__xtermReadBuffer?.() ?? "";
  });
}

async function waitForTerminalReady(page: Page) {
  await expect
    .poll(() => readQuickTerminalBuffer(page), {
      timeout: 15_000,
      message: "Waiting for Quick Chat terminal shell prompt",
    })
    .not.toBe("");
}

async function sendMarker(page: Page, marker: string) {
  await page.getByTestId("quick-terminal-terminal").click();
  await page.keyboard.type(`echo ${marker}`);
  await page.keyboard.press("Enter");
  await expect
    .poll(() => readQuickTerminalBuffer(page), {
      timeout: 10_000,
      message: `Waiting for terminal marker ${marker}`,
    })
    .toContain(marker);
}

function terminalTab(dialog: Locator, sequence: number) {
  return dialog.locator(`[data-testid="quick-terminal-tab"][data-terminal-sequence="${sequence}"]`);
}

async function closeSurvivingQuickTerminals(page: Page, launcherTestId: string) {
  const dialog = page.getByRole("dialog", { name: QUICK_CHAT_TITLE });
  if (!(await dialog.isVisible().catch(() => false))) {
    const launcher = page.getByTestId(launcherTestId);
    if (await launcher.isVisible().catch(() => false)) await launcher.click();
  }
  if (!(await dialog.isVisible().catch(() => false))) return;

  const tabs = dialog.locator('[data-testid="quick-terminal-tab"]');
  for (let attempts = 0; attempts < 8; attempts += 1) {
    const count = await tabs.count();
    if (count === 0) return;
    await tabs
      .nth(count - 1)
      .getByRole("button", { name: /^Close Terminal \d+$/ })
      .click();
    await expect(tabs).toHaveCount(count - 1, { timeout: 10_000 });
  }
}

test.describe("quick terminal tabs", () => {
  test("creates, detaches, reuses, switches, and closes independent terminals", async ({
    testPage,
  }) => {
    await testPage.goto("/");
    try {
      const terminalButton = testPage.getByTestId("sidebar-quick-terminal-shortcut");
      const quickChatButton = testPage.getByTestId("sidebar-quick-chat-shortcut");
      await expect(terminalButton).toBeVisible();
      await expect(quickChatButton).toBeVisible();
      expect(
        await terminalButton.evaluate((element) =>
          element.nextElementSibling?.getAttribute("data-testid"),
        ),
      ).toBe("sidebar-quick-chat-shortcut");

      await terminalButton.click();
      const dialog = testPage.getByRole("dialog", { name: QUICK_CHAT_TITLE });
      await expect(dialog).toBeVisible();
      await expect(dialog.getByTestId("quick-terminal-tab-panel")).toBeVisible();
      await expect(dialog.getByTestId("quick-terminal-terminal")).toBeVisible();
      await expect(dialog.getByTestId("host-shell-done")).toHaveCount(0);
      await waitForTerminalReady(testPage);
      await sendMarker(testPage, "QUICK_TERMINAL_ONE");
      await expect(dialog.locator('[data-testid="quick-terminal-tab"]')).toHaveCount(1);

      // The descriptor and detached PTY survive a full page reload. The
      // launcher must reattach the existing session instead of creating a
      // second terminal, including the buffered marker output.
      await testPage.reload();
      await expect(terminalButton).toBeVisible();
      await terminalButton.click();
      await expect(dialog).toBeVisible();
      await expect(dialog.locator('[data-testid="quick-terminal-tab"]')).toHaveCount(1);
      await expect(dialog.getByTestId("quick-terminal-tab-panel")).toBeVisible();
      await expect.poll(() => readQuickTerminalBuffer(testPage)).toContain("QUICK_TERMINAL_ONE");

      // Dismissing the shared surface detaches the terminal but does not stop it.
      await testPage.keyboard.press("Escape");
      await expect(dialog).toBeHidden();
      await expect(terminalButton).toBeFocused();
      await expect(testPage.getByRole("tooltip", { name: "Quick terminal" })).toHaveCount(0);

      await terminalButton.click();
      await expect(dialog).toBeVisible();
      await expect(dialog.locator('[data-testid="quick-terminal-tab"]')).toHaveCount(1);
      await expect.poll(() => readQuickTerminalBuffer(testPage)).toContain("QUICK_TERMINAL_ONE");

      // The grouped menu's New Terminal action always creates a second PTY.
      await dialog.getByTestId("quick-chat-add-menu-trigger").click();
      await expect(testPage.getByText("Agents", { exact: true })).toBeVisible();
      await expect(testPage.getByText("Terminals", { exact: true })).toBeVisible();
      await testPage.getByTestId("quick-chat-new-terminal").click();
      await expect(dialog.locator('[data-testid="quick-terminal-tab"]')).toHaveCount(2);
      await waitForTerminalReady(testPage);
      await sendMarker(testPage, "QUICK_TERMINAL_TWO");

      const firstTab = terminalTab(dialog, 1);
      const secondTab = terminalTab(dialog, 2);
      await firstTab.getByRole("button", { name: "Terminal 1", exact: true }).click();
      await expect.poll(() => readQuickTerminalBuffer(testPage)).toContain("QUICK_TERMINAL_ONE");
      await secondTab.getByRole("button", { name: "Terminal 2", exact: true }).click();
      await expect.poll(() => readQuickTerminalBuffer(testPage)).toContain("QUICK_TERMINAL_TWO");

      // Closing one tab stops/removes only that tab and falls back to its sibling.
      await secondTab.getByRole("button", { name: "Close Terminal 2" }).click();
      await expect(dialog.locator('[data-testid="quick-terminal-tab"]')).toHaveCount(1);
      await expect.poll(() => readQuickTerminalBuffer(testPage)).toContain("QUICK_TERMINAL_ONE");

      // The chat launcher switches content kind without discarding the terminal.
      await testPage.keyboard.press("Escape");
      await expect(dialog).toBeHidden();
      await quickChatButton.click();
      await expect(dialog.getByTestId("quick-chat-setup")).toBeVisible({ timeout: 10_000 });
      await expect(firstTab).toBeVisible();
      await firstTab.getByRole("button", { name: "Terminal 1", exact: true }).click();
      await expect(dialog.getByTestId("quick-terminal-tab-panel")).toBeVisible();

      const dialogBox = await dialog.boundingBox();
      expect(dialogBox).not.toBeNull();
      expect(dialogBox!.width).toBeGreaterThan(820);
      expect(dialogBox!.height).toBeGreaterThan(420);
      await assertLocatorWithinViewportX(dialog, "desktop shared Quick Chat dialog");

      await testPage.keyboard.press("Escape");
      await expect(dialog).toBeHidden();
    } finally {
      await closeSurvivingQuickTerminals(testPage, "sidebar-quick-terminal-shortcut");
    }
  });

  test("keeps tablet launcher order and 44px hit targets", async ({ tabletTestPage }) => {
    const testPage = tabletTestPage;
    await testPage.goto("/");
    try {
      const terminalButton = testPage.getByTestId("tablet-quick-terminal-button");
      const quickChatButton = testPage.getByTestId("tablet-quick-chat-button");
      await expect(terminalButton).toBeVisible();
      await expect(quickChatButton).toBeVisible();
      expect(
        await terminalButton.evaluate((element) =>
          element.nextElementSibling?.getAttribute("data-testid"),
        ),
      ).toBe("tablet-quick-chat-button");

      for (const button of [terminalButton, quickChatButton]) {
        const box = await button.boundingBox();
        expect(box).not.toBeNull();
        expect(box!.width).toBeGreaterThanOrEqual(44);
        expect(box!.height).toBeGreaterThanOrEqual(44);
      }

      await terminalButton.click();
      const dialog = testPage.getByRole("dialog", { name: QUICK_CHAT_TITLE });
      await expect(dialog).toBeVisible();
      await expect(dialog.getByTestId("quick-terminal-terminal")).toBeVisible();
      const dialogBox = await dialog.boundingBox();
      expect(dialogBox).not.toBeNull();
      expect(dialogBox!.width).toBeGreaterThan(500);
      expect(dialogBox!.height).toBeGreaterThan(420);
      await assertLocatorWithinViewportX(dialog, "tablet shared Quick Chat dialog");

      await testPage.keyboard.press("Escape");
      await expect(dialog).toBeHidden();
      await expect(terminalButton).toBeFocused();
    } finally {
      await closeSurvivingQuickTerminals(testPage, "tablet-quick-terminal-button");
    }
  });
});
