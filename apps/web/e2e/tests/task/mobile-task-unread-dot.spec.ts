import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { watchWs } from "../../helpers/causal-waits";
import {
  TASK_UNREAD_DOT_LABEL,
  rowHeight,
  seedUnreadDotTasks,
  waitForTaskUnreadSummary,
} from "../../helpers/unread-divider";

test.describe("Mobile task unread dot", () => {
  test.beforeEach(async ({ apiClient }) => {
    await apiClient.saveUserSettings({
      unread_divider: false,
      unread_task_dot: true,
      unread_task_dot_hide_delay_seconds: 0,
    });
  });

  // @covers AC-TASKS-TASK-UNREAD-DOT-001.1
  // @covers AC-TASKS-TASK-UNREAD-DOT-001.15
  // @covers AC-TASKS-TASK-UNREAD-DOT-001.16
  test("shows the dot in the task switcher without changing the row", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const seeded = await seedUnreadDotTasks(apiClient, seedData, "Mobile unread dot");

    const ws = watchWs(testPage);
    await testPage.goto(`/t/${seeded.navTaskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await testPage.getByTestId("mobile-task-picker-trigger").click();
    const sheet = testPage.getByRole("dialog");
    const targetRow = sheet
      .getByTestId("sidebar-task-item")
      .filter({ hasText: seeded.targetTitle });
    await expect(targetRow).toBeVisible({ timeout: 15_000 });
    const dot = targetRow.getByRole("img", { name: TASK_UNREAD_DOT_LABEL });
    await expect(dot).toHaveCount(0);
    const heightWithoutDot = await rowHeight(targetRow);

    const targetUnread = waitForTaskUnreadSummary(ws, seeded.targetTaskId);
    await apiClient.seedSessionMessage(seeded.targetSessionId, {
      type: "message",
      content: "background reply",
    });

    await targetUnread;
    await expect(dot).toBeVisible();
    expect(await rowHeight(targetRow)).toBe(heightWithoutDot);
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);

    // The dot is passive: tapping the row still opens the task.
    await targetRow.tap();
    await expect(testPage).toHaveURL(new RegExp(`/t/${seeded.targetTaskId}`));
  });
});
