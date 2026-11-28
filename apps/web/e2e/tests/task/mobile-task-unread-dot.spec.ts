import { randomUUID } from "node:crypto";
import { expect, type Locator, type Page } from "@playwright/test";
import type { Route } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { dwell, watchWs } from "../../helpers/causal-waits";
import {
  expectLastStoppedSessionRemoval,
  expectLatePromotionAfterRemoval,
  seedStoppedRemovalTask,
  holdRemovalMembership,
  observeRemovalSelection,
  seedRemovalChoiceTask,
} from "../../helpers/session-removal";
import {
  FAILED_SESSION_ERROR,
  TASK_UNREAD_DOT_LABEL,
  type UnreadSettings,
  captureUnreadSettings,
  cleanupFailingResumeTask,
  failedResumeTail,
  waitForFailedResume,
  observeReadTraffic,
  rowHeight,
  seedFailedSiblingTask,
  seedFailingResumeTask,
  seedUnreadDotTasks,
  sessionReadState,
  taskUnreadSessionIds,
  waitForTaskUnreadSummary,
} from "../../helpers/unread-divider";
import { waitForActiveTaskSession, waitForTaskSessionsSettled } from "../../helpers/session-store";
import { expectConversationRouteAfterRemoval } from "../../helpers/session-route-admission";

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

  // @covers AC-TASKS-TASK-UNREAD-DOT-001.6, AC-TASKS-TASK-UNREAD-DOT-001.17
  test("opens an explicit conversation after another client removes the selected session", async ({
    testPage,
    desktopTestPage,
    apiClient,
    seedData,
  }) => {
    const { settings } = await apiClient.getUserSettings();
    await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: true });
    try {
      await expectConversationRouteAfterRemoval(testPage, desktopTestPage, apiClient, seedData, {
        rememberPlan: true,
      });
      await testPage.screenshot({
        path: test.info().outputPath("phone-explicit-route-after-removal.png"),
      });
    } finally {
      await apiClient.saveUserSettings({
        prevent_auto_start_agent_on_open: settings.prevent_auto_start_agent_on_open === true,
        enable_preview_on_click: settings.enable_preview_on_click === true,
      });
    }
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
      expect((await failingState()).cursor).toBe(seeded.failingCursor);
      expect(
        (await apiClient.listTaskSessions(seeded.taskId)).sessions.find((item) => item.is_primary)
          ?.id,
      ).toBe(seeded.failingSessionId);
      const traffic = observeReadTraffic(testPage);
      const settled = waitForFailedResume(watchWs(testPage), seeded);
      await testPage.goto(`/t/${seeded.taskId}`);
      const session = new SessionPage(testPage);
      await expect(session.activeChat()).toBeVisible({ timeout: 30_000 });
      await settled;
      expect(
        (await apiClient.listTaskSessions(seeded.taskId)).sessions.find((item) => item.is_primary)
          ?.id,
      ).toBe(seeded.siblingSessionId);
      await expect
        .poll(async () => (await failingState()).state, { timeout: 60_000 })
        .toBe("FAILED");
      await dwell(testPage, 1_500, "negative-assertion", "a failed resume must keep focus");
      await expect(session.activeChat()).toHaveAttribute(
        "data-session-id",
        seeded.failingSessionId,
      );
      const failureTail = await failedResumeTail(apiClient, seeded);
      await expect(session.activeChat().getByTestId("session-recovery-card")).toBeVisible();
      await expect(
        session
          .activeChat()
          .getByTestId("session-recovery-card")
          .getByRole("button", { name: "Start fresh session", exact: true }),
      ).toBeVisible();
      await expect(
        session.activeChat().getByText("Failed to resume agent Mock", { exact: false }).first(),
      ).toBeVisible();
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

  // @covers AC-TASKS-TASK-UNREAD-DOT-001.6, AC-TASKS-TASK-UNREAD-DOT-001.11
  test("keeps the phone route sibling unread during an owned deletion gap", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { settings } = await apiClient.getUserSettings();
    const preventAutoStart = settings.prevent_auto_start_agent_on_open === true;
    await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: true });
    const { task, old, primary, other } = await seedRemovalChoiceTask(apiClient, seedData);
    await apiClient.setPrimarySession(other.session_id);
    const watcher = watchWs(testPage);
    const traffic = observeReadTraffic(testPage);
    const proof = await holdRemovalMembership(testPage, task.task_id, old.session_id);
    const session = new SessionPage(testPage);
    let history: Awaited<ReturnType<typeof observeRemovalSelection>> | undefined;
    try {
      await testPage.goto(`/t/${task.task_id}`);
      await expect(session.activeChat()).toHaveAttribute("data-session-id", other.session_id);
      await expect
        .poll(async () => (await sessionReadState(apiClient, other.session_id)).cursor)
        .not.toBeNull();
      await tapSessionRow(testPage, old.session_id);
      await expect(session.activeChat()).toHaveAttribute("data-session-id", old.session_id);
      const promoted = watcher.waitForEvent("task.updated", {
        where: (payload) =>
          payload.task_id === task.task_id && payload.primary_session_id === primary.session_id,
      });
      await apiClient.setPrimarySession(primary.session_id);
      await promoted;
      const before = (await sessionReadState(apiClient, other.session_id)).cursor;
      const tail = await apiClient.seedSessionMessage(other.session_id, {
        type: "message",
        content: "Unread route sibling while replacement membership is pending",
      });
      history = await observeRemovalSelection(testPage);
      const documentStart = await testPage.evaluate(() => performance.timeOrigin);
      proof.arm();
      await testPage.getByTestId("mobile-sessions-pill").tap();
      const picker = testPage.getByRole("dialog", { name: "Sessions", exact: true });
      await picker
        .getByTestId(`mobile-session-row-${old.session_id}`)
        .getByRole("button", { name: "Session actions", exact: true })
        .tap();
      await testPage.getByRole("menuitem", { name: "Delete", exact: true }).tap();
      const acknowledged = watcher.waitForResponse("session.delete");
      await testPage.getByTestId("mobile-session-delete-confirm").tap();
      await acknowledged;
      await proof.queued;
      await waitForActiveTaskSession(testPage, task.task_id, null);
      await testPage.keyboard.press("Escape");
      await expect(picker).toBeHidden();
      await dwell(
        testPage,
        1500,
        "negative-assertion",
        "The route sibling cannot become readable before deletion membership proves its recipient",
      );
      expect((await sessionReadState(apiClient, other.session_id)).cursor).toBe(before);
      expect(
        traffic.markReads.filter(
          (read) => read.sessionId === other.session_id && read.messageId === tail.messageId,
        ),
      ).toEqual([]);
      await testPage.screenshot({ path: test.info().outputPath("phone-owned-removal-gap.png") });
      expect((await history.read()).at(-1)?.sessionId).toBeNull();
      await expect(
        testPage.locator('[data-testid="session-chat"][data-session-id]:visible'),
      ).toHaveCount(0);
      expect((await history.read()).map((sample) => sample.sessionId)).not.toContain(
        other.session_id,
      );
      proof.release();
      await waitForActiveTaskSession(testPage, task.task_id, primary.session_id);
      await waitForTaskSessionsSettled(testPage, task.task_id);
      await expect(session.activeChat()).toHaveAttribute("data-session-id", primary.session_id);
      expect((await sessionReadState(apiClient, other.session_id)).cursor).toBe(before);
      await tapSessionRow(testPage, other.session_id);
      await expect(session.activeChat()).toHaveAttribute("data-session-id", other.session_id);
      await expect
        .poll(async () => (await sessionReadState(apiClient, other.session_id)).cursor)
        .toBe(tail.messageId);
      expect(await testPage.evaluate(() => performance.timeOrigin)).toBe(documentStart);
    } finally {
      await proof.dispose();
      await history?.stop();
      await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: preventAutoStart });
    }
  });

  // @covers AC-TASKS-TASK-UNREAD-DOT-001.6, AC-TASKS-TASK-UNREAD-DOT-001.11
  test("keeps the phone route unread when a workflow move fails after remote retirement", async ({
    testPage,
    desktopTestPage,
    apiClient,
    seedData,
  }) => {
    const { settings } = await apiClient.getUserSettings();
    const preventAutoStart = settings.prevent_auto_start_agent_on_open === true;
    await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: true });
    const { task, old, primary, other } = await seedRemovalChoiceTask(apiClient, seedData);
    const targetStep = seedData.steps.find((step) => step.id !== seedData.startStepId);
    if (!targetStep) throw new Error("Workflow cancellation requires a destination step");
    await apiClient.setPrimarySession(other.session_id);
    const proof = await holdRemovalMembership(testPage, task.task_id, old.session_id);
    const movePattern = `**/api/v1/tasks/${task.task_id}/move`;
    const moveStarted = Promise.withResolvers<void>();
    const moveRelease = Promise.withResolvers<void>();
    let moving: Promise<void> | undefined;
    const failMove = (route: Route) => {
      moving = (async () => {
        moveStarted.resolve();
        await moveRelease.promise;
        await route.fulfill({ status: 500, json: { error: "Injected workflow move failure" } });
      })();
      return moving;
    };
    await testPage.route(movePattern, failMove);
    const watcher = watchWs(testPage);
    const traffic = observeReadTraffic(testPage);
    const session = new SessionPage(testPage);
    try {
      await testPage.goto(`/t/${task.task_id}`);
      await expect(session.activeChat()).toHaveAttribute("data-session-id", other.session_id);
      await expect
        .poll(async () => (await sessionReadState(apiClient, other.session_id)).cursor)
        .not.toBeNull();
      await tapSessionRow(testPage, old.session_id);
      const promoted = watcher.waitForEvent("task.updated", {
        where: (payload) =>
          payload.task_id === task.task_id && payload.primary_session_id === primary.session_id,
      });
      await apiClient.setPrimarySession(primary.session_id);
      await promoted;
      const before = (await sessionReadState(apiClient, other.session_id)).cursor;
      const tail = await apiClient.seedSessionMessage(other.session_id, {
        type: "message",
        content: "Unread route sibling after independent workflow failure",
      });
      await testPage.getByTestId("mobile-task-picker-trigger").tap();
      const picker = testPage.getByRole("dialog", { name: "Tasks", exact: true });
      await picker
        .getByTestId("sidebar-task-item")
        .filter({ hasText: task.title })
        .getByRole("button", { name: "Task actions", exact: true })
        .tap();
      await testPage.getByTestId("task-context-move-to").tap();
      await testPage.getByTestId(`task-context-step-${targetStep.id}`).tap();
      await moveStarted.promise;
      proof.arm();
      await desktopTestPage.goto(`/t/${task.task_id}`);
      const deleting = new SessionPage(desktopTestPage);
      await expect(deleting.activeChat()).toHaveAttribute("data-session-id", primary.session_id);
      await deleting.sessionTabBySessionId(old.session_id).click({ button: "right" });
      await deleting.contextMenuItem("Delete").click();
      const removed = watcher.waitForEvent("session.removed", {
        where: (payload) => payload.session_id === old.session_id,
      });
      await desktopTestPage.getByTestId("session-delete-confirm").click();
      await removed;
      await proof.queued;
      proof.release();
      await waitForActiveTaskSession(testPage, task.task_id, null);
      await waitForTaskSessionsSettled(testPage, task.task_id);
      const failed = testPage.waitForResponse(
        (response) =>
          response.request().method() === "POST" &&
          new URL(response.url()).pathname === `/api/v1/tasks/${task.task_id}/move`,
      );
      moveRelease.resolve();
      expect((await failed).status()).toBe(500);
      await expect(testPage.getByText("Failed to move task", { exact: true })).toBeVisible();
      await testPage.keyboard.press("Escape");
      await expect(picker).toBeHidden();
      await dwell(
        testPage,
        1500,
        "negative-assertion",
        "A failed workflow intent is not permission to read a route sibling",
      );
      await waitForActiveTaskSession(testPage, task.task_id, null);
      await expect(
        testPage.locator('[data-testid="session-chat"][data-session-id]:visible'),
      ).toHaveCount(0);
      expect((await sessionReadState(apiClient, other.session_id)).cursor).toBe(before);
      expect(
        traffic.markReads.filter(
          (read) => read.sessionId === other.session_id && read.messageId === tail.messageId,
        ),
      ).toEqual([]);
      await testPage.screenshot({
        path: test.info().outputPath("phone-workflow-cancelled-gap.png"),
      });
      const successor = await apiClient.seedTaskSession(task.task_id, {
        sessionId: randomUUID(),
        agentProfileId: seedData.agentProfileId,
        state: "WAITING_FOR_INPUT",
      });
      const successorTail = await apiClient.seedSessionMessage(successor.session_id, {
        type: "message",
        content: "Proven successor after failed independent move",
      });
      const changedPrimary = watcher.waitForEvent("task.updated", {
        where: (payload) =>
          payload.task_id === task.task_id && payload.primary_session_id === successor.session_id,
      });
      await apiClient.setPrimarySession(successor.session_id);
      await changedPrimary;
      await expect(session.activeChat()).toHaveAttribute("data-session-id", successor.session_id);
      await expect
        .poll(async () => (await sessionReadState(apiClient, successor.session_id)).cursor)
        .toBe(successorTail.messageId);
      expect((await sessionReadState(apiClient, other.session_id)).cursor).toBe(before);
      await tapSessionRow(testPage, other.session_id);
      await expect(session.activeChat()).toHaveAttribute("data-session-id", other.session_id);
      await expect
        .poll(async () => (await sessionReadState(apiClient, other.session_id)).cursor)
        .toBe(tail.messageId);
    } finally {
      moveRelease.resolve();
      await moving;
      await testPage.unroute(movePattern, failMove);
      await proof.dispose();
      await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: preventAutoStart });
    }
  });
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.11
test("retires the last stopped session deleted from another client without reloading", async ({
  testPage,
  desktopTestPage,
  apiClient,
  seedData,
}) => {
  await expectLastStoppedSessionRemoval(testPage, desktopTestPage, apiClient, seedData);
  await testPage.screenshot({ path: test.info().outputPath("phone-last-session-removed.png") });
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.11
test("fills a retired session slot after the later primary promotion", async ({
  testPage,
  desktopTestPage,
  apiClient,
  seedData,
}) => {
  await expectLatePromotionAfterRemoval(testPage, desktopTestPage, apiClient, seedData, true);
  await testPage.screenshot({ path: test.info().outputPath("phone-late-session-promotion.png") });
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.11, AC-TASKS-TASK-UNREAD-DOT-001.15
test("deletes the displayed session through the phone picker and follows its proven survivor", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const { task, old } = await seedStoppedRemovalTask(apiClient, seedData);
  const survivor = await apiClient.seedTaskSession(task.task_id, {
    sessionId: randomUUID(),
    agentProfileId: seedData.agentProfileId,
    state: "WAITING_FOR_INPUT",
  });
  await apiClient.setPrimarySession(old.session_id);
  const watcher = watchWs(testPage);
  await testPage.goto(`/t/${task.task_id}`);
  await expect(new SessionPage(testPage).activeChat()).toHaveAttribute(
    "data-session-id",
    old.session_id,
  );
  const documentStart = await testPage.evaluate(() => performance.timeOrigin);
  await testPage.getByTestId("mobile-sessions-pill").tap();
  const picker = testPage.getByRole("dialog", { name: "Sessions", exact: true });
  const row = picker.getByTestId(`mobile-session-row-${old.session_id}`);
  await expect(row).toHaveAttribute("aria-current", "true");
  await row.getByRole("button", { name: "Session actions", exact: true }).tap();
  await testPage.getByRole("menuitem", { name: "Delete", exact: true }).tap();
  const confirmation = testPage.getByTestId("mobile-session-delete-confirm");
  await expect(confirmation).toBeVisible();
  const acknowledged = watcher.waitForResponse("session.delete");
  await confirmation.tap();
  await acknowledged;
  await expect
    .poll(async () =>
      (await apiClient.listTaskSessions(task.task_id)).sessions.map((item) => item.id),
    )
    .toEqual([survivor.session_id]);
  await expect(picker).toBeVisible();
  await expect(row).toHaveCount(0);
  await expect(picker.getByTestId(`mobile-session-row-${survivor.session_id}`)).toHaveAttribute(
    "aria-current",
    "true",
  );
  await expect(
    testPage.locator(`[data-testid="session-chat"][data-session-id="${old.session_id}"]`),
  ).toHaveCount(0);
  expect(await testPage.evaluate(() => performance.timeOrigin)).toBe(documentStart);
  await expect(
    testPage.getByTestId("toast-message").getByText("Deleting session successful", { exact: true }),
  ).toBeHidden({ timeout: 10_000 });
  await testPage.screenshot({ path: test.info().outputPath("phone-local-session-removed.png") });
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.11, AC-TASKS-TASK-UNREAD-DOT-001.15
test("opens the requested phone conversation after an agentless move settles", async ({
  testPage,
  desktopTestPage,
  apiClient,
  seedData,
}) => {
  const { settings } = await apiClient.getUserSettings();
  await apiClient.saveUserSettings({
    unread_divider: false,
    unread_task_dot: true,
    unread_task_dot_hide_delay_seconds: 0,
    prevent_auto_start_agent_on_open: true,
  });
  try {
    await expectConversationRouteAfterRemoval(testPage, desktopTestPage, apiClient, seedData, {
      moveOutcome: "agentless",
    });
    await testPage.screenshot({ path: test.info().outputPath("phone-agentless-conversation.png") });
  } finally {
    await apiClient.saveUserSettings({
      unread_divider: settings.unread_divider,
      unread_task_dot: settings.unread_task_dot,
      unread_task_dot_hide_delay_seconds: settings.unread_task_dot_hide_delay_seconds,
      prevent_auto_start_agent_on_open: settings.prevent_auto_start_agent_on_open,
      enable_preview_on_click: settings.enable_preview_on_click,
    });
  }
});
