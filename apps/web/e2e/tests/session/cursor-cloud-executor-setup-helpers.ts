import { expect, type Page } from "@playwright/test";
import type { BackendContext } from "../../fixtures/backend";
import type { ApiClient } from "../../helpers/api-client";

export async function createCloudProfileFromSettings(
  page: Page,
  apiClient: ApiClient,
  backend: BackendContext,
  mobile: boolean,
) {
  const name = mobile ? "Cloud setup phone" : "Cloud setup desktop";
  const secret = await apiClient.createSecret(name, "cursor-cloud-e2e-key");
  try {
    await page.goto("/settings/executors");
    const setup = page.getByRole("button", { name: /Cursor Cloud/ });
    await expect(setup).toHaveCount(1);
    if (mobile) await setup.tap();
    else await setup.click();
    await expect(page).toHaveURL(/\/settings\/executors\/new\/cursor_cloud$/);
    await page.locator("#profile-name").fill(name);
    await page.getByRole("combobox").click();
    await page.getByRole("option", { name, exact: true }).click();
    const callback = `${backend.baseUrl}/api/v1/managed-agent-mcp`;
    await page.locator('input[type="url"]').fill(callback);
    await page.getByRole("button", { name: "Test connection" }).click();
    await expect(page.getByText("Mock Cursor Model", { exact: true })).toBeVisible();
    const save = page.getByRole("button", { name: "Save changes", exact: true });
    if (mobile) {
      await save.scrollIntoViewIfNeeded();
      expect((await save.boundingBox())?.height).toBeGreaterThanOrEqual(44);
      await save.tap();
    } else await save.click();
    await expect(page).toHaveURL(/\/settings\/executors\/(?!new\/)[^/]+$/);
    await page.reload();
    await expect(page.locator("#profile-name")).toHaveValue(name);
    await expect(page.locator('input[type="url"]')).toHaveValue(callback);
    await expect(
      page
        .locator('[data-slot="card"]')
        .filter({ has: page.locator('input[type="url"]') })
        .getByRole("combobox"),
    ).toContainText(name);
    expect(
      (await apiClient.listAvailableAgents()).agents.some((a) => a.name === "cursor_cloud"),
    ).toBe(true);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
      ),
    ).toBeLessThanOrEqual(0);
  } finally {
    const { executors } = await apiClient.listExecutors();
    for (const executor of executors.filter((e) => e.type === "cursor_cloud" && e.name === name)) {
      await apiClient.deleteExecutor(executor.id);
    }
    await apiClient.deleteSecretIfPresent(secret.id);
  }
}
