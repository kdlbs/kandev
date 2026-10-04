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
    const launcher = page.getByTestId("mobile-quick-chat-button");
    const menu = page.getByTestId("app-nav-trigger");
    if (!(await menu.isVisible())) return;
    if ((await menu.getAttribute("aria-expanded")) !== "true") await menu.tap();
    await expect(menu).toHaveAttribute("aria-expanded", "true");
    await page.getByRole("dialog", { name: "Menu", exact: true }).waitFor();
    await launcher.tap({ timeout: 10_000 });
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
  // @covers AC-UI-QUICK-TERMINAL-001.8, AC-UI-QUICK-TERMINAL-001.13
  test("shortcuts interrupt the selected shell and retain keyboard focus", async ({
    testPage,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    await testPage.goto("/");
    try {
      await testPage.getByTestId("app-nav-trigger").tap();
      await testPage.getByTestId("mobile-quick-terminal-button").tap();
      const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
      await expect(dialog).toBeVisible();
      await waitForTerminalReady(testPage);
      const bar = dialog.getByTestId("mobile-terminal-keybar");
      await expect(dialog.getByRole("button", { name: "Control", exact: true })).toBeVisible();
      await expect(bar).toHaveCount(1);
      for (const id of [
        "ctrl",
        "shift",
        "ctrl-c",
        "ctrl-d",
        "esc",
        "tab",
        "up",
        "down",
        "left",
        "right",
        "home",
        "end",
        "pageup",
        "pagedown",
        "pipe",
        "tilde",
        "slash",
        "dash",
        "underscore",
      ]) {
        const key = bar.getByTestId(`keybar-key-${id}`);
        await key.scrollIntoViewIfNeeded();
        const box = await key.boundingBox();
        expect(box!.height).toBeGreaterThanOrEqual(44);
        expect(box!.width).toBeGreaterThanOrEqual(44);
      }
      await bar.getByTestId("keybar-key-ctrl").scrollIntoViewIfNeeded();
      await testPage.getByTestId("quick-terminal-terminal").tap();
      await testPage.keyboard.type("printf 'FIRST%s\\n' '_SHELL_READY'\n");
      await expect
        .poll(async () => normalizeTerminalText(await readQuickTerminalBuffer(testPage)))
        .toContain("FIRST_SHELL_READY");
      const tabs = dialog.getByTestId("quick-terminal-tab");
      await dialog.getByTestId("quick-chat-add-menu-trigger").tap();
      await testPage.getByTestId("quick-chat-new-terminal").tap();
      await expect(tabs).toHaveCount(2);
      await waitForTerminalReady(testPage);
      await testPage.getByTestId("quick-terminal-terminal").tap();
      await testPage.keyboard.type("sleep 30\n");
      await bar.getByTestId("keybar-key-ctrl-c").tap();
      await expect(dialog.locator(".xterm-helper-textarea")).toBeFocused();
      // Assembled output cannot match the echoed command text.
      await testPage.keyboard.type("printf 'SECOND%s\\n' '_INTERRUPTED'\n");
      await expect
        .poll(async () => normalizeTerminalText(await readQuickTerminalBuffer(testPage)), {
          timeout: 10_000,
        })
        .toContain("SECOND_INTERRUPTED");
      await testPage.keyboard.type("discard_this");
      await bar.getByTestId("keybar-key-ctrl").tap();
      await testPage.keyboard.type("u");
      await expect(bar.getByTestId("keybar-key-ctrl")).toHaveAttribute("aria-pressed", "false");
      await testPage.keyboard.type("printf 'LATCH%s\\n' '_CLEARED'\n");
      await expect
        .poll(async () => normalizeTerminalText(await readQuickTerminalBuffer(testPage)))
        .toContain("LATCH_CLEARED");
      await tabs.nth(0).tap();
      await expect
        .poll(async () => normalizeTerminalText(await readQuickTerminalBuffer(testPage)))
        .toContain("FIRST_SHELL_READY");
      expect(normalizeTerminalText(await readQuickTerminalBuffer(testPage))).not.toContain(
        "SECOND_INTERRUPTED",
      );
      await testPage.evaluate(() => {
        const vv = window.visualViewport!;
        Object.defineProperty(vv, "height", {
          configurable: true,
          value: window.innerHeight - 300,
        });
        vv.dispatchEvent(new Event("resize"));
      });
      await expect
        .poll(async () => {
          const box = await bar.boundingBox();
          return box!.y + box!.height;
        })
        .toBeLessThanOrEqual(testPage.viewportSize()!.height - 300 + 1);
      const terminalBox = await dialog.getByTestId("quick-terminal-terminal").boundingBox();
      const barBox = await bar.boundingBox();
      expect(terminalBox!.y + terminalBox!.height).toBeLessThanOrEqual(barBox!.y + 1);
      await bar.getByTestId("keybar-key-tab").tap();
      await expect(dialog.locator(".xterm-helper-textarea")).toBeFocused();
      await assertNoDocumentHorizontalOverflow(testPage, "quick terminal shortcut controls");
      await prCapture.screenshot("phone-keyboard-controls", {
        caption: "Quick Terminal shortcuts above the on-screen keyboard",
      });
      await testPage.evaluate(() => {
        const vv = window.visualViewport!;
        Object.defineProperty(vv, "height", { configurable: true, value: window.innerHeight });
        vv.dispatchEvent(new Event("resize"));
      });
      await expect
        .poll(async () => (await bar.boundingBox())!.y)
        .toBeGreaterThan(testPage.viewportSize()!.height - 100);
      await prCapture.screenshot("phone-controls", {
        caption: "Quick Terminal shares the task terminal shortcut controls",
      });
      // Close descriptors before dismissal, while their hydrated tabs are visible.
      await closeQuickTerminalTab(testPage, tabs.nth(1));
      await expect(tabs).toHaveCount(1);
      await bar.getByTestId("keybar-key-ctrl-c").tap();
      await testPage.keyboard.type("cat; printf 'EOF%s\\n' '_RECEIVED'\n");
      await bar.getByTestId("keybar-key-ctrl-d").tap();
      await expect
        .poll(async () => normalizeTerminalText(await readQuickTerminalBuffer(testPage)))
        .toContain("EOF_RECEIVED");
      await testPage.keyboard.type("exit\n");
      await expect(dialog.getByTestId("quick-terminal-status")).toContainText("exited");
      await expect(bar).toHaveCount(0);
      await closeQuickTerminalTab(testPage, tabs.nth(0));
      await expect(tabs).toHaveCount(0);
      await expect(dialog).toBeHidden();
      await expect(testPage.getByTestId("mobile-terminal-keybar")).toHaveCount(0);
    } finally {
      await closeSurvivingQuickTerminals(testPage);
    }
  });

  test("uses a safe full-height surface, touch-safe menu, and contained terminal scroll", async ({
    testPage,
  }) => {
    test.setTimeout(120_000);
    await testPage.goto("/");
    try {
      const terminalButton = testPage.getByTestId("mobile-quick-terminal-button");
      const quickChatButton = testPage.getByTestId("mobile-quick-chat-button");
      const menuButton = testPage.getByTestId("app-nav-trigger");
      await expect(menuButton).toBeVisible();
      const headerMenuBox = await menuButton.boundingBox();
      expect(headerMenuBox).not.toBeNull();
      if (!headerMenuBox) throw new Error("mobile menu geometry unavailable");
      await menuButton.tap();
      await expect(terminalButton).toBeVisible();
      await expect(quickChatButton).toBeVisible();
      for (const button of [terminalButton, quickChatButton]) {
        const buttonBox = await button.boundingBox();
        expect(buttonBox).not.toBeNull();
        if (!buttonBox) throw new Error("mobile launcher geometry unavailable");
        expect(buttonBox.width).toBeGreaterThanOrEqual(44);
        expect(buttonBox.height).toBeGreaterThanOrEqual(44);
        expect(buttonBox.x + buttonBox.width).toBeLessThanOrEqual(testPage.viewportSize()!.width);
      }

      await terminalButton.tap();
      const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
      await expect(dialog).toBeVisible();
      await expect(dialog).toHaveClass(/pt-safe/);
      await expect(dialog).toHaveClass(/pb-safe/);
      await expect(dialog.getByTestId("quick-terminal-tab-panel")).toBeVisible();
      await waitForTerminalReady(testPage);
      await sendCommand(testPage, "echo", "MOBILE_TERMINAL_ONE");

      const terminalTabs = dialog.locator('[data-testid="quick-terminal-tab"]');
      await expect(terminalTabs).toHaveCount(1);
      const firstTab = terminalTabs.first();
      const firstSequence = await firstTab.getAttribute("data-terminal-sequence");
      if (!firstSequence) throw new Error("first Quick Chat terminal is missing its sequence");
      const firstActions = firstTab.getByRole("button", {
        name: `Actions for Terminal ${firstSequence}`,
      });
      const firstActionsBox = await firstActions.boundingBox();
      expect(firstActionsBox).not.toBeNull();
      expect(firstActionsBox!.width).toBeGreaterThanOrEqual(44);
      expect(firstActionsBox!.height).toBeGreaterThanOrEqual(44);
      await firstActions.tap();
      await expect(
        testPage.getByRole("menuitem", {
          name: `Close Terminal ${firstSequence}`,
          exact: true,
        }),
      ).toBeVisible();
      await testPage.keyboard.press("Escape");

      // A mobile reload restores the durable descriptor and reattaches the
      // detached PTY before the user creates any additional terminal.
      await testPage.reload();
      await menuButton.tap();
      await expect(terminalButton).toBeVisible();
      await terminalButton.tap();
      await expect(dialog).toBeVisible();
      await expect(terminalTabs).toHaveCount(1);
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
      await expect(
        menu.getByRole("menuitem", { name: `Terminal ${firstSequence}`, exact: true }),
      ).toHaveCount(0);
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

      await expect(terminalTabs).toHaveCount(2);
      const secondTab = terminalTabs.last();
      const secondSequence = await secondTab.getAttribute("data-terminal-sequence");
      if (!secondSequence) throw new Error("second Quick Chat terminal is missing its sequence");
      expect(secondSequence).not.toBe(firstSequence);
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
      await closeQuickTerminalTab(testPage, secondTab);
      await expect(terminalTabs).toHaveCount(1);
      await expect
        .poll(async () => normalizeTerminalText(await readQuickTerminalBuffer(testPage)))
        .toContain(normalizeTerminalText("MOBILE_TERMINAL_ONE"));

      await dialog.getByTestId("quick-chat-close").tap();
      await expect(dialog).toBeHidden();
      await expect(menuButton).toBeFocused();

      // Reopening through the mobile launcher selects the same terminal tab.
      await menuButton.tap();
      await terminalButton.tap();
      await expect(dialog).toBeVisible();
      await expect(terminalTabs).toHaveCount(1);
      await expect
        .poll(async () => normalizeTerminalText(await readQuickTerminalBuffer(testPage)))
        .toContain(normalizeTerminalText("MOBILE_TERMINAL_ONE"));
      await dialog.getByTestId("quick-chat-close").tap();
      await expect(dialog).toBeHidden();
      await expect(menuButton).toBeFocused();
    } finally {
      await closeSurvivingQuickTerminals(testPage);
    }
  });
});
