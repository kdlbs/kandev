import { type Locator, type Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { quickChatTabReferences, startQuickChatFromSetup } from "./quick-chat-helpers";

async function openMobileQuickChat(page: Page): Promise<Locator> {
  await page.goto("/");
  await page.waitForLoadState("networkidle");
  await page.getByTestId("mobile-quick-chat-button").tap();
  const dialog = page.getByRole("dialog", { name: "Quick Chat" });
  await expect(dialog).toBeVisible({ timeout: 10_000 });
  return dialog;
}

async function closeSurvivingQuickChatTerminals(page: Page) {
  const dialog = page.getByRole("dialog", { name: "Quick Chat" });
  if (!(await dialog.isVisible().catch(() => false))) return;

  const terminals = dialog.getByTestId("quick-terminal-tab");
  for (let attempts = 0; attempts < 8; attempts += 1) {
    const count = await terminals.count();
    if (count === 0) return;
    const tab = terminals.nth(count - 1);
    const closeLabel = await tab.getAttribute("data-terminal-sequence");
    if (closeLabel) {
      await tab.getByRole("button", { name: `Actions for Terminal ${closeLabel}` }).tap();
      await page.getByRole("menuitem", { name: `Close Terminal ${closeLabel}` }).tap();
    }
    await expect(terminals)
      .toHaveCount(count - 1, { timeout: 10_000 })
      .catch(() => undefined);
  }
}

test.describe("mobile quick chat tabs", () => {
  test("moves mixed tabs, exposes rename, and keeps touch targets in the viewport", async ({
    testPage,
  }) => {
    test.setTimeout(120_000);
    const dialog = await openMobileQuickChat(testPage);
    expect(await testPage.evaluate(() => window.matchMedia("(pointer: coarse)").matches)).toBe(
      true,
    );

    try {
      const firstStart = testPage.waitForResponse(
        (response) =>
          response.url().includes("/quick-chat") && response.request().method() === "POST",
      );
      await startQuickChatFromSetup(dialog, testPage);
      const first = (await (await firstStart).json()) as { session_id: string };
      const firstReference = `conversation:${first.session_id}`;

      // Add a terminal before the second conversation. The stable baseline
      // keeps conversations first until the user chooses a mixed order.
      await dialog.getByTestId("quick-chat-add-menu-trigger").tap();
      await testPage.getByTestId("quick-chat-new-terminal").tap();
      await expect(dialog.getByTestId("quick-terminal-tab")).toHaveCount(1, {
        timeout: 15_000,
      });
      await expect(dialog.getByTestId("quick-terminal-terminal")).toBeVisible({ timeout: 15_000 });
      const terminalSortable = dialog.locator(
        '[data-testid="quick-chat-sortable-tab"][data-tab-reference^="terminal:"]',
      );
      const terminalReference = await terminalSortable.getAttribute("data-tab-reference");
      if (!terminalReference) throw new Error("quick terminal reference was not rendered");
      const terminalSequence = await dialog
        .getByTestId("quick-terminal-tab")
        .getAttribute("data-terminal-sequence");
      if (!terminalSequence) throw new Error("quick terminal sequence was not rendered");

      await dialog.getByTestId("quick-chat-add-menu-trigger").tap();
      await testPage.getByTestId("quick-chat-new-agent").tap();
      const secondStart = testPage.waitForResponse(
        (response) =>
          response.url().includes("/quick-chat") && response.request().method() === "POST",
      );
      await startQuickChatFromSetup(dialog, testPage);
      const second = (await (await secondStart).json()) as { session_id: string };
      const secondReference = `conversation:${second.session_id}`;

      await expect
        .poll(() => quickChatTabReferences(dialog), { timeout: 15_000 })
        .toEqual([firstReference, secondReference, terminalReference]);

      const terminalTab = dialog.getByTestId("quick-terminal-tab");
      await terminalTab
        .getByRole("button", { name: `Actions for Terminal ${terminalSequence}` })
        .tap();
      await testPage
        .getByRole("menuitem", { name: `Move Terminal ${terminalSequence} left` })
        .tap();
      await expect
        .poll(() => quickChatTabReferences(dialog), { timeout: 15_000 })
        .toEqual([firstReference, terminalReference, secondReference]);

      const firstTab = dialog.locator(
        `[data-tab-reference="${firstReference}"] [data-testid="quick-chat-tab"]`,
      );
      await firstTab.getByRole("button", { name: /^Actions for / }).tap();
      await testPage.getByRole("menuitem", { name: "Rename" }).tap();
      const renameInput = firstTab.getByRole("textbox", { name: "Rename chat" });
      await expect(renameInput).toBeVisible();
      await renameInput.fill("Phone renamed");
      await renameInput.press("Enter");
      await expect(firstTab.getByTestId("quick-chat-tab-name")).toHaveText("Phone renamed");

      const controls = dialog.locator(
        '[data-testid="quick-chat-tab"] button, [data-testid="quick-terminal-tab"] button',
      );
      const sizes = await controls.evaluateAll((buttons) =>
        buttons.map((button) => {
          const rect = button.getBoundingClientRect();
          return { width: rect.width, height: rect.height };
        }),
      );
      expect(sizes.length).toBeGreaterThan(0);
      for (const size of sizes) {
        expect(size.width).toBeGreaterThanOrEqual(44);
        expect(size.height).toBeGreaterThanOrEqual(44);
      }
      await assertNoDocumentHorizontalOverflow(testPage, "mobile mixed Quick Chat tabs");

      await terminalTab
        .getByRole("button", { name: `Actions for Terminal ${terminalSequence}` })
        .tap();
      await testPage.getByRole("menuitem", { name: `Close Terminal ${terminalSequence}` }).tap();
      await expect(dialog.getByTestId("quick-terminal-tab")).toHaveCount(0);
    } finally {
      await closeSurvivingQuickChatTerminals(testPage);
    }
  });
});
