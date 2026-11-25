import { expect, type Locator } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { watchWs } from "../../helpers/causal-waits";
import {
  TASK_UNREAD_DOT_LABEL,
  rowHeight,
  seedUnreadDotTasks,
  waitForTaskUnreadSummary,
} from "../../helpers/unread-divider";

function unreadDot(row: Locator): Locator {
  return row.getByRole("img", { name: TASK_UNREAD_DOT_LABEL });
}

test.describe("Task unread dot", () => {
  test.beforeEach(async ({ apiClient }) => {
    await apiClient.saveUserSettings({
      unread_divider: false,
      unread_task_dot: true,
      unread_task_dot_hide_delay_seconds: 3,
    });
  });

  // @covers AC-TASKS-TASK-UNREAD-DOT-001.1
  // @covers AC-TASKS-TASK-UNREAD-DOT-001.2
  // @covers AC-TASKS-TASK-UNREAD-DOT-001.3
  // @covers AC-TASKS-TASK-UNREAD-DOT-001.4
  // @covers AC-TASKS-TASK-UNREAD-DOT-001.5
  // @covers AC-TASKS-TASK-UNREAD-DOT-001.6
  // @covers AC-TASKS-TASK-UNREAD-DOT-001.7
  // @covers AC-TASKS-TASK-UNREAD-DOT-001.8
  // @covers AC-TASKS-TASK-UNREAD-DOT-001.13
  // @covers AC-TASKS-TASK-UNREAD-DOT-001.14
  // @covers AC-TASKS-TASK-UNREAD-DOT-001.18
  // @covers AC-TASKS-TASK-UNREAD-DOT-001.16
  test("marks background tasks with new messages until they are read", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const seeded = await seedUnreadDotTasks(apiClient, seedData, "Unread dot");

    const ws = watchWs(testPage);
    await testPage.goto(`/t/${seeded.navTaskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const navRow = session.sidebarTaskItem(seeded.navTitle);
    const targetRow = session.sidebarTaskItem(seeded.targetTitle);
    const cursorlessRow = session.sidebarTaskItem(seeded.cursorlessTitle);
    await expect(targetRow).toBeVisible({ timeout: 15_000 });
    await expect(cursorlessRow).toBeVisible();
    await expect(unreadDot(targetRow)).toHaveCount(0);
    // A never-opened session with agent output is unread on its first load.
    await expect(unreadDot(cursorlessRow)).toBeVisible({ timeout: 15_000 });
    const heightWithoutDot = await rowHeight(targetRow);

    // The user's own message and messages for the open task stay clean.
    await apiClient.seedSessionMessage(seeded.targetSessionId, {
      type: "message",
      content: "my own follow-up",
      authorType: "user",
    });
    await apiClient.seedSessionMessage(seeded.navSessionId, {
      type: "message",
      content: "arrives while open",
    });
    await expect(unreadDot(targetRow)).toHaveCount(0);
    const targetUnread = waitForTaskUnreadSummary(ws, seeded.targetTaskId);
    const unreadReply = await apiClient.seedSessionMessage(seeded.targetSessionId, {
      type: "message",
      content: "background reply",
    });

    await targetUnread;
    await expect(unreadDot(targetRow)).toBeVisible();
    expect(await rowHeight(targetRow)).toBe(heightWithoutDot);
    await expect(unreadDot(navRow)).toHaveCount(0);

    await testPage.reload();
    await session.waitForLoad();
    await expect(unreadDot(targetRow)).toBeVisible({ timeout: 15_000 });

    await apiClient.saveUserSettings({ unread_task_dot: false });
    await testPage.reload();
    await session.waitForLoad();
    await expect(targetRow).toBeVisible({ timeout: 15_000 });
    await expect(unreadDot(targetRow)).toHaveCount(0);

    await apiClient.saveUserSettings({ unread_task_dot: true });
    await testPage.reload();
    await session.waitForLoad();
    await expect(unreadDot(targetRow)).toBeVisible({ timeout: 15_000 });

    await targetRow.click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${seeded.targetTaskId}`));
    // The opened task keeps its dot for the configured hide delay, then clears it.
    await expect(unreadDot(targetRow)).toBeVisible();
    await expect(unreadDot(targetRow)).toHaveCount(0, { timeout: 10_000 });
    await expect
      .poll(
        async () =>
          (await apiClient.getTaskSession(seeded.targetSessionId)).session.last_read_message_id,
        { timeout: 15_000 },
      )
      .toBe(unreadReply.messageId);

    await navRow.click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${seeded.navTaskId}`));
    await expect(targetRow).toBeVisible();
    await expect(unreadDot(targetRow)).toHaveCount(0);
  });
});
