import { expect, test } from "../../fixtures/test-base";
import { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { closeQuickTerminalTab } from "../terminal/terminal-test-helpers";

test("reopening the most recent terminal reveals its tab in an overflowing row", async ({
  testPage: page,
  apiClient,
}) => {
  test.setTimeout(120_000);
  await page.setViewportSize({ width: 900, height: 800 });
  await apiClient.saveUserSettings({
    sidebar_fast_actions_enabled: true,
    sidebar_new_task_style: "compact",
  });
  await page.goto("/");
  const launcher = page.getByTestId("sidebar-quick-terminal-shortcut");
  await launcher.click();
  const dialog = page.getByRole("dialog", { name: "Quick Chat" });
  const tabs = dialog.getByTestId("quick-terminal-tab");
  await expect(tabs).toHaveCount(1);
  for (let count = 2; count <= 8; count += 1) {
    await dialog.getByTestId("quick-chat-add-menu-trigger").click();
    await page.getByTestId("quick-chat-new-terminal").click();
    await expect(tabs).toHaveCount(count);
  }
  const last = tabs.last();
  const sequence = await last.getAttribute("data-terminal-sequence");
  await expect(
    last.getByRole("button", { name: `Terminal ${sequence}`, exact: true }),
  ).toHaveAttribute("aria-current", "page");
  await dialog.getByTestId("quick-chat-add-menu-trigger").focus();
  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();
  const before = await page.evaluate(() => ({
    documentY: window.scrollY,
    focus: (document.activeElement as HTMLElement | null)?.outerHTML.slice(0, 300),
  }));
  await launcher.click();
  await expect(dialog).toBeVisible();
  const selected = dialog.getByTestId("quick-terminal-tab").last();
  await expect(
    selected.getByRole("button", { name: `Terminal ${sequence}`, exact: true }),
  ).toHaveAttribute("aria-current", "page");
  const geometry = await selected.evaluate((tab) => {
    const strip = tab.closest(".overflow-x-auto")!;
    const row = strip.getBoundingClientRect();
    const bounds = tab.getBoundingClientRect();
    return {
      sequence: tab.getAttribute("data-terminal-sequence"),
      rowLeft: row.left,
      rowRight: row.right,
      tabLeft: bounds.left,
      tabRight: bounds.right,
      scrollLeft: strip.scrollLeft,
      scrollWidth: strip.scrollWidth,
      clientWidth: strip.clientWidth,
      documentY: window.scrollY,
      dialogY: tab.closest('[role="dialog"]')?.scrollTop,
      focus: (document.activeElement as HTMLElement | null)?.outerHTML.slice(0, 300),
    };
  });
  console.log("ACTIVE_TAB_EVIDENCE", JSON.stringify({ before, after: geometry }));
  expect(geometry.scrollWidth).toBeGreaterThan(geometry.clientWidth);
  expect(geometry.tabLeft).toBeGreaterThanOrEqual(geometry.rowLeft - 1);
  expect(geometry.tabRight).toBeLessThanOrEqual(geometry.rowRight + 1);
  expect(geometry.documentY).toBe(before.documentY);
  expect(geometry.dialogY).toBe(0);
  expect(geometry.focus).toContain('data-terminal-sequence="1"');
  await assertNoDocumentHorizontalOverflow(page, "desktop Quick Chat tab reveal");
  const capture = new PrAssetCapture(page, "quick-chat-active-tab-visibility.spec.ts", {
    captureKey: "desktop",
  });
  if (capture.capturing) {
    await waitForFiniteAnimations(dialog);
    await capture.screenshot("active-terminal", {
      caption: "The selected terminal remains visible in the overflowing Quick Chat row.",
    });
    capture.flush();
  }
  for (let count = 8; count > 0; count -= 1) {
    await closeQuickTerminalTab(page, tabs.last());
    await expect(tabs).toHaveCount(count - 1);
  }
});
