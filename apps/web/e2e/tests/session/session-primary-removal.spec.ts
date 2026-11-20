import { randomUUID } from "node:crypto";
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { KanbanPage } from "../../pages/kanban-page";
import { dwell, watchWs } from "../../helpers/causal-waits";
import {
  expectLastStoppedSessionRemoval,
  expectLatePromotionAfterRemoval,
} from "../../helpers/session-removal";
import {
  holdRemovalMembership,
  observeRemovalSelection,
  REMOVAL_PRIMARY_TEXT,
  seedRemovalChoiceTask,
} from "../../helpers/session-removal";
import { waitForActiveTaskSession, waitForTaskSessionsSettled } from "../../helpers/session-store";
import {
  captureUnreadSettings,
  observeReadTraffic,
  sessionReadState,
  type UnreadSettings,
  cleanupFailingResumeTask,
  seedFailingResumeTask,
  waitForFailedResume,
} from "../../helpers/unread-divider";
import type { DockviewApi } from "dockview-react";

// Following confirmed removal must not require a full document reload.
test("follows a stopped primary deleted from another client without reloading", async ({
  testPage,
  desktopTestPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.seedTask(seedData.workspaceId, "Remote primary removal", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const old = await apiClient.seedTaskSession(task.task_id, {
    sessionId: randomUUID(),
    agentProfileId: seedData.agentProfileId,
    state: "FAILED",
    completedAt: new Date().toISOString(),
    errorMessage: "Stored failure before removal",
  });
  const next = await apiClient.seedTaskSession(task.task_id, {
    sessionId: randomUUID(),
    agentProfileId: seedData.agentProfileId,
    state: "WAITING_FOR_INPUT",
  });
  await apiClient.seedSessionMessage(old.session_id, {
    type: "error",
    content: "Stored failure before removal",
  });
  await apiClient.setPrimarySession(old.session_id);
  const observer = new SessionPage(testPage);
  const deletingClient = new SessionPage(desktopTestPage);
  await testPage.goto(`/t/${task.task_id}`);
  await observer.waitForLoad();
  await expect(observer.activeChat()).toHaveAttribute("data-session-id", old.session_id);
  await desktopTestPage.goto(`/t/${task.task_id}`);
  await deletingClient.waitForLoad();
  await expect(deletingClient.activeChat()).toHaveAttribute("data-session-id", old.session_id);
  const before = (await apiClient.listTaskSessions(task.task_id)).sessions;
  expect(before.find((row) => row.is_primary)?.id).toBe(old.session_id);
  expect(before.find((row) => row.id === old.session_id)?.state).toBe("FAILED");
  const documentStart = await testPage.evaluate(() => performance.timeOrigin);

  await deletingClient.sessionTabBySessionId(old.session_id).click({ button: "right" });
  await deletingClient.contextMenuItem("Delete").click();
  const confirmation = desktopTestPage.getByTestId("session-delete-confirm-popover");
  await expect(confirmation).toBeVisible();
  await confirmation.getByTestId("session-delete-confirm").click();

  await expect
    .poll(async () => {
      const { sessions } = await apiClient.listTaskSessions(task.task_id);
      return sessions.map((row) => ({ id: row.id, primary: row.is_primary }));
    })
    .toEqual([{ id: next.session_id, primary: true }]);
  await expect(observer.activeChat()).toHaveAttribute("data-session-id", next.session_id, {
    timeout: 15_000,
  });
  await expect(observer.sessionTabBySessionId(old.session_id)).toHaveCount(0);
  expect(await testPage.evaluate(() => performance.timeOrigin)).toBe(documentStart);
});

test("follows removal after a failed resume has already lost primary", async ({
  testPage,
  desktopTestPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(240_000);
  const seeded = await seedFailingResumeTask(
    apiClient,
    seedData,
    backend.tmpDir,
    "Remote retained failure removal",
  );
  try {
    const observer = new SessionPage(testPage);
    const deletingClient = new SessionPage(desktopTestPage);
    const watcher = watchWs(testPage);
    const failed = waitForFailedResume(watcher, seeded);
    await testPage.goto(`/t/${seeded.taskId}`);
    await expect(observer.activeChat()).toBeVisible({ timeout: 30_000 });
    await failed;
    await expect(observer.activeChat()).toHaveAttribute("data-session-id", seeded.failingSessionId);
    await expect(observer.activeChat().getByTestId("session-recovery-card")).toBeVisible();
    const before = (await apiClient.listTaskSessions(seeded.taskId)).sessions;
    expect(before.find((row) => row.is_primary)?.id).toBe(seeded.siblingSessionId);
    expect(before.find((row) => row.id === seeded.failingSessionId)?.state).toBe("FAILED");
    const documentStart = await testPage.evaluate(() => performance.timeOrigin);

    await desktopTestPage.goto(`/t/${seeded.taskId}`);
    await deletingClient.waitForLoad();
    const removed = watcher.waitForEvent("session.removed", {
      where: (payload) => payload.session_id === seeded.failingSessionId,
    });
    await deletingClient.sessionTabBySessionId(seeded.failingSessionId).click({ button: "right" });
    await deletingClient.contextMenuItem("Delete").click();
    const confirmation = desktopTestPage.getByTestId("session-delete-confirm-popover");
    await expect(confirmation).toBeVisible();
    await confirmation.getByTestId("session-delete-confirm").click();
    await removed;
    await expect
      .poll(async () => {
        const { sessions } = await apiClient.listTaskSessions(seeded.taskId);
        return sessions.map((row) => ({ id: row.id, primary: row.is_primary }));
      })
      .toEqual([{ id: seeded.siblingSessionId, primary: true }]);
    await expect(observer.activeChat()).toHaveAttribute("data-session-id", seeded.siblingSessionId);
    await expect(observer.sessionTabBySessionId(seeded.failingSessionId)).toHaveCount(0);
    expect(await testPage.evaluate(() => performance.timeOrigin)).toBe(documentStart);
  } finally {
    await cleanupFailingResumeTask(apiClient, seeded);
  }
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.11
test("retires the last stopped session deleted from another client without reloading", async ({
  testPage,
  desktopTestPage,
  apiClient,
  seedData,
}) => {
  await expectLastStoppedSessionRemoval(testPage, desktopTestPage, apiClient, seedData);
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.11
test("fills a retired session slot after the later primary promotion", async ({
  testPage,
  desktopTestPage,
  apiClient,
  seedData,
}) => {
  await expectLatePromotionAfterRemoval(testPage, desktopTestPage, apiClient, seedData);
});

test.describe("owned removal transcript visibility", () => {
  let baseline: UnreadSettings = {};
  let preventAutoStart = false;
  test.beforeEach(async ({ testPage, apiClient }) => {
    await new KanbanPage(testPage).goto();
    baseline = await captureUnreadSettings(apiClient);
    const { settings } = await apiClient.getUserSettings();
    preventAutoStart = settings.prevent_auto_start_agent_on_open === true;
    await apiClient.saveUserSettings({
      unread_divider: false,
      unread_task_dot: true,
      unread_task_dot_hide_delay_seconds: 0,
      prevent_auto_start_agent_on_open: true,
    });
  });
  test.afterEach(async ({ apiClient }) => {
    await apiClient.saveUserSettings({
      ...baseline,
      prevent_auto_start_agent_on_open: preventAutoStart,
    });
  });

  for (const winner of ["proven-chat", "explicit-sibling", "independent-changes"] as const) {
    // @covers AC-TASKS-TASK-UNREAD-DOT-001.6
    // @covers AC-TASKS-TASK-UNREAD-DOT-001.11
    test(`local removal activates ${winner} across the owned empty slot`, async ({
      testPage,
      apiClient,
      seedData,
    }) => {
      test.setTimeout(90_000);
      const { task, old, primary, other } = await seedRemovalChoiceTask(apiClient, seedData);
      await apiClient.setPrimarySession(old.session_id);
      const proof = await holdRemovalMembership(testPage, task.task_id, old.session_id);
      const watcher = watchWs(testPage);
      const traffic = observeReadTraffic(testPage);
      const session = new SessionPage(testPage);
      await testPage.goto(`/t/${task.task_id}`);
      await expect(session.activeChat()).toHaveAttribute("data-session-id", old.session_id);
      await expect(session.sessionTabBySessionId(other.session_id)).toBeVisible();
      await session.sessionTabBySessionId(other.session_id).click();
      await expect(session.activeChat()).toHaveAttribute("data-session-id", other.session_id);
      await session.sessionTabBySessionId(old.session_id).click();
      await expect(session.activeChat()).toHaveAttribute("data-session-id", old.session_id);
      const promoted = watcher.waitForEvent("task.updated", {
        where: (payload) =>
          payload.task_id === task.task_id && payload.primary_session_id === primary.session_id,
      });
      await apiClient.setPrimarySession(primary.session_id);
      await promoted;
      await waitForTaskSessionsSettled(testPage, task.task_id);
      await expect
        .poll(async () => (await sessionReadState(apiClient, other.session_id)).cursor)
        .not.toBeNull();
      const beforeCursor = (await sessionReadState(apiClient, other.session_id)).cursor;
      const tail = await apiClient.seedSessionMessage(other.session_id, {
        type: "message",
        content: "Unread neighboring output after returning to A",
      });
      const history = await observeRemovalSelection(testPage);
      const documentStart = await testPage.evaluate(() => performance.timeOrigin);
      try {
        proof.arm();
        await session.sessionTabBySessionId(old.session_id).click({ button: "right" });
        await session.contextMenuItem("Delete").click();
        const confirmation = testPage.getByTestId("session-delete-confirm-popover");
        await expect(confirmation).toBeVisible();
        const acknowledged = watcher.waitForResponse("session.delete");
        await confirmation.getByTestId("session-delete-confirm").click();
        await acknowledged;
        await proof.queued;
        if (winner === "explicit-sibling") {
          await session.sessionTabBySessionId(other.session_id).click();
          await expect(session.activeChat()).toHaveAttribute("data-session-id", other.session_id);
          await expect
            .poll(async () => (await sessionReadState(apiClient, other.session_id)).cursor)
            .toBe(tail.messageId);
        } else {
          if (winner === "independent-changes") await session.clickTab("Changes");
          await dwell(
            testPage,
            1500,
            "negative-assertion",
            "An implicit fallback cannot acknowledge unread neighboring output while membership is held",
          );
          expect((await sessionReadState(apiClient, other.session_id)).cursor).toBe(beforeCursor);
          expect(
            traffic.markReads.filter(
              (read) => read.sessionId === other.session_id && read.messageId === tail.messageId,
            ),
          ).toEqual([]);
          await expect(session.activeChat()).toHaveCount(0);
        }
        await testPage.screenshot({
          path: test.info().outputPath(`owned-removal-held-${winner}.png`),
        });
        proof.release();
        const expectedSession =
          winner === "explicit-sibling" ? other.session_id : primary.session_id;
        await waitForActiveTaskSession(testPage, task.task_id, expectedSession);
        await waitForTaskSessionsSettled(testPage, task.task_id);
        if (winner !== "explicit-sibling") {
          await expect(session.activeChat()).toHaveAttribute("data-session-id", primary.session_id);
          await expect(
            session.activeChat().getByText(REMOVAL_PRIMARY_TEXT, { exact: true }),
          ).toBeVisible();
          expect((await sessionReadState(apiClient, other.session_id)).cursor).toBe(beforeCursor);
          expect(
            traffic.markReads.filter(
              (read) => read.sessionId === other.session_id && read.messageId === tail.messageId,
            ),
          ).toEqual([]);
          if (winner === "independent-changes") await expect(session.changes).toBeVisible();
          const samples = await history.read();
          expect(samples.map((sample) => sample.sessionId)).not.toContain(other.session_id);
          expect(samples.map((sample) => sample.pinnedSessionId)).not.toContain(other.session_id);
        } else {
          await expect(session.activeChat()).toHaveAttribute("data-session-id", other.session_id);
        }
        expect(await testPage.evaluate(() => performance.timeOrigin)).toBe(documentStart);
        await testPage.screenshot({ path: test.info().outputPath(`owned-removal-${winner}.png`) });
      } finally {
        await proof.dispose();
        await history.stop();
      }
    });
  }

  // @covers AC-TASKS-TASK-UNREAD-DOT-001.6, AC-TASKS-TASK-UNREAD-DOT-001.11
  test("keeps a remote removal's unowned split neighbor unread", async ({
    testPage,
    desktopTestPage,
    apiClient,
    seedData,
  }) => {
    const { task, old, primary, other } = await seedRemovalChoiceTask(apiClient, seedData);
    const watcher = watchWs(testPage);
    const traffic = observeReadTraffic(testPage);
    const session = new SessionPage(testPage);
    const chat = (id: string) =>
      testPage.locator(`[data-testid="session-chat"][data-session-id="${id}"]:visible`);
    await testPage.goto(`/t/${task.task_id}`);
    await expect(chat(primary.session_id)).toBeVisible();
    await session.sessionTabBySessionId(other.session_id).click();
    await expect(chat(other.session_id)).toBeVisible();
    await session.sessionTabBySessionId(old.session_id).click();
    await expect(chat(old.session_id)).toBeVisible();
    await session.clickTab("Files");
    const files = testPage.locator(".dv-default-tab:visible").filter({ hasText: /^Files$/ });
    await expect(files).toHaveCount(1);
    await session.sessionTabBySessionId(primary.session_id).dragTo(files);
    await expect
      .poll(() =>
        testPage.evaluate(
          ({ oldId, primaryId }) => {
            const api = (window as Window & { __dockviewApi__?: DockviewApi }).__dockviewApi__;
            if (!api) throw new Error("Dockview API not attached");
            const oldPanel = api.getPanel(`session:${oldId}`);
            const primaryPanel = api.getPanel(`session:${primaryId}`);
            return !!oldPanel && !!primaryPanel && oldPanel.group.id !== primaryPanel.group.id;
          },
          { oldId: old.session_id, primaryId: primary.session_id },
        ),
      )
      .toBe(true);
    await session.sessionTabBySessionId(old.session_id).click();
    await expect(chat(old.session_id)).toBeVisible();
    await expect(chat(primary.session_id)).toBeVisible();
    await expect
      .poll(async () => (await sessionReadState(apiClient, other.session_id)).cursor)
      .not.toBeNull();
    const before = (await sessionReadState(apiClient, other.session_id)).cursor;
    const tail = await apiClient.seedSessionMessage(other.session_id, {
      type: "message",
      content: "Unseen split neighbor after returning to the removed view",
    });
    const documentStart = await testPage.evaluate(() => performance.timeOrigin);
    const deleting = new SessionPage(desktopTestPage);
    await desktopTestPage.goto(`/t/${task.task_id}`);
    await deleting.waitForLoad();
    await deleting.sessionTabBySessionId(old.session_id).click({ button: "right" });
    await deleting.contextMenuItem("Delete").click();
    const removed = watcher.waitForEvent("session.removed", {
      where: (payload) => payload.session_id === old.session_id,
    });
    await desktopTestPage.getByTestId("session-delete-confirm").click();
    await removed;
    await waitForActiveTaskSession(testPage, task.task_id, primary.session_id);
    await waitForTaskSessionsSettled(testPage, task.task_id);
    await expect(session.sessionTabBySessionId(old.session_id)).toHaveCount(0);
    await expect(
      chat(primary.session_id).getByText(REMOVAL_PRIMARY_TEXT, { exact: true }),
    ).toBeVisible();
    await dwell(
      testPage,
      1500,
      "negative-assertion",
      "A direct remote proof cannot make its unproven split neighbor readable",
    );
    await expect(chat(other.session_id)).toHaveCount(0);
    expect((await sessionReadState(apiClient, other.session_id)).cursor).toBe(before);
    expect(
      traffic.markReads.filter(
        (read) => read.sessionId === other.session_id && read.messageId === tail.messageId,
      ),
    ).toEqual([]);
    await testPage.screenshot({ path: test.info().outputPath("remote-split-neighbor-unread.png") });
    await session.sessionTabBySessionId(other.session_id).click();
    await expect(chat(other.session_id)).toBeVisible();
    await expect
      .poll(async () => (await sessionReadState(apiClient, other.session_id)).cursor)
      .toBe(tail.messageId);
    await expect(chat(primary.session_id)).toBeVisible();
    expect(await testPage.evaluate(() => performance.timeOrigin)).toBe(documentStart);
  });

  // @covers AC-TASKS-TASK-UNREAD-DOT-001.6, AC-TASKS-TASK-UNREAD-DOT-001.12
  test("keeps a hidden proven primary from selecting an unread neighbor", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.saveUserSettings({ agent_tab_close_behavior: "hide_panel" });
    const { task, old, primary, other } = await seedRemovalChoiceTask(apiClient, seedData);
    await apiClient.setPrimarySession(old.session_id);
    const proof = await holdRemovalMembership(testPage, task.task_id, old.session_id);
    const watcher = watchWs(testPage);
    const traffic = observeReadTraffic(testPage);
    const session = new SessionPage(testPage);
    await testPage.goto(`/t/${task.task_id}`);
    await expect(session.activeChat()).toHaveAttribute("data-session-id", old.session_id);
    await session.sessionTabBySessionId(other.session_id).click();
    await expect(session.activeChat()).toHaveAttribute("data-session-id", other.session_id);
    await session.sessionTabBySessionId(old.session_id).click();
    const promoted = watcher.waitForEvent("task.updated", {
      where: (payload) =>
        payload.task_id === task.task_id && payload.primary_session_id === primary.session_id,
    });
    await apiClient.setPrimarySession(primary.session_id);
    await promoted;
    await session.sessionTabCloseButton(primary.session_id).click();
    await expect(session.sessionTabBySessionId(primary.session_id)).toHaveCount(0);
    await expect(session.activeChat()).toHaveAttribute("data-session-id", old.session_id);
    await expect
      .poll(async () => (await sessionReadState(apiClient, other.session_id)).cursor)
      .not.toBeNull();
    const before = (await sessionReadState(apiClient, other.session_id)).cursor;
    const hiddenCursor = (await sessionReadState(apiClient, primary.session_id)).cursor;
    const tail = await apiClient.seedSessionMessage(other.session_id, {
      type: "message",
      content: "Unseen neighbor while the proven primary is hidden",
    });
    try {
      proof.arm();
      await session.sessionTabBySessionId(old.session_id).click({ button: "right" });
      await session.contextMenuItem("Delete").click();
      const acknowledged = watcher.waitForResponse("session.delete");
      await testPage.getByTestId("session-delete-confirm").click();
      await acknowledged;
      await proof.queued;
      proof.release();
      await waitForActiveTaskSession(testPage, task.task_id, primary.session_id);
      await waitForTaskSessionsSettled(testPage, task.task_id);
      await dwell(
        testPage,
        1500,
        "negative-assertion",
        "A hidden proven primary cannot authorize a neighboring transcript or cursor",
      );
      await expect(session.activeChat()).toHaveCount(0);
      await expect(session.sessionTabBySessionId(primary.session_id)).toHaveCount(0);
      expect((await sessionReadState(apiClient, other.session_id)).cursor).toBe(before);
      expect((await sessionReadState(apiClient, primary.session_id)).cursor).toBe(hiddenCursor);
      expect(
        traffic.markReads.filter(
          (read) => read.sessionId === other.session_id && read.messageId === tail.messageId,
        ),
      ).toEqual([]);
      await testPage.screenshot({ path: test.info().outputPath("hidden-primary-owned-empty.png") });
      await session.sessionTabBySessionId(other.session_id).click();
      await expect(session.activeChat()).toHaveAttribute("data-session-id", other.session_id);
      await expect
        .poll(async () => (await sessionReadState(apiClient, other.session_id)).cursor)
        .toBe(tail.messageId);
      expect((await sessionReadState(apiClient, primary.session_id)).cursor).toBe(hiddenCursor);
    } finally {
      await proof.dispose();
    }
  });
});
