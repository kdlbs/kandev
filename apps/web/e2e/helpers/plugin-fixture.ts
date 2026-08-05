import path from "node:path";
import { type Page, expect } from "@playwright/test";

export const PLUGIN_ID = "kandev-plugin-e2e";
export const PACKAGE_PATH = path.resolve(
  __dirname,
  "../../../backend/.build/kandev-plugin-e2e-1.0.0.tar.gz",
);

export async function installFixturePlugin(page: Page): Promise<void> {
  await page.goto("/settings/plugins");
  await page.getByTestId("install-plugin-trigger").click();
  await expect(page.getByTestId("install-plugin-dialog")).toBeVisible();
  await page.getByTestId("install-plugin-tab-upload").click();
  await page.getByTestId("install-plugin-file-input").setInputFiles(PACKAGE_PATH);
  await page.getByTestId("install-plugin-upload-submit").click();
  await expect(page.getByTestId(`plugin-row-${PLUGIN_ID}`)).toBeVisible({ timeout: 30_000 });
}
