import { expect, type Locator, type Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { dwell, watchWs } from "../../helpers/causal-waits";
import {
  FAILED_SESSION_ERROR,
  TASK_UNREAD_DOT_LABEL,
  type UnreadSettings,
  captureUnreadSettings,
  cleanupFailingResumeTask,
  newestStoredMessageId,
  observeReadTraffic,
  rowHeight,
  seedFailedSiblingTask,
  seedFailingResumeTask,
  seedUnreadDotTasks,
  sessionReadState,
  taskUnreadSessionIds,
  waitForTaskUnreadSummary,
} from "../../helpers/unread-divider";

/** Opens the phone task switcher and returns the row for `title`. */
async function openTaskSwitcherRow(page: Page, title: string): Promise<Locator> {
  await page.getByTestId("mobile-task-picker-trigger").tap();
  const row = page.getByRole("dialog").getByTestId("sidebar-task-item").filter({ hasText: title });
  await expect(row).toBeVisible({ timeout: 15_000 });
  return row;
}

async function tapSessionRow(page: Page, sessionId: string): Promise<void> {
  await page.getByTestId("mobile-task-layout").getByTestId("mobile-sessions-pill").tap();
  await page.getByTestId(`mobile-session-row-${sessionId}`).tap();
}

test.describe("Mobile task unread dot", () => {
  let baselineSettings: UnreadSettings = {};

  test.beforeEach(async ({ apiClient }) => {
    baselineSettings = await captureUnreadSettings(apiClient);
    await apiClient.saveUserSettings({
      unread_divider: false,
      unread_task_dot: true,
      unread_task_dot_hide_delay_seconds: 0,
    });
  });

  test.afterEach(async ({ apiClient }) => {
    await apiClient.saveUserSettings(baselineSettings);
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

  // @covers AC-TASKS-TASK-UNREAD-DOT-001.6
  // @covers AC-TASKS-TASK-UNREAD-DOT-001.9
  // @covers AC-TASKS-TASK-UNREAD-DOT-001.15
  // @covers AC-TASKS-TASK-UNREAD-DOT-001.17
  test("clears a cursorless failed sibling only after its transcript is read", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(150_000);
    const seeded = await seedFailedSiblingTask(apiClient, seedData, "Mobile failed sibling");
    const unreadIds = () => taskUnreadSessionIds(apiClient, seedData.workspaceId, seeded.taskId);
    const failedState = () => sessionReadState(apiClient, seeded.failedSessionId);
    await expect
      .poll(unreadIds, { timeout: 15_000 })
      .toEqual([seeded.failedSessionId, seeded.liveSessionId].sort());

    const traffic = observeReadTraffic(testPage);
    await testPage.goto(`/t/${seeded.taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.activeChat()).toHaveAttribute("data-session-id", seeded.primarySessionId);
    await dwell(testPage, 1_500, "negative-assertion", "primary visit must not read siblings");
    expect((await failedState()).cursor).toBeNull();
    expect(await unreadIds()).toEqual([seeded.failedSessionId, seeded.liveSessionId].sort());

    await tapSessionRow(testPage, seeded.failedSessionId);
    await expect(session.activeChat()).toHaveAttribute("data-session-id", seeded.failedSessionId);
    await expect(session.activeChat().getByText(FAILED_SESSION_ERROR)).toBeVisible();
    await expect
      .poll(async () => (await failedState()).cursor, { timeout: 15_000 })
      .toBe(seeded.failedTailMessageId);
    await expect.poll(unreadIds, { timeout: 15_000 }).toEqual([seeded.liveSessionId]);
    expect((await sessionReadState(apiClient, seeded.liveSessionId)).cursor).toBeNull();
    expect(traffic.markReads.map((read) => read.sessionId)).not.toContain(seeded.liveSessionId);
    expect(traffic.sentFrameCount()).toBeGreaterThan(0);
    expect(traffic.agentStarts()).toEqual([]);
    expect(await failedState()).toEqual({
      cursor: seeded.failedTailMessageId,
      state: "FAILED",
      executionId: null,
    });
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
    await testPage.screenshot({ path: test.info().outputPath("phone-failed-session.png") });

    // The unread live sibling keeps the dot after leaving and reloading.
    await (await openTaskSwitcherRow(testPage, seeded.navTitle)).tap();
    await expect(testPage).toHaveURL(new RegExp(`/t/${seeded.navTaskId}`));
    await testPage.reload();
    await session.waitForLoad();
    const taskRow = await openTaskSwitcherRow(testPage, seeded.title);
    await expect(taskRow.getByRole("img", { name: TASK_UNREAD_DOT_LABEL })).toBeVisible();
    expect(await unreadIds()).toEqual([seeded.liveSessionId]);
    expect((await failedState()).cursor).toBe(seeded.failedTailMessageId);

    // Reading the remaining sibling clears the task durably.
    await taskRow.tap();
    await expect(testPage).toHaveURL(new RegExp(`/t/${seeded.taskId}`));
    await session.waitForLoad();
    await tapSessionRow(testPage, seeded.liveSessionId);
    await expect(session.activeChat()).toHaveAttribute("data-session-id", seeded.liveSessionId);
    await expect.poll(unreadIds, { timeout: 15_000 }).toEqual([]);
    await expect
      .poll(async () => (await sessionReadState(apiClient, seeded.liveSessionId)).cursor)
      .toBe(seeded.liveTailMessageId);
    await (await openTaskSwitcherRow(testPage, seeded.navTitle)).tap();
    await expect(testPage).toHaveURL(new RegExp(`/t/${seeded.navTaskId}`));
    await testPage.reload();
    await session.waitForLoad();
    const clearedRow = await openTaskSwitcherRow(testPage, seeded.title);
    await expect(clearedRow.getByRole("img", { name: TASK_UNREAD_DOT_LABEL })).toHaveCount(0);
    expect((await sessionReadState(apiClient, seeded.liveSessionId)).cursor).toBe(
      seeded.liveTailMessageId,
    );
    expect(traffic.agentStarts()).toEqual([]);
    expect((await failedState()).state).toBe("FAILED");
  });

  // @covers AC-TASKS-TASK-UNREAD-DOT-001.6
  // @covers AC-TASKS-TASK-UNREAD-DOT-001.15
  test("keeps a session whose resume fails on screen until its failure is read", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(240_000);
    const seeded = await seedFailingResumeTask(
      apiClient,
      seedData,
      backend.tmpDir,
      "Mobile failed resume",
    );
    try {
      const unreadIds = () => taskUnreadSessionIds(apiClient, seedData.workspaceId, seeded.taskId);
      const failingState = () => sessionReadState(apiClient, seeded.failingSessionId);
      const traffic = observeReadTraffic(testPage);
      await testPage.goto(`/t/${seeded.taskId}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await expect
        .poll(async () => (await failingState()).state, { timeout: 60_000 })
        .toBe("FAILED");
      await dwell(testPage, 1_500, "negative-assertion", "a failed resume must keep focus");
      await expect(session.activeChat()).toHaveAttribute(
        "data-session-id",
        seeded.failingSessionId,
      );
      const failureTail = await newestStoredMessageId(apiClient, seeded.failingSessionId);
      await expect
        .poll(async () => (await failingState()).cursor, { timeout: 15_000 })
        .toBe(failureTail);
      await expect.poll(unreadIds, { timeout: 15_000 }).toEqual([]);
      expect(traffic.markReads.map((read) => read.sessionId)).not.toContain(
        seeded.siblingSessionId,
      );
      expect(
        await testPage.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      ).toBe(true);
      await testPage.screenshot({ path: test.info().outputPath("phone-failed-resume.png") });

      await (await openTaskSwitcherRow(testPage, seeded.navTitle)).tap();
      await expect(testPage).toHaveURL(new RegExp(`/t/${seeded.navTaskId}`));
      await testPage.reload();
      await session.waitForLoad();
      const taskRow = await openTaskSwitcherRow(testPage, seeded.title);
      await expect(taskRow.getByRole("img", { name: TASK_UNREAD_DOT_LABEL })).toHaveCount(0);
      expect((await failingState()).cursor).toBe(failureTail);
    } finally {
      await cleanupFailingResumeTask(apiClient, seeded);
    }
  });
});
