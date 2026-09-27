import { expect, type Locator } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { seedPluginExecutorStatusTask } from "../../helpers/plugin-executor-status";
import { uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import { SessionPage } from "../../pages/session-page";

async function expectTouchTarget(locator: Locator, label: string) {
  const box = await locator.boundingBox();
  expect(box, `${label} must have geometry`).not.toBeNull();
  expect(box!.height, `${label} must be at least 44px tall`).toBeGreaterThanOrEqual(44);
  expect(box!.width, `${label} must be at least 44px wide`).toBeGreaterThanOrEqual(44);
}

test("phone drawer exposes cleanup retry and status with touch-sized actions", async ({
  apiClient,
  backend,
  seedData,
  testPage,
}) => {
  test.setTimeout(120_000);
  const releaseFeature = await backend.useEnv({ KANDEV_FEATURES_REMOTE_EXECUTOR_PLUGINS: "true" });
  let taskId = "";
  try {
    const seeded = await seedPluginExecutorStatusTask(testPage, {
      backend,
      apiClient,
      seedData,
      state: "cleanup_pending",
      touch: true,
    });
    taskId = seeded.taskId;
    let resetRequested = false;
    await testPage.route(`**/api/v1/tasks/${taskId}/environment/reset`, async (route) => {
      resetRequested = true;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: '{"success":true}',
      });
    });

    await testPage.goto(`/t/${taskId}`);
    await new SessionPage(testPage).waitForLoad();
    const trigger = testPage.getByTestId("executor-settings-button");
    await expectTouchTarget(trigger, "Environment drawer trigger");
    await trigger.tap();
    const drawer = testPage.getByTestId("executor-settings-drawer");
    await expect(drawer).toBeVisible();
    await expectTouchTarget(drawer.getByRole("button", { name: "Close" }), "Drawer close action");
    await expect(drawer.getByTestId("plugin-executor-status-message")).toContainText(
      "Resource removal is unconfirmed.",
    );
    await expect(drawer.getByTestId("executor-settings-reset")).toHaveText("Retry cleanup");
    await expectTouchTarget(drawer.getByTestId("executor-settings-refresh"), "Refresh action");
    await expectTouchTarget(
      drawer.getByTestId("executor-settings-link"),
      "Executor settings action",
    );
    await expectTouchTarget(drawer.getByTestId("executor-settings-reset"), "Cleanup retry action");

    await drawer.getByTestId("executor-settings-reset").tap();
    const confirm = testPage.getByTestId("reset-env-confirm");
    await expect(confirm).toBeDisabled();
    await testPage.getByText("I understand any uncommitted changes will be lost.").tap();
    await expect(confirm).toBeEnabled();
    await confirm.tap();
    await expect.poll(() => resetRequested).toBe(true);
    await assertNoDocumentHorizontalOverflow(testPage, "plugin executor status drawer");
  } finally {
    if (taskId) await apiClient.deleteTask(taskId).catch(() => undefined);
    await uninstallFixturePlugin(apiClient);
    await releaseFeature();
  }
});
