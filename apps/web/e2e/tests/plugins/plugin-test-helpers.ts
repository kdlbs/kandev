import path from "node:path";
import type { Page } from "@playwright/test";
import { expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

export const PLUGIN_ID = "kandev-plugin-e2e";

export const PACKAGE_PATH = path.resolve(
  __dirname,
  "../../../../../apps/backend/.build/kandev-plugin-e2e-1.0.0.tar.gz",
);

export async function openInstallDialog(page: Page) {
  await page.goto("/settings/plugins");
  await page.getByTestId("install-plugin-trigger").click();
  await expect(page.getByTestId("install-plugin-dialog")).toBeVisible();
}

export async function uploadPackage(page: Page, filePath: string) {
  await page.getByTestId("install-plugin-tab-upload").click();
  await page.getByTestId("install-plugin-file-input").setInputFiles(filePath);
  await page.getByTestId("install-plugin-upload-submit").click();
}

export async function uninstallPluginFixture(apiClient: ApiClient) {
  await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
}
