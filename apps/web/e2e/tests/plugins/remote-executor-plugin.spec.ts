import { expect, test } from "../../fixtures/test-base";
import { seedPluginExecutorStatusTask } from "../../helpers/plugin-executor-status";
import { uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import { SessionPage } from "../../pages/session-page";
import { KanbanPage } from "../../pages/kanban-page";

test("packaged provider appears in a live task environment disclosure", async ({
  apiClient,
  backend,
  seedData,
  testPage,
}) => {
  test.setTimeout(120_000);
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
  }
});

test("built-in executor selection works when no provider plugin is installed", async ({
  apiClient,
  testPage,
}) => {
  const { executors } = await apiClient.listExecutors();
  expect(executors.some((executor) => executor.provider)).toBe(false);
  const worktreeProfile = executors.find((executor) => executor.type === "worktree")?.profiles?.[0];
  expect(worktreeProfile, "the built-in worktree profile must remain available").toBeDefined();

  const kanban = new KanbanPage(testPage);
  await kanban.goto();
  await kanban.createTaskButton.first().click();
  const dialog = testPage.getByTestId("create-task-dialog");
  const selector = dialog.getByTestId("executor-profile-selector");
  await expect(selector).toBeVisible();
  await selector.click();
  const option = testPage.getByRole("option", { name: worktreeProfile!.name });
  await expect(option).toBeVisible();
  await option.click();
  await expect(selector).toContainText(worktreeProfile!.name);
});
