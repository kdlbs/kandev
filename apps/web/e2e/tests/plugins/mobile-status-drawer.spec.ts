import path from "node:path";
import type { Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";

const PLUGIN_ID = "kandev-plugin-e2e";
const PACKAGE_PATH = path.resolve(
  __dirname,
  "../../../../../apps/backend/.build/kandev-plugin-e2e-1.0.0.tar.gz",
);

async function installFixture(page: Page) {
  await page.goto("/settings/plugins");
  await page.getByTestId("install-plugin-trigger").click();
  await page.getByTestId("install-plugin-tab-upload").click();
  await page.getByTestId("install-plugin-file-input").setInputFiles(PACKAGE_PATH);
  await page.getByTestId("install-plugin-upload-submit").click();
  await expect(page.getByTestId(`plugin-row-${PLUGIN_ID}`)).toBeVisible({ timeout: 15_000 });
}

test.describe("Mobile Status drawer", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
  });

  test("opens native Status paths without a persistent phone footer", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await installFixture(testPage);
    await testPage.goto("/");
    await testPage.reload();

    await testPage.getByRole("button", { name: "Open menu" }).click();
    const statusTrigger = testPage.getByTestId("mobile-home-status-button");
    await expect(statusTrigger).toBeVisible();
    const triggerBox = await statusTrigger.boundingBox();
    expect(triggerBox?.height).toBeGreaterThanOrEqual(44);
    await statusTrigger.click();

    const drawer = testPage.getByTestId("app-status-drawer");
    await expect(drawer).toBeVisible();
    await expect(drawer.getByTestId("app-status-connection")).toBeVisible();
    await expect(testPage.locator("#hello-status-left")).toContainText("mobile-drawer no-task");
    await expect(testPage.getByTestId("app-status-bar")).toHaveCount(0);
    expect(await drawer.locator("[class*='overflow-y-auto']").count()).toBe(1);
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
      await testPage.evaluate(() => document.documentElement.clientWidth),
    );
    await testPage.keyboard.press("Escape");
    await expect(drawer).toBeHidden();

    const task = await apiClient.createTask(seedData.workspaceId, "Mobile status task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto(`/t/${task.id}`);
    await testPage.getByRole("button", { name: "Status" }).click();
    await expect(testPage.locator("#hello-status-left")).toContainText(`mobile-drawer ${task.id}`);
    await testPage.keyboard.press("Escape");

    await testPage.goto("/stats");
    const pageTopbarStatus = testPage.getByTestId("app-status-drawer-trigger");
    await expect(pageTopbarStatus).toBeVisible();
    await pageTopbarStatus.click();
    await expect(drawer).toBeVisible();
    await testPage.keyboard.press("Escape");
    await expect(pageTopbarStatus).toBeFocused();
  });
});
