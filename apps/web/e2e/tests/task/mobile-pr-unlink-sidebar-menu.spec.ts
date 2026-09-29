import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { SessionPage } from "../../pages/session-page";
import {
  SIDEBAR_PR_UNLINK_LABEL,
  SIDEBAR_PR_UNLINK_TITLE,
  seedSidebarTaskWithPR,
} from "./task-pr-unlink-sidebar-fixtures";

test.describe("Mobile sidebar task PR unlink menu", () => {
  test("unlinks the selected association from the task row actions", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(90_000);
    await testPage.setViewportSize({ width: 393, height: 852 });
    const { navigationTask, targetTask } = await seedSidebarTaskWithPR(
      apiClient,
      seedData,
      "Mobile sidebar PR unlink navigation",
    );
    await testPage.goto(`/t/${navigationTask.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const picker = testPage.getByTestId("mobile-task-switcher-list");
    const row = picker
      .getByTestId("sidebar-task-item")
      .filter({ hasText: SIDEBAR_PR_UNLINK_TITLE });
    const beforeUrl = testPage.url();

    await testPage.getByTestId("mobile-task-picker-trigger").tap();
    await expect(row).toBeVisible({ timeout: 15_000 });
    const taskActions = row.locator("button.mobile-task-actions-button");
    const taskActionsBounds = await taskActions.boundingBox();
    expect(taskActionsBounds).not.toBeNull();
    expect(taskActionsBounds!.width).toBeGreaterThanOrEqual(44);
    expect(taskActionsBounds!.height).toBeGreaterThanOrEqual(44);
    await taskActions.tap();
    await testPage.getByRole("menuitem", { name: "Edit", exact: true }).tap();
    const edit = testPage.getByTestId("task-row-edit-submenu");
    await edit.tap();
    await waitForFiniteAnimations(testPage.getByRole("menu").last());

    const unlinkChoice = testPage.getByRole("menuitem", { name: SIDEBAR_PR_UNLINK_LABEL });
    await expect(unlinkChoice).toBeVisible();
    await waitForFiniteAnimations(testPage.getByRole("menu").last());
    await prCapture.screenshot("mobile-sidebar-task-pr-unlink-menu", {
      caption: "Mobile sidebar task PR unlink menu",
    });
    const choiceBounds = await unlinkChoice.boundingBox();
    expect(choiceBounds).not.toBeNull();
    expect(choiceBounds!.height).toBeGreaterThanOrEqual(44);
    const viewport = testPage.viewportSize();
    expect(viewport).not.toBeNull();
    expect(choiceBounds!.x).toBeGreaterThanOrEqual(0);
    expect(choiceBounds!.x + choiceBounds!.width).toBeLessThanOrEqual(viewport!.width + 1);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile sidebar unlink menu");
    await unlinkChoice.tap();

    await expect.poll(() => apiClient.listTaskPRs(targetTask.task_id)).toEqual([]);
    await expect(row).toBeVisible();
    expect(testPage.url()).toBe(beforeUrl);

    await testPage.reload();
    await session.waitForLoad();
    await testPage.getByTestId("mobile-task-picker-trigger").tap();
    await expect(
      testPage
        .getByTestId("mobile-task-switcher-list")
        .getByTestId("sidebar-task-item")
        .filter({ hasText: SIDEBAR_PR_UNLINK_TITLE }),
    ).toBeVisible({ timeout: 15_000 });
    await expect.poll(() => apiClient.listTaskPRs(targetTask.task_id)).toEqual([]);
  });
});
