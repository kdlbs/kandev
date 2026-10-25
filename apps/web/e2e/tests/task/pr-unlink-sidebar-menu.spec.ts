import { test, expect } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { SessionPage } from "../../pages/session-page";
import {
  SIDEBAR_PR_UNLINK_LABEL,
  SIDEBAR_PR_UNLINK_TITLE,
  seedSidebarTaskWithPR,
} from "./task-pr-unlink-sidebar-fixtures";

test.describe("Sidebar task PR unlink menu", () => {
  test("unlinks the selected association from the row Edit submenu", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(90_000);
    const { navigationTask, targetTask } = await seedSidebarTaskWithPR(
      apiClient,
      seedData,
      "Sidebar PR unlink navigation",
    );
    await testPage.goto(`/t/${navigationTask.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const targetRow = session.sidebarTaskItem(SIDEBAR_PR_UNLINK_TITLE);
    await expect(targetRow).toBeVisible({ timeout: 15_000 });
    const beforeUrl = testPage.url();

    await session.openSidebarTaskContextMenu(SIDEBAR_PR_UNLINK_TITLE);
    const edit = testPage.getByTestId("task-row-edit-submenu");
    await expect(edit).toBeVisible();
    await edit.focus();
    await testPage.keyboard.press("ArrowRight");
    const unlinkChoice = testPage.getByRole("menuitem", { name: SIDEBAR_PR_UNLINK_LABEL });
    await expect(unlinkChoice).toBeVisible();
    await waitForFiniteAnimations(testPage.getByRole("menu").last());
    await prCapture.screenshot("desktop-sidebar-task-pr-unlink-menu", {
      caption: "Desktop sidebar task PR unlink menu",
    });
    await unlinkChoice.click();

    await expect.poll(() => apiClient.listTaskPRs(targetTask.task_id)).toEqual([]);
    await expect(targetRow).toBeVisible();
    await expect(targetRow.getByTestId(`pr-task-icon-${targetTask.task_id}`)).toBeHidden();
    expect(testPage.url()).toBe(beforeUrl);

    await testPage.reload();
    await session.waitForLoad();
    const reloadedRow = session.sidebarTaskItem(SIDEBAR_PR_UNLINK_TITLE);
    await expect(reloadedRow).toBeVisible({ timeout: 15_000 });
    await expect(reloadedRow.getByTestId(`pr-task-icon-${targetTask.task_id}`)).toHaveCount(0);
    await expect.poll(() => apiClient.listTaskPRs(targetTask.task_id)).toEqual([]);
  });
});
