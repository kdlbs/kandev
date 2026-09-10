import fs from "node:fs";
import path from "node:path";
import type { Locator, Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";

async function openDeleteDialog(page: Page, title: string) {
  const session = new SessionPage(page);
  await session.mobileSessionMenu.tap();
  const drawer = page
    .getByTestId("mobile-task-switcher-list")
    .locator("xpath=ancestor::*[@data-slot='drawer-content'][1]");
  await expect(drawer).toBeVisible();
  const taskRow = drawer.getByTestId("sidebar-task-item").filter({ hasText: title });
  await expect(taskRow).toBeVisible();
  await taskRow.locator("button.mobile-task-actions-button").tap();
  const menu = page.locator('[data-slot="context-menu-content"]:visible').last();
  await expect(menu).toBeVisible();
  await menu.getByRole("menuitem", { name: "Delete", exact: true }).tap();
  const dialog = page.getByRole("alertdialog");
  await expect(dialog).toBeVisible();
  await waitForFiniteAnimations(dialog);
  return dialog;
}

async function assertContainedTouchDialog(dialog: Locator): Promise<void> {
  const [dialogBox, viewport] = await Promise.all([
    dialog.boundingBox(),
    dialog.page().evaluate(() => ({ width: window.innerWidth, height: window.innerHeight })),
  ]);
  if (!dialogBox) throw new Error("mobile delete dialog has no layout box");
  expect(dialogBox.x).toBeGreaterThanOrEqual(12);
  expect(dialogBox.x + dialogBox.width).toBeLessThanOrEqual(viewport.width - 12);
  expect(dialogBox.y).toBeGreaterThanOrEqual(0);
  expect(dialogBox.y + dialogBox.height).toBeLessThanOrEqual(viewport.height);
  for (const name of ["Cancel", "Delete"]) {
    const actionBox = await dialog.getByRole("button", { name, exact: true }).boundingBox();
    if (!actionBox) throw new Error(`${name} action has no layout box`);
    expect(actionBox.height).toBeGreaterThanOrEqual(44);
  }
  await assertNoDocumentHorizontalOverflow(dialog.page(), "mobile delete consent dialog");
}

test.describe("Mobile delete discard consent", () => {
  test("deletes a clean task without rendering discard consent", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 393, height: 700 });
    const task = await apiClient.createTask(seedData.workspaceId, "Mobile clean delete", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto(`/t/${task.id}`);
    await expect(testPage.getByTestId("mobile-task-layout")).toBeVisible();

    const dialog = await openDeleteDialog(testPage, "Mobile clean delete");
    await assertContainedTouchDialog(dialog);
    await expect(dialog.getByTestId("delete-discard-worktree-checkbox")).toHaveCount(0);
    const deleteAction = dialog.getByRole("button", { name: "Delete", exact: true });
    await expect(deleteAction).toBeEnabled();
    await deleteAction.tap();
    await expect
      .poll(async () => (await apiClient.rawRequest("GET", `/api/v1/tasks/${task.id}`)).status, {
        timeout: 15_000,
      })
      .toBe(404);
  });

  test("requires touch consent before deleting a dirty worktree task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await testPage.setViewportSize({ width: 393, height: 700 });
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile dirty delete",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    await expect
      .poll(async () => (await apiClient.getTaskEnvironment(task.id))?.status ?? null, {
        timeout: 60_000,
        message: "the mobile dirty-delete worktree did not become ready",
      })
      .toBe("ready");
    const environment = await apiClient.getTaskEnvironment(task.id);
    const worktreePath = environment?.repos?.[0]?.worktree_path ?? environment?.worktree_path ?? "";
    if (!worktreePath) throw new Error("the mobile dirty-delete task has no worktree path");
    const dirtyFilePath = path.join(worktreePath, "mobile-delete-consent.txt");
    fs.writeFileSync(dirtyFilePath, "keep this local change\n", "utf8");

    await testPage.goto(`/t/${task.id}`);
    await expect(testPage.getByTestId("mobile-task-layout")).toBeVisible();
    const dialog = await openDeleteDialog(testPage, "Mobile dirty delete");
    await assertContainedTouchDialog(dialog);
    const discard = dialog.getByTestId("delete-discard-worktree-checkbox");
    const deleteAction = dialog.getByRole("button", { name: "Delete", exact: true });
    await expect(discard).toBeVisible();
    await expect(deleteAction).toBeDisabled();
    await discard.tap();
    await expect(deleteAction).toBeEnabled();
    await deleteAction.tap();

    await expect
      .poll(async () => (await apiClient.rawRequest("GET", `/api/v1/tasks/${task.id}`)).status, {
        timeout: 15_000,
      })
      .toBe(404);
    await expect.poll(() => fs.existsSync(dirtyFilePath), { timeout: 15_000 }).toBe(false);
  });
});
