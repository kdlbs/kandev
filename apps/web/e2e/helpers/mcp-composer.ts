import { expect, type Page } from "@playwright/test";

export async function openMCPComposerCustomization(page: Page) {
  await page.getByTestId("mcp-status-trigger").click();
  const explorer = page.getByTestId("mcp-server-explorer");
  await expect(explorer).toBeVisible();
  await explorer.getByTestId("mcp-customize-tab").click();
  return explorer;
}
