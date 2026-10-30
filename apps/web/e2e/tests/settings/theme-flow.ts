import { expect, type Page } from "@playwright/test";
export const monokaiFixture = {
  name: "Monokai study",
  type: "dark",
  colors: {
    "editor.background": "#272822",
    "editor.foreground": "#f8f8f2",
    "sideBar.background": "#272822",
    "terminal.background": "#272822",
    "terminal.ansiGreen": "#a6e22e",
  },
  tokenColors: [
    { settings: { foreground: "#eeeeee", background: "#222222", fontStyle: "italic" } },
    { scope: ["keyword", "storage.type"], settings: { foreground: "#f92672" } },
    { scope: "string", settings: { foreground: "#e6db74" } },
  ],
};
export async function importAndSelectTheme(page: Page) {
  await page.goto("/settings/plugins?tab=themes");
  await page.getByTestId("theme-import-trigger").click();
  await page.getByTestId("theme-import-file").setInputFiles({
    name: "monokai-study.json",
    mimeType: "application/json",
    buffer: Buffer.from(JSON.stringify(monokaiFixture)),
  });
  await page.getByTestId("theme-review").click();
  await expect(page.getByTestId("theme-review-content")).toContainText("Monokai study");
  const confirm = page.getByTestId("theme-confirm");
  const bounds = await confirm.boundingBox();
  expect(bounds?.height).toBeGreaterThanOrEqual(44);
  const dialog = await page.getByRole("dialog").boundingBox();
  expect(dialog!.height).toBeLessThanOrEqual((await page.evaluate(() => innerHeight)) + 1);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.getByTestId("theme-confirm").click();
  await expect(page.getByTestId("theme-installed")).toBeVisible();
  await page.getByRole("dialog").getByRole("link", { name: "Appearance", exact: true }).click();
  await page.locator("#theme").click();
  await page.getByRole("option", { name: /Monokai study/ }).click();
  await expect
    .poll(() =>
      page.evaluate(() =>
        getComputedStyle(document.documentElement).getPropertyValue("--background").trim(),
      ),
    )
    .toBe("#272822");
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  await expect
    .poll(() => page.evaluate(() => localStorage.getItem("theme")))
    .toMatch(/^theme:imported-/);
  await page.reload();
  await expect
    .poll(() =>
      page.evaluate(() =>
        getComputedStyle(document.documentElement).getPropertyValue("--background").trim(),
      ),
    )
    .toBe("#272822");
  return page.evaluate(() => localStorage.getItem("theme")!);
}
