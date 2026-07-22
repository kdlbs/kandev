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
    const leftOrderingId = `plugin:${PLUGIN_ID}:app-status-bar-left:0`;
    const rightOrderingId = `plugin:${PLUGIN_ID}:app-status-bar-right:0`;
    const orderResponse = await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      system_metrics_display: { show_in_topbar: true },
      app_status_bar_order: {
        left_item_ids: [rightOrderingId, "builtin:metrics"],
        right_item_ids: ["builtin:connection", leftOrderingId],
      },
    });
    expect(orderResponse.ok).toBe(true);
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
    const statusRows = drawer.locator("[data-status-item-id]");
    await expect(statusRows).toHaveCount(4);
    expect(
      await statusRows.evaluateAll((rows) => rows.map((row) => row.dataset.statusItemId)),
    ).toEqual([rightOrderingId, "builtin:metrics", "builtin:connection", leftOrderingId]);
    for (const row of await statusRows.all()) {
      expect((await row.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    }
    let orderPatchCount = 0;
    testPage.on("request", (request) => {
      if (request.method() === "PATCH" && request.url().endsWith("/api/v1/user/settings")) {
        orderPatchCount += 1;
      }
    });
    const [firstRowBox, lastRowBox] = await Promise.all([
      statusRows.first().boundingBox(),
      statusRows.last().boundingBox(),
    ]);
    if (!firstRowBox || !lastRowBox) throw new Error("mobile status row geometry unavailable");
    await testPage.keyboard.down("Control");
    await testPage.mouse.move(
      firstRowBox.x + firstRowBox.width / 2,
      firstRowBox.y + firstRowBox.height / 2,
    );
    await testPage.mouse.down();
    await testPage.mouse.move(
      lastRowBox.x + lastRowBox.width / 2,
      lastRowBox.y + lastRowBox.height / 2,
      { steps: 6 },
    );
    await testPage.mouse.up();
    await testPage.keyboard.up("Control");
    expect(orderPatchCount).toBe(0);
    expect(
      await statusRows.evaluateAll((rows) => rows.map((row) => row.dataset.statusItemId)),
    ).toEqual([rightOrderingId, "builtin:metrics", "builtin:connection", leftOrderingId]);
    expect(await drawer.locator("[class*='overflow-y-auto']").count()).toBe(1);
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
      await testPage.evaluate(() => document.documentElement.clientWidth),
    );
    // Chromium may consume Escape immediately after the synthetic modifier drag.
    await testPage
      .locator('[data-slot="drawer-overlay"][data-state="open"]')
      .click({ position: { x: 4, y: 4 } });
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
