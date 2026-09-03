import type { Locator } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import {
  assertLocatorWithinViewportX,
  assertNoDescendantOverflowsRight,
  assertNoDocumentHorizontalOverflow,
} from "../../helpers/layout-assertions";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { SessionPage } from "../../pages/session-page";

const LONG_TASK_TITLE = `Desktop cleanup confirmation ${"X".repeat(31)}`;

async function assertDesktopDeleteDialog(dialog: Locator): Promise<void> {
  await waitForFiniteAnimations(dialog);

  const title = dialog.locator('[data-slot="alert-dialog-title"]');
  const description = dialog.locator('[data-slot="alert-dialog-description"]');
  await expect(title).toBeVisible();
  await expect(description).toBeVisible();
  await expect(title).toHaveCSS("text-wrap", "balance");
  await expect(description).toHaveCSS("text-wrap", "pretty");
  await expect(description).toHaveCSS("text-align", "left");

  const labelledBy = await dialog.getAttribute("aria-labelledby");
  const describedBy = await dialog.getAttribute("aria-describedby");
  expect(labelledBy).toBe(await title.getAttribute("id"));
  expect(describedBy).toBe(await description.getAttribute("id"));
  expect(await description.locator("p").count()).toBeGreaterThan(0);
  expect(await description.locator("ul, ol").count()).toBeGreaterThan(0);

  await assertLocatorWithinViewportX(dialog, "desktop delete dialog");
  await assertNoDescendantOverflowsRight(dialog, "desktop delete dialog");
  await assertNoDocumentHorizontalOverflow(dialog.page(), "desktop delete dialog");

  const footer = dialog.locator('[data-slot="alert-dialog-footer"]');
  const cancel = dialog.locator('[data-slot="alert-dialog-cancel"]');
  const deleteAction = dialog.locator('[data-slot="alert-dialog-action"]');
  const [footerBox, cancelBox, deleteBox] = await Promise.all([
    footer.boundingBox(),
    cancel.boundingBox(),
    deleteAction.boundingBox(),
  ]);
  if (!footerBox || !cancelBox || !deleteBox) {
    throw new Error("desktop delete dialog footer has no rendered action boxes");
  }
  expect(Math.round(cancelBox.height)).toBeLessThan(44);
  expect(Math.round(deleteBox.height)).toBeLessThan(44);
  expect(cancelBox.y).toBeCloseTo(deleteBox.y, 1);
  expect(deleteBox.x + deleteBox.width).toBeCloseTo(footerBox.x + footerBox.width, 1);
  await expect(deleteAction).toHaveAttribute("data-variant", "destructive");
  await expect(deleteAction).not.toHaveClass(/bg-primary/);
}

test.describe("Desktop confirmation text hierarchy", () => {
  // @covers AC-UI-SURFACE-TEXT-HIERARCHY-001.1, AC-UI-SURFACE-TEXT-HIERARCHY-001.2
  // @covers AC-UI-SURFACE-TEXT-HIERARCHY-001.3, AC-UI-SURFACE-TEXT-HIERARCHY-001.5
  // @covers AC-UI-TASK-CLEANUP-CONFIRMATION-001.5, AC-UI-TASK-CLEANUP-CONFIRMATION-001.6
  // @covers AC-UI-TASK-CLEANUP-CONFIRMATION-001.7
  test("keeps the fine-pointer archive popover wide and the sidebar row stable", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 900, height: 700 });
    const title = "Desktop archive popover geometry target";
    const task = await apiClient.seedTask(seedData.workspaceId, title, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await testPage.goto(`/t/${task.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const taskRow = session.sidebarTaskItem(title);
    await expect(taskRow).toBeVisible();
    const heightBefore = await taskRow.evaluate(
      (element) => element.getBoundingClientRect().height,
    );

    await session.openSidebarMenuAndClick(title, "Archive");
    const popover = testPage.getByTestId("task-archive-confirm-popover");
    await expect(popover).toBeVisible();
    await waitForFiniteAnimations(popover);

    const popoverDescription = popover.locator('[data-slot="popover-description"]');
    await expect(popoverDescription).toBeVisible();
    await expect(popoverDescription).toHaveCSS("text-wrap", "pretty");
    const [heightOpen, popoverBox, viewport] = await Promise.all([
      taskRow.evaluate((element) => element.getBoundingClientRect().height),
      popover.boundingBox(),
      testPage.evaluate(() => ({ width: window.innerWidth, height: window.innerHeight })),
    ]);
    if (!popoverBox) throw new Error("desktop archive popover has no rendered layout box");

    expect(heightOpen).toBeCloseTo(heightBefore, 3);
    expect(popoverBox.width).toBeGreaterThan(256);
    expect(popoverBox.x).toBeGreaterThanOrEqual(0);
    expect(popoverBox.x + popoverBox.width).toBeLessThanOrEqual(viewport.width);
    expect(popoverBox.y).toBeGreaterThanOrEqual(0);
    expect(popoverBox.y + popoverBox.height).toBeLessThanOrEqual(viewport.height);
    await assertNoDocumentHorizontalOverflow(testPage, "desktop archive popover");

    await popover.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(popover).toBeHidden();
    const heightAfter = await taskRow.evaluate((element) => element.getBoundingClientRect().height);
    expect(heightAfter).toBeCloseTo(heightBefore, 3);
  });

  // @covers AC-UI-SURFACE-TEXT-HIERARCHY-001.1, AC-UI-SURFACE-TEXT-HIERARCHY-001.2
  // @covers AC-UI-SURFACE-TEXT-HIERARCHY-001.3, AC-UI-SURFACE-TEXT-HIERARCHY-001.5
  // @covers AC-UI-TASK-CLEANUP-CONFIRMATION-001.5, AC-UI-TASK-CLEANUP-CONFIRMATION-001.6
  // @covers AC-UI-TASK-CLEANUP-CONFIRMATION-001.7
  test("preserves Dialog and Delete behavior with compact desktop actions", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.seedTask(seedData.workspaceId, LONG_TASK_TITLE, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const survivor = await apiClient.seedTask(seedData.workspaceId, "Desktop delete survivor", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await testPage.goto(`/t/${task.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.taskInSidebar(LONG_TASK_TITLE)).toBeVisible();

    await session.openSidebarMenuAndClick(LONG_TASK_TITLE, "Rename");
    const renameDialog = testPage.locator('[data-slot="dialog-content"]:visible').last();
    await expect(renameDialog).toBeVisible();
    const renameTitle = renameDialog.locator('[data-slot="dialog-title"]');
    await expect(renameTitle).toBeVisible();
    await expect(renameTitle).toHaveCSS("text-wrap", "balance");
    const renameLabelledBy = await renameDialog.getAttribute("aria-labelledby");
    expect(renameLabelledBy).toBe(await renameTitle.getAttribute("id"));
    await assertLocatorWithinViewportX(renameDialog, "desktop rename Dialog");
    await renameDialog.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(renameDialog).toBeHidden();

    await session.openSidebarMenuAndClick(LONG_TASK_TITLE, "Delete");
    const deleteDialog = testPage.getByRole("alertdialog");
    await expect(deleteDialog).toBeVisible();
    await assertDesktopDeleteDialog(deleteDialog);

    await deleteDialog.locator('[data-slot="alert-dialog-cancel"]').click();
    await expect(deleteDialog).toBeHidden();
    expect((await apiClient.getTask(task.task_id)).id).toBe(task.task_id);

    await session.openSidebarMenuAndClick(LONG_TASK_TITLE, "Delete");
    await testPage.getByRole("alertdialog").locator('[data-slot="alert-dialog-action"]').click();
    await expect
      .poll(
        async () => (await apiClient.rawRequest("GET", `/api/v1/tasks/${task.task_id}`)).status,
        { timeout: 15_000, message: "the desktop Delete action should remove the task" },
      )
      .toBe(404);
    await expect(session.taskInSidebar(LONG_TASK_TITLE)).toHaveCount(0, { timeout: 15_000 });
    await expect(session.taskInSidebar("Desktop delete survivor")).toBeVisible({
      timeout: 15_000,
    });
    expect((await apiClient.getTask(survivor.task_id)).id).toBe(survivor.task_id);
  });
});
