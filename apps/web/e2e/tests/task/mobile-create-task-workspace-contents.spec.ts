import fs from "node:fs";
import path from "node:path";
import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { useRegularMode } from "../../helpers/regular-mode";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

useRegularMode();

test("mobile task creation adds a folder through one contents sheet", async ({
  testPage,
  apiClient,
  backend,
}) => {
  const folderPath = path.join(backend.tmpDir, "mobile-task-assets");
  fs.mkdirSync(folderPath, { recursive: true });

  const mobile = new MobileKanbanPage(testPage);
  await mobile.goto();
  await mobile.mobileFab.tap();
  const dialog = testPage.getByTestId("create-task-dialog");
  await expect(dialog).toBeVisible();
  const { executors } = await apiClient.listExecutors();
  const localExecutor = executors.find((executor) => ["local", "local_pc"].includes(executor.type));
  const localProfile = localExecutor?.profiles?.[0];
  expect(localProfile, "a direct local executor profile is required by the fixture").toBeDefined();
  await dialog.getByTestId("executor-profile-selector").tap();
  await testPage.getByRole("option", { name: new RegExp(localProfile!.name) }).tap();

  const manager = testPage.getByTestId("mobile-repository-manager");
  await manager.tap();
  const sheet = testPage.getByTestId("mobile-repository-sheet-content");
  await expect(sheet).toBeVisible();
  const management = sheet.getByTestId("mobile-repository-management");
  if (await management.isVisible().catch(() => false)) {
    await sheet.getByTestId("mobile-repository-add").tap();
  }

  const options = sheet.getByTestId("workspace-source-menu-options");
  const labels = await options
    .locator("button")
    .evaluateAll((buttons) =>
      buttons.map((button) => button.querySelector("span.font-medium")?.textContent?.trim()),
    );
  expect(labels).toEqual(["Repository", "Local Folder", "Repository Set"]);
  for (const option of await options.locator("button").all()) {
    await expect(option).toBeVisible();
    expect((await option.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
  }
  await options.getByTestId("workspace-source-menu-folder").tap();

  const picker = sheet.getByTestId("folder-picker-inline");
  await expect(picker).toBeVisible();
  await picker.getByTestId("folder-picker-path-input").fill(folderPath);
  await picker.getByTestId("folder-picker-path-go").tap();
  await expect(picker.getByTestId("folder-picker-choose")).toBeEnabled({ timeout: 15_000 });
  await picker.getByTestId("folder-picker-choose").tap();

  await expect(sheet).not.toBeVisible();
  await manager.tap();
  await expect(sheet.getByTestId("workspace-folder-selection")).toContainText("mobile-task-assets");
  await expect(sheet.getByTestId("repo-chip")).toHaveCount(1);
  await testPage.getByTestId("mobile-repository-done").tap();
  await assertNoDocumentHorizontalOverflow(testPage, "mobile task workspace contents");

  await dialog.getByTestId("task-title-input").fill("Mobile task with a local folder");
  await dialog.getByTestId("task-description-input").fill("Keep mobile workspace contents ordered");
  const responsePromise = testPage.waitForResponse(
    (response) =>
      response.url().endsWith("/api/v1/tasks") && response.request().method() === "POST",
  );
  await dialog.getByRole("button", { name: "Create only", exact: true }).tap();
  const response = await responsePromise;
  expect(response.ok()).toBe(true);
  const created = (await response.json()) as { id: string };
  await expect(dialog).not.toBeVisible({ timeout: 10_000 });

  const task = await apiClient.getTask(created.id);
  expect(task.repositories).toEqual([expect.objectContaining({ position: 0 })]);
  expect(task.workspace_folders).toEqual([
    expect.objectContaining({
      local_path: folderPath,
      display_name: "mobile-task-assets",
      position: 1,
    }),
  ]);
});
