import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { waitForHttp } from "../../helpers/causal-waits";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

const INLINE_CONFIRMATION_TEST_ID = "task-archive-inline-confirmation";
const TASK_TITLE = "Mobile Kanban archive confirmation";

async function openArchiveMenu(testPage: Page, taskId: string) {
  const mobile = new MobileKanbanPage(testPage);
  const card = mobile.taskCard(taskId);
  await card.getByRole("button", { name: "More options" }).tap();

  const menu = testPage.locator('[data-slot="dropdown-menu-content"]:visible').last();
  await expect(menu).toBeVisible();
  const archiveItem = menu.getByRole("menuitem", { name: "Archive", exact: true });
  await expect(archiveItem).toBeVisible();
  await archiveItem.tap();
}

test.describe("Mobile Kanban card archive confirmation", () => {
  test("uses a contained dark dialog and archives the card after confirmation", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await testPage.addInitScript(() => localStorage.setItem("theme", "dark"));
    const task = await apiClient.createTask(seedData.workspaceId, TASK_TITLE, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.createTask(seedData.workspaceId, "Mobile Kanban archive neighbor", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await expect(testPage.locator("html")).toHaveClass(/(^|\s)dark(\s|$)/);

    const card = mobile.taskCard(task.id);
    await expect(card).toBeVisible();
    const startUrl = testPage.url();
    const beforeBox = await card.boundingBox();
    if (!beforeBox) throw new Error("mobile archive task card has no layout box");

    await openArchiveMenu(testPage, task.id);

    const dialog = testPage.getByRole("alertdialog", { name: /Archive task/ });
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText(TASK_TITLE);
    await expect(dialog.getByTestId("task-confirmation-outcome")).toContainText(TASK_TITLE);
    await expect(dialog.getByRole("button", { name: "Cancel", exact: true })).toBeVisible();
    await expect(dialog.getByRole("button", { name: "Archive", exact: true })).toBeVisible();
    await expect(testPage.getByTestId(INLINE_CONFIRMATION_TEST_ID)).toHaveCount(0);

    await waitForFiniteAnimations(dialog);
    await prCapture.screenshot("after-dark", {
      caption:
        "After: mobile Kanban uses the contained archive alert dialog without changing card layout.",
    });
    const [dialogBox, viewport, backgroundColor] = await Promise.all([
      dialog.boundingBox(),
      testPage.evaluate(() => ({ width: window.innerWidth, height: window.innerHeight })),
      dialog.evaluate((element) => getComputedStyle(element).backgroundColor),
    ]);
    if (!dialogBox) throw new Error("mobile archive dialog has no layout box");
    expect(dialogBox.x).toBeGreaterThanOrEqual(12);
    expect(viewport.width - (dialogBox.x + dialogBox.width)).toBeGreaterThanOrEqual(12);
    expect(dialogBox.y).toBeGreaterThanOrEqual(0);
    expect(dialogBox.y + dialogBox.height).toBeLessThanOrEqual(viewport.height);
    expect(backgroundColor).not.toBe("rgba(0, 0, 0, 0)");
    expect(backgroundColor).toMatch(/^rgb/);

    for (const actionName of ["Cancel", "Archive"]) {
      const actionBox = await dialog
        .getByRole("button", { name: actionName, exact: true })
        .boundingBox();
      if (!actionBox) throw new Error(`${actionName} action has no layout box`);
      expect(actionBox.height).toBeGreaterThanOrEqual(44);
    }
    const afterOpenBox = await card.boundingBox();
    if (!afterOpenBox)
      throw new Error("mobile archive task card disappeared while dialog was open");
    expect(afterOpenBox.height).toBeCloseTo(beforeBox.height, 1);
    expect(testPage.url()).toBe(startUrl);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile Kanban archive dialog");

    await dialog.getByRole("button", { name: "Cancel", exact: true }).tap();
    await expect(dialog).not.toBeVisible();
    await expect(card).toBeVisible();
    expect(testPage.url()).toBe(startUrl);

    await openArchiveMenu(testPage, task.id);
    const reopenedDialog = testPage.getByRole("alertdialog", { name: /Archive task/ });
    await expect(reopenedDialog).toBeVisible();
    const archiveResponse = waitForHttp(
      testPage,
      "POST",
      new RegExp(`/api/v1/tasks/${task.id}/archive$`),
    );
    await reopenedDialog.getByRole("button", { name: "Archive", exact: true }).tap();
    await archiveResponse;
    await expect(card).not.toBeVisible();
  });

  test("bypasses the dialog when archive confirmation is disabled", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Mobile archive bypass", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.saveUserSettings({ confirm_task_archive: false });

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    const card = mobile.taskCard(task.id);
    await expect(card).toBeVisible();

    const archiveResponse = waitForHttp(
      testPage,
      "POST",
      new RegExp(`/api/v1/tasks/${task.id}/archive$`),
    );
    await openArchiveMenu(testPage, task.id);
    await archiveResponse;
    await expect(card).not.toBeVisible();
    await expect(testPage.getByRole("alertdialog")).toHaveCount(0);
    await expect(testPage.getByTestId(INLINE_CONFIRMATION_TEST_ID)).toHaveCount(0);
  });
});
