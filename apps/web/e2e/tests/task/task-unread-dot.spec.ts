import { expect, type Locator } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { dwell, watchWs } from "../../helpers/causal-waits";
import {
  FAILED_SESSION_ERROR,
  TASK_UNREAD_DOT_LABEL,
  type UnreadSettings,
  captureUnreadSettings,
  observeReadTraffic,
  rowHeight,
  seedFailedSiblingTask,
  seedUnreadDotTasks,
  sessionReadState,
  taskUnreadSessionIds,
  waitForTaskUnreadSummary,
} from "../../helpers/unread-divider";

function unreadDot(row: Locator): Locator {
  return row.getByRole("img", { name: TASK_UNREAD_DOT_LABEL });
}

test.describe("Task unread dot", () => {
  let baselineSettings: UnreadSettings = {};

  test.beforeEach(async ({ apiClient }) => {
    baselineSettings = await captureUnreadSettings(apiClient);
    await apiClient.saveUserSettings({
      unread_divider: false,
      unread_task_dot: true,
      unread_task_dot_hide_delay_seconds: 3,
    });
  });

  test.afterEach(async ({ apiClient }) => {
    await apiClient.saveUserSettings(baselineSettings);
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

  // @covers AC-TASKS-TASK-UNREAD-DOT-001.6
  // @covers AC-TASKS-TASK-UNREAD-DOT-001.9
  // @covers AC-TASKS-TASK-UNREAD-DOT-001.12
  // @covers AC-TASKS-TASK-UNREAD-DOT-001.17
  test("clears a cursorless failed sibling only after its transcript is read", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(150_000);
    await apiClient.saveUserSettings({
      unread_task_dot_hide_delay_seconds: 0,
      agent_tab_close_behavior: "hide_panel",
    });
    const seeded = await seedFailedSiblingTask(apiClient, seedData, "Failed sibling");
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
    const taskRow = session.sidebarTaskItem(seeded.title);
    await expect(taskRow).toBeVisible({ timeout: 15_000 });
    // Viewing the primary does not read its siblings.
    await expect(unreadDot(taskRow)).toBeVisible({ timeout: 15_000 });
    // Hide the unread failed tab without activating or acknowledging it.
    await expect(session.sessionTabCloseButton(seeded.failedSessionId)).toHaveAccessibleName(
      "Hide panel",
    );
    await session.sessionTabCloseButton(seeded.failedSessionId).click();
    await expect(session.sessionTabBySessionId(seeded.failedSessionId)).toHaveCount(0);
    await expect(session.activeChat()).toHaveAttribute("data-session-id", seeded.primarySessionId);
    expect((await failedState()).cursor).toBeNull();
    expect(await unreadIds()).toEqual([seeded.failedSessionId, seeded.liveSessionId].sort());
    await testPage.reload();
    await session.waitForLoad();
    // These agent-less seeds have no environment, so hide state is visit-local.
    await expect(session.sessionTabBySessionId(seeded.failedSessionId)).toBeVisible();
    expect((await failedState()).cursor).toBeNull();
    expect(await unreadIds()).toEqual([seeded.failedSessionId, seeded.liveSessionId].sort());
    await session.sessionTabCloseButton(seeded.failedSessionId).click();
    await expect(session.sessionTabBySessionId(seeded.failedSessionId)).toHaveCount(0);
    await expect(session.activeChat()).toHaveAttribute("data-session-id", seeded.primarySessionId);
    await dwell(testPage, 1_500, "negative-assertion", "primary visit must not read siblings");
    expect((await failedState()).cursor).toBeNull();
    expect(await unreadIds()).toEqual([seeded.failedSessionId, seeded.liveSessionId].sort());

    await session.addPanelButton().click();
    await session.sessionReopenItem(seeded.failedSessionId).click();
    await expect(session.sessionTabBySessionId(seeded.failedSessionId)).toBeVisible();
    await expect(session.activeChat()).toHaveAttribute("data-session-id", seeded.failedSessionId);
    await expect(session.activeChat().getByText(FAILED_SESSION_ERROR)).toBeVisible();
    await expect
      .poll(async () => (await failedState()).cursor, { timeout: 15_000 })
      .toBe(seeded.failedTailMessageId);
    await expect.poll(unreadIds, { timeout: 15_000 }).toEqual([seeded.liveSessionId]);
    // The live sibling keeps its contribution, so the row keeps its dot.
    await expect(unreadDot(taskRow)).toBeVisible();
    expect((await sessionReadState(apiClient, seeded.liveSessionId)).cursor).toBeNull();
    expect((await sessionReadState(apiClient, seeded.primarySessionId)).cursor).toBe(
      seeded.primaryCursor,
    );
    expect(traffic.markReads.map((read) => read.sessionId)).not.toContain(seeded.liveSessionId);
    expect(traffic.markReads).toContainEqual({
      sessionId: seeded.failedSessionId,
      messageId: seeded.failedTailMessageId,
    });
    // Reading stored output never starts the failed session's agent.
    expect(traffic.sentFrameCount()).toBeGreaterThan(0);
    expect(traffic.agentStarts()).toEqual([]);
    expect(await failedState()).toEqual({
      cursor: seeded.failedTailMessageId,
      state: "FAILED",
      executionId: null,
    });

    const navRow = session.sidebarTaskItem(seeded.navTitle);
    await navRow.click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${seeded.navTaskId}`));
    await testPage.reload();
    await session.waitForLoad();
    await expect(unreadDot(taskRow)).toBeVisible({ timeout: 15_000 });
    expect(await unreadIds()).toEqual([seeded.liveSessionId]);
    expect((await failedState()).cursor).toBe(seeded.failedTailMessageId);

    // Selecting the remaining sibling from the session menu clears the task.
    await taskRow.click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${seeded.taskId}`));
    await session.waitForLoad();
    await session.addPanelButton().click();
    await session.sessionReopenItem(seeded.liveSessionId).click();
    await expect(session.activeChat()).toHaveAttribute("data-session-id", seeded.liveSessionId);
    await expect.poll(unreadIds, { timeout: 15_000 }).toEqual([]);
    await expect
      .poll(async () => (await sessionReadState(apiClient, seeded.liveSessionId)).cursor)
      .toBe(seeded.liveTailMessageId);
    await navRow.click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${seeded.navTaskId}`));
    await testPage.reload();
    await session.waitForLoad();
    await expect(taskRow).toBeVisible({ timeout: 15_000 });
    await expect(unreadDot(taskRow)).toHaveCount(0);
    expect((await sessionReadState(apiClient, seeded.liveSessionId)).cursor).toBe(
      seeded.liveTailMessageId,
    );
    expect(traffic.agentStarts()).toEqual([]);
    expect((await failedState()).state).toBe("FAILED");
  });
});
