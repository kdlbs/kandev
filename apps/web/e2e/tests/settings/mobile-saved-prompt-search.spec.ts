import { expect, test } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";

async function expectSearchGeometry(page: Page) {
  const search = page.getByRole("searchbox", { name: "Search..." });
  const surface = search.locator("..");
  const box = await search.boundingBox();
  const parent = await surface.boundingBox();
  expect(box).not.toBeNull();
  expect(parent).not.toBeNull();
  expect(box!.height).toBeGreaterThanOrEqual(44);
  expect(box!.x).toBeGreaterThanOrEqual(parent!.x);
  expect(box!.x + box!.width).toBeLessThanOrEqual(parent!.x + parent!.width);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
}

test("searches saved prompts by touch on phone and fits narrow and coarse layouts", async ({
  testPage,
  desktopTestPage,
  coarseDesktopTestPage,
  apiClient,
  prCapture,
}) => {
  const ids: string[] = [];
  try {
    const first = await apiClient.createPrompt(
      "mobile-search-alpha",
      "First line\nHidden phone needle",
    );
    ids.push(first.id);
    const second = await apiClient.createPrompt("mobile-search-beta", "Other content");
    ids.push(second.id);
    await testPage.goto("/settings/prompts");
    const search = testPage.getByRole("searchbox", { name: "Search..." });
    const rows = testPage.getByTestId("prompt-list-item");
    await expect(rows.filter({ hasText: "mobile-search-alpha" })).toBeVisible();
    await expectSearchGeometry(testPage);
    await search.tap();
    await testPage.keyboard.type("NEEDLE");
    await expect(rows).toHaveCount(1);
    await expect(rows.first()).toHaveAttribute("data-prompt-name", "mobile-search-alpha");
    await prCapture.screenshot("saved-prompt-search-mobile");
    await search.fill("mobile-search-beta");
    await expect(rows).toHaveCount(1);
    await expect(rows.first()).toHaveAttribute("data-prompt-name", "mobile-search-beta");
    await search.fill("no-mobile-match");
    await expect(testPage.getByText("No results.")).toBeVisible();
    await search.fill("");
    await expect(rows.filter({ hasText: "mobile-search-alpha" })).toBeVisible();
    await expect(rows.filter({ hasText: "mobile-search-beta" })).toBeVisible();
    await desktopTestPage.setViewportSize({ width: 390, height: 844 });
    await desktopTestPage.goto("/settings/prompts");
    await expectSearchGeometry(desktopTestPage);
    await coarseDesktopTestPage.goto("/settings/prompts");
    await expectSearchGeometry(coarseDesktopTestPage);
  } finally {
    for (const id of ids) await apiClient.deletePrompt(id);
  }
});
