import { expect, type Page } from "@playwright/test";
export async function fileViewerMarketplaceFlow(page: Page) {
  const extensions: (string | null)[] = [];
  await page.route("**/api/plugins/marketplace?*", async (route) => {
    extensions.push(new URL(route.request().url()).searchParams.get("extension"));
    await route.fulfill({ json: { plugins: [], sources: [] } });
  });
  await page.goto("/settings/plugins?tab=browse&extension=STL");
  const chip = page.getByRole("button", { name: "File type: stl (clear)", exact: true });
  await expect(chip).toBeVisible();
  await expect(
    page.getByText("No plugins match your search. Try clearing the filters.", { exact: true }),
  ).toBeVisible();
  expect(extensions).toContain("stl");
  await chip.click();
  await expect(chip).toHaveCount(0);
  await expect.poll(() => extensions.at(-1)).toBeNull();
  await expect(
    page.getByText(
      "No plugins available yet. Check back soon, or add a marketplace source with your own.",
      { exact: true },
    ),
  ).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
}
