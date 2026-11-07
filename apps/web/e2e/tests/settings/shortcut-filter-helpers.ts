import { expect, type Page } from "@playwright/test";

// @covers AC-UI-SHORTCUT-SETTINGS-FILTER-001.1, .2, .3, .4
export async function exerciseShortcutFilter(page: Page) {
  await page.goto("/settings/preferences/keyboard-shortcuts");
  const search = page.getByRole("textbox", { name: "Search shortcuts" });
  const core = page.getByTestId("shortcut-recorder-SEARCH");
  const github = page.getByTestId("shortcut-recorder-integration:github");
  await expect(core).toBeVisible();
  await expect(github).toBeVisible();
  await core.click();
  await expect(core).toHaveAttribute("data-shortcut-recording", "true");
  await search.click();
  await page.keyboard.type("github");
  await expect(search).toHaveValue("github");
  await expect(github).toBeVisible();
  const clear = page.getByRole("button", { name: "Clear search" });
  await clear.focus();
  await page.keyboard.press("Enter");
  await expect(search).toBeFocused();
  await expect(search).toHaveValue("");
  await expect(core).toHaveAttribute("data-settings-dirty", "false");
  await expect(core).toHaveAttribute("data-shortcut-recording", "false");
  await search.fill("  CoMmAnD PaNeL  ");
  await expect(core).toBeVisible();
  await expect(github).toHaveCount(0);
  await search.fill("  GITHUB  ");
  await expect(github).toBeVisible();
  await expect(core).toHaveCount(0);
  await github.click();
  await page.keyboard.press("Control+k");
  await expect(github).toHaveAttribute("data-settings-dirty", "true");
  await search.fill("command panel");
  await expect(core).toBeVisible();
  await expect(page.getByTitle("Same shortcut as: Open GitHub")).toBeVisible();
  await search.fill("no such shortcut");
  await expect(page.getByText("No matching shortcuts", { exact: true })).toHaveCount(1);
  await page.getByRole("button", { name: "Clear search" }).click();
  await expect(core).toBeVisible();
  await expect(github).toHaveAttribute("data-settings-dirty", "true");
  await expect(github).toContainText("K");
  await search.fill("  GITHUB  ");
  return { search, clear: page.getByRole("button", { name: "Clear search" }), github };
}
