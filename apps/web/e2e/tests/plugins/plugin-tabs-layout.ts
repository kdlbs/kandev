import { expect, type Page } from "@playwright/test";

export async function expectPluginTabsContained(page: Page, expectedHeight: number) {
  await page.goto("/settings/plugins");
  const tabs = page.getByRole("tablist");
  await expect(page.getByTestId("plugins-tab-installed")).toBeVisible();
  const active = page.getByTestId("plugins-tab-installed");
  const activeBackground = await active.evaluate((e) => getComputedStyle(e).backgroundColor);
  const barBackground = await tabs.evaluate((e) => getComputedStyle(e).backgroundColor);
  expect(activeBackground).not.toBe(barBackground);
  expect(activeBackground).not.toBe("rgba(0, 0, 0, 0)");
  const outer = (await tabs.boundingBox())!;
  const buttons = await tabs.getByRole("tab").all();
  expect(buttons.length).toBeGreaterThanOrEqual(3);
  for (const button of buttons) {
    const box = (await button.boundingBox())!;
    expect(box.height).toBe(expectedHeight);
    expect(box.y).toBeGreaterThanOrEqual(outer.y);
    expect(box.y + box.height).toBeLessThanOrEqual(outer.y + outer.height);
    expect(box.x + box.width).toBeLessThanOrEqual(outer.x + outer.width);
  }
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.getByTestId("plugins-tab-themes").click();
  await expect(page.getByTestId("plugins-tab-themes")).toHaveAttribute("aria-selected", "true");
  await expect(page.getByTestId("theme-import-trigger")).toBeVisible();
}
