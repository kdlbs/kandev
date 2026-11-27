import type { Locator, Page } from "@playwright/test";

/**
 * Opens the workspace Repositories page's "Add repository" menu and picks the
 * local flow. Returns the discover/add-local-repository dialog.
 */
export async function openAddLocalRepositoryDialog(page: Page): Promise<Locator> {
  await page.getByRole("button", { name: "Add repository" }).click();
  await page.getByRole("menuitem", { name: "Local repository" }).click();
  return page.getByRole("dialog", { name: "Add Local Repository" });
}
