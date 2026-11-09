import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";

test("phone launcher reveals the selected terminal in an overflowing row", async ({
  testPage: page,
}) => {
  test.setTimeout(120_000);
  await page.goto("/");
  await page.getByTestId("app-nav-trigger").tap();
  await page.getByTestId("mobile-quick-terminal-button").tap();
  const dialog = page.getByRole("dialog", { name: "Quick Chat" });
  const tabs = dialog.getByTestId("quick-terminal-tab");
  await expect(tabs).toHaveCount(1);
  for (let count = 2; count <= 5; count += 1) {
    await dialog.getByTestId("quick-chat-add-menu-trigger").tap();
    await page.getByTestId("quick-chat-new-terminal").tap();
    await expect(tabs).toHaveCount(count);
  }
  const sequence = await tabs.last().getAttribute("data-terminal-sequence");
  await expect(
    tabs.last().getByRole("button", { name: `Terminal ${sequence}`, exact: true }),
  ).toHaveAttribute("aria-current", "page");
  await dialog.getByTestId("quick-chat-close").tap();
  await expect(dialog).toBeHidden();
  const beforeY = await page.evaluate(() => window.scrollY);
  await page.getByTestId("app-nav-trigger").tap();
  await page.getByTestId("mobile-quick-terminal-button").tap();
  await expect(dialog).toBeVisible();
  const selected = tabs.last();
  await expect(
    selected.getByRole("button", { name: `Terminal ${sequence}`, exact: true }),
  ).toHaveAttribute("aria-current", "page");
  await waitForFiniteAnimations(dialog);
  const geometry = await selected.evaluate((tab) => {
    const strip = tab.closest(".overflow-x-auto")!;
    const row = strip.getBoundingClientRect();
    const bounds = tab.getBoundingClientRect();
    return {
      rowLeft: row.left,
      rowRight: row.right,
      tabLeft: bounds.left,
      tabRight: bounds.right,
      scrollWidth: strip.scrollWidth,
      clientWidth: strip.clientWidth,
      documentY: window.scrollY,
      dialogY: tab.closest('[role="dialog"]')?.scrollTop,
    };
  });
  expect(geometry.scrollWidth).toBeGreaterThan(geometry.clientWidth);
  expect(geometry.tabLeft).toBeGreaterThanOrEqual(geometry.rowLeft - 1);
  expect(geometry.tabRight).toBeLessThanOrEqual(geometry.rowRight + 1);
  expect(geometry.documentY).toBe(beforeY);
  expect(geometry.dialogY).toBe(0);
  await assertNoDocumentHorizontalOverflow(page, "phone Quick Chat tab reveal");
  const capture = new PrAssetCapture(page, "mobile-quick-chat-active-tab-visibility.spec.ts", {
    captureKey: "phone",
  });
  if (capture.capturing) {
    await capture.screenshot("active-terminal", {
      caption: "The active terminal is visible in the phone tab row.",
    });
    capture.flush();
  }
  for (let count = 5; count > 0; count -= 1) {
    const tab = tabs.last();
    const tabSequence = await tab.getAttribute("data-terminal-sequence");
    await tab.getByRole("button", { name: `Actions for Terminal ${tabSequence}` }).tap();
    await page.getByRole("menuitem", { name: `Close Terminal ${tabSequence}` }).tap();
    await expect(tabs).toHaveCount(count - 1);
  }
});
