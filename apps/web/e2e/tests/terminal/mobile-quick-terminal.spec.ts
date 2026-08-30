import { type Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { closeQuickTerminalTab } from "./terminal-test-helpers";

async function readQuickTerminalBuffer(page: Page): Promise<string> {
  return page.evaluate(() => {
    const container = document.querySelector('[data-testid="quick-terminal-terminal"]') as
      | (HTMLDivElement & { __xtermReadBuffer?: () => string })
      | null;
    return container?.__xtermReadBuffer?.() ?? "";
  });
}

function normalizeTerminalText(text: string): string {
  // xterm can wrap a marker across visual lines on narrow mobile viewports.
  return text.replace(/\s+/g, "");
}

async function waitForTerminalReady(page: Page) {
  await expect
    .poll(async () => normalizeTerminalText(await readQuickTerminalBuffer(page)), {
      timeout: 30_000,
      message: "Waiting for mobile Quick Chat terminal shell prompt",
    })
    .not.toBe("");
}

async function sendCommand(page: Page, command: string, marker: string) {
  await page.getByTestId("quick-terminal-terminal").tap();
  await page.keyboard.type(`${command} ${marker}`);
  await page.keyboard.press("Enter");
  await expect
    .poll(async () => normalizeTerminalText(await readQuickTerminalBuffer(page)), {
      timeout: 20_000,
      message: `Waiting for mobile terminal marker ${marker}`,
    })
    .toContain(normalizeTerminalText(marker));
}

async function closeSurvivingQuickTerminals(page: Page) {
  const dialog = page.getByRole("dialog", { name: "Quick Chat" });
  if (!(await dialog.isVisible().catch(() => false))) {
    const launcher = page.getByTestId("mobile-quick-terminal-button");
    if (await launcher.isVisible().catch(() => false)) await launcher.tap();
  }
  if (!(await dialog.isVisible().catch(() => false))) return;

  const tabs = dialog.locator('[data-testid="quick-terminal-tab"]');
  for (let attempts = 0; attempts < 8; attempts += 1) {
    const count = await tabs.count();
    if (count === 0) return;
    await closeQuickTerminalTab(page, tabs.nth(count - 1));
    await expect(tabs).toHaveCount(count - 1, { timeout: 10_000 });
  }
}

test.describe("mobile quick terminal tabs", () => {
  test("uses a safe full-height surface, touch-safe menu, and contained terminal scroll", async ({
    testPage,
  }) => {
    test.setTimeout(120_000);
    await testPage.goto("/");
    try {
      const terminalButton = testPage.getByTestId("mobile-quick-terminal-button");
      const quickChatButton = testPage.getByTestId("mobile-quick-chat-button");
      const menuButton = testPage.getByTestId("mobile-topbar-menu");
      await expect(terminalButton).toBeVisible();
      await expect(quickChatButton).toBeVisible();
      await expect(menuButton).toBeVisible();
      const headerMenuBox = await menuButton.boundingBox();
      expect(headerMenuBox).not.toBeNull();
      if (!headerMenuBox) throw new Error("mobile menu geometry unavailable");
      for (const button of [terminalButton, quickChatButton]) {
        const buttonBox = await button.boundingBox();
        expect(buttonBox).not.toBeNull();
        if (!buttonBox) throw new Error("mobile launcher geometry unavailable");
        expect(buttonBox.width).toBeCloseTo(headerMenuBox.width, 1);
        expect(buttonBox.height).toBeCloseTo(headerMenuBox.height, 1);
      }

      await terminalButton.tap();
      const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
      await expect(dialog).toBeVisible();
      await expect(dialog).toHaveClass(/pt-safe/);
      await expect(dialog).toHaveClass(/pb-safe/);
      await expect(dialog.getByTestId("quick-terminal-tab-panel")).toBeVisible();
      await waitForTerminalReady(testPage);
      await sendCommand(testPage, "echo", "MOBILE_TERMINAL_ONE");

      const firstTab = dialog.locator(
        '[data-testid="quick-terminal-tab"][data-terminal-sequence="1"]',
      );
      const firstActions = firstTab.getByRole("button", { name: "Actions for Terminal 1" });
      const firstActionsBox = await firstActions.boundingBox();
      expect(firstActionsBox).not.toBeNull();
      expect(firstActionsBox!.width).toBeGreaterThanOrEqual(44);
      expect(firstActionsBox!.height).toBeGreaterThanOrEqual(44);
      await firstActions.tap();
      await expect(
        testPage.getByRole("menuitem", { name: "Close Terminal 1", exact: true }),
      ).toBeVisible();
      await testPage.keyboard.press("Escape");

      // A mobile reload restores the durable descriptor and reattaches the
      // detached PTY before the user creates any additional terminal.
      await testPage.reload();
      await expect(terminalButton).toBeVisible();
      await terminalButton.tap();
      await expect(dialog).toBeVisible();
      await expect(dialog.locator('[data-testid="quick-terminal-tab"]')).toHaveCount(1);
      await expect
        .poll(async () => normalizeTerminalText(await readQuickTerminalBuffer(testPage)))
        .toContain(normalizeTerminalText("MOBILE_TERMINAL_ONE"));

      const viewport = testPage.viewportSize();
      const dialogBox = await dialog.boundingBox();
      const terminalBox = await dialog.getByTestId("quick-terminal-terminal").boundingBox();
      expect(viewport).not.toBeNull();
      expect(dialogBox).not.toBeNull();
      expect(terminalBox).not.toBeNull();
      expect(dialogBox!.x).toBeGreaterThanOrEqual(-1);
      expect(dialogBox!.y).toBeGreaterThanOrEqual(-1);
      expect(dialogBox!.x + dialogBox!.width).toBeLessThanOrEqual(viewport!.width + 1);
      expect(dialogBox!.y + dialogBox!.height).toBeLessThanOrEqual(viewport!.height + 1);
      expect(dialogBox!.width).toBeGreaterThanOrEqual(viewport!.width - 8);
      expect(dialogBox!.height).toBeGreaterThanOrEqual(viewport!.height - 8);
      expect(terminalBox!.x).toBeGreaterThanOrEqual(dialogBox!.x - 1);
      expect(terminalBox!.x + terminalBox!.width).toBeLessThanOrEqual(
        dialogBox!.x + dialogBox!.width + 1,
      );
      expect(terminalBox!.y).toBeGreaterThanOrEqual(dialogBox!.y - 1);
      expect(terminalBox!.y + terminalBox!.height).toBeLessThanOrEqual(
        dialogBox!.y + dialogBox!.height + 1,
      );
      await assertNoDocumentHorizontalOverflow(testPage, "mobile shared Quick Chat");

      // The plus trigger and bottom-sheet rows retain the touch-safe 44px target.
      const addTrigger = dialog.getByTestId("quick-chat-add-menu-trigger");
      const triggerBox = await addTrigger.boundingBox();
      expect(triggerBox).not.toBeNull();
      expect(triggerBox!.width).toBeGreaterThanOrEqual(44);
      expect(triggerBox!.height).toBeGreaterThanOrEqual(44);
      await addTrigger.tap();
      await expect(testPage.getByText("Agents", { exact: true })).toBeVisible();
      const menu = testPage.locator('[data-slot="dropdown-menu-content"]');
      await expect(menu).toBeVisible();
      await expect(menu.getByRole("menuitem", { name: "Terminal 1", exact: true })).toHaveCount(0);
      const menuBox = await menu.boundingBox();
      expect(menuBox).not.toBeNull();
      expect(menuBox!.y).toBeGreaterThan(viewport!.height / 2);
      expect(menuBox!.y + menuBox!.height).toBeLessThanOrEqual(viewport!.height + 1);
      const newTerminal = testPage.getByTestId("quick-chat-new-terminal");
      await expect
        .poll(async () => Math.round((await newTerminal.boundingBox())?.height ?? 0), {
          message: "Waiting for the mobile menu row animation to settle",
        })
        .toBeGreaterThanOrEqual(44);
      await newTerminal.tap();

      await expect(dialog.locator('[data-testid="quick-terminal-tab"]')).toHaveCount(2);
      await waitForTerminalReady(testPage);
      await sendCommand(testPage, "echo", "MOBILE_TERMINAL_TWO");
      await sendCommand(testPage, "seq 1 160; echo", "MOBILE_SCROLL_MARKER");
      const xtermViewport = dialog
        .getByTestId("quick-terminal-terminal")
        .locator(".xterm-viewport");
      await expect
        .poll(() =>
          xtermViewport.evaluate((element) => element.scrollHeight >= element.clientHeight),
        )
        .toBe(true);
      await assertNoDocumentHorizontalOverflow(testPage, "mobile shared terminal tabs");

      // Explicit close stops one sibling and returns to the first tab.
      await closeQuickTerminalTab(
        testPage,
        dialog.locator('[data-testid="quick-terminal-tab"][data-terminal-sequence="2"]'),
      );
      await expect(dialog.locator('[data-testid="quick-terminal-tab"]')).toHaveCount(1);
      await expect
        .poll(async () => normalizeTerminalText(await readQuickTerminalBuffer(testPage)))
        .toContain(normalizeTerminalText("MOBILE_TERMINAL_ONE"));

      await dialog.getByTestId("quick-chat-close").tap();
      await expect(dialog).toBeHidden();
      await expect(terminalButton).toBeFocused();

      // Reopening through the mobile launcher selects the same terminal tab.
      await terminalButton.tap();
      await expect(dialog).toBeVisible();
      await expect(dialog.locator('[data-testid="quick-terminal-tab"]')).toHaveCount(1);
      await expect
        .poll(async () => normalizeTerminalText(await readQuickTerminalBuffer(testPage)))
        .toContain(normalizeTerminalText("MOBILE_TERMINAL_ONE"));
      await dialog.getByTestId("quick-chat-close").tap();
      await expect(dialog).toBeHidden();
    } finally {
      await closeSurvivingQuickTerminals(testPage);
    }
  });
});
