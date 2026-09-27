import fs from "node:fs";
import path from "node:path";
import { expect, test } from "../../fixtures/test-base";
import { seedPluginExecutorStatusTask } from "../../helpers/plugin-executor-status";
import { uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import { SessionPage } from "../../pages/session-page";

test("packaged provider appears in a live task environment disclosure", async ({
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
    await testPage.goto(`/t/${taskId}`);
    await new SessionPage(testPage).waitForLoad();
    await testPage.getByTestId("executor-settings-button").click();
    const disclosure = testPage.getByTestId("executor-settings-popover");
    await expect(disclosure.getByTestId("plugin-executor-status-message")).toBeVisible();
    await expect(disclosure.getByTestId("plugin-executor-retention")).toContainText(
      "compute expires",
    );
    await expect(disclosure.getByTestId("plugin-executor-expires-at")).not.toHaveText("");
    await expect(disclosure.getByTestId("executor-settings-link")).toHaveAttribute(
      "href",
      new RegExp(`/settings/executors/${seeded.profileId}`),
    );
  } finally {
    if (taskId) await apiClient.deleteTask(taskId).catch(() => undefined);
    await uninstallFixturePlugin(apiClient);
    await releaseFeature();
  }
});

test("feature-off provider admission disables profile creation without provisioning", async ({
  apiClient,
  backend,
  testPage,
}) => {
  test.setTimeout(120_000);
  const releaseFeature = await backend.useEnv({ KANDEV_FEATURES_REMOTE_EXECUTOR_PLUGINS: "false" });
  try {
    await testPage.goto("/settings/plugins");
    const { installFixtureExecutorProvider } =
      await import("../../helpers/plugin-executor-profile");
    const executor = await installFixtureExecutorProvider(testPage, apiClient);
    await testPage.goto("/settings/executors");

    const providerCard = testPage.getByTestId(`executor-profiles-card-${executor.id}`);
    await expect(providerCard).toBeVisible();
    await expect(providerCard.getByRole("button", { name: "Add" })).toBeDisabled();
    await expect(providerCard.getByRole("status")).toBeVisible();

    const inventoryPath = path.join(
      backend.tmpDir,
      ".kandev",
      "plugins",
      "kandev-plugin-e2e",
      "data",
      "executor-resources.json",
    );
    expect(fs.existsSync(inventoryPath)).toBe(false);
  } finally {
    await uninstallFixturePlugin(apiClient);
    await releaseFeature();
  }
});
