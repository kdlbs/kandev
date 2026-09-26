import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { seedPluginExecutorStatusTask } from "../../helpers/plugin-executor-status";
import { uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import { SessionPage } from "../../pages/session-page";

test("remote executor disclosure explains expiry and keeps reset authorized", async ({
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
      state: "expired",
      touch: false,
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
    await testPage.getByTestId("executor-settings-button").click();
    const disclosure = testPage.getByTestId("executor-settings-popover");
    await expect(disclosure).toBeVisible();
    await expect(disclosure.getByTestId("plugin-executor-status-message")).toContainText(
      "The workspace expired and cannot be resumed.",
    );
    await expect(disclosure.getByTestId("plugin-executor-retention")).toContainText(
      "Workspace data may be lost when compute expires.",
    );
    await expect(disclosure.getByTestId("plugin-executor-expires-at")).not.toHaveText("");
    await expect(disclosure.getByTestId("executor-settings-link")).toHaveAttribute(
      "href",
      new RegExp(`/settings/executors/${seeded.profileId}`),
    );

    await disclosure.getByTestId("executor-settings-reset").click();
    const confirm = testPage.getByTestId("reset-env-confirm");
    await expect(confirm).toBeDisabled();
    await testPage.getByText("I understand any uncommitted changes will be lost.").click();
    await expect(confirm).toBeEnabled();
    await confirm.click();
    await expect.poll(() => resetRequested).toBe(true);
    await assertNoDocumentHorizontalOverflow(testPage, "plugin executor expiry disclosure");
  } finally {
    if (taskId) await apiClient.deleteTask(taskId).catch(() => undefined);
    await uninstallFixturePlugin(apiClient);
    await releaseFeature();
  }
});
