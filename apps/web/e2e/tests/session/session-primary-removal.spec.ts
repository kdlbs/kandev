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
  holdParkedLayoutFrames,
  holdInitialMembership,
  holdInitialTaskIdentity,
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
  taskUnreadSessionIds,
  TASK_UNREAD_DOT_LABEL,
  newestStoredMessageId,
} from "../../helpers/unread-divider";
import type { DockviewApi } from "dockview-react";
import {
  expectConversationRouteAfterRemoval,
  expectPrimarySwapDuringRouteAdmission,
} from "../../helpers/session-route-admission";
import { expectBootConversationRoute } from "../../helpers/session-route-boot";

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
  let previewEnabled = false;
  test.beforeEach(async ({ testPage, apiClient }) => {
    await new KanbanPage(testPage).goto();
    baseline = await captureUnreadSettings(apiClient);
    const { settings } = await apiClient.getUserSettings();
    preventAutoStart = settings.prevent_auto_start_agent_on_open === true;
    previewEnabled = settings.enable_preview_on_click === true;
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
      enable_preview_on_click: previewEnabled,
    });
  });

  for (const local of [false, true]) {
    test(`settles ${local ? "confirmed local" : "remote"} removal on an ordinary foreground membership refresh after proof failure`, async ({
      testPage,
      desktopTestPage,
      apiClient,
      seedData,
    }) => {
      const { task, old, primary, other } = await seedRemovalChoiceTask(apiClient, seedData);
      await apiClient.seedTaskSession(task.task_id, {
        sessionId: other.session_id,
        agentProfileId: seedData.agentProfileId,
        state: "COMPLETED",
        completedAt: new Date().toISOString(),
      });
      await apiClient.setPrimarySession(old.session_id);
      const observer = new SessionPage(testPage);
      const watcher = watchWs(testPage);
      const traffic = observeReadTraffic(testPage);
      await testPage.goto(`/t/${task.task_id}`);
      await expect(observer.activeChat()).toHaveAttribute("data-session-id", old.session_id);
      await waitForTaskSessionsSettled(testPage, task.task_id);
      const tail = await newestStoredMessageId(apiClient, primary.session_id);
      const failed = Promise.withResolvers<void>();
      let fresh = false;
      let positiveReads = 0;
      const pattern = `**/api/v1/tasks/${task.task_id}/sessions`;
      const routeMembership = async (route: import("@playwright/test").Route) => {
        if (!fresh) {
          await route.fulfill({
            status: 503,
            contentType: "application/json",
            body: JSON.stringify({ error: "temporary proof failure" }),
          });
          failed.resolve();
        } else {
          positiveReads++;
          await route.continue();
        }
      };
      await testPage.route(pattern, routeMembership);
      try {
        const removed = local
          ? Promise.resolve()
          : watcher.waitForEvent("session.removed", {
              where: (payload) => payload.session_id === old.session_id,
            });
        const promoted = watcher.waitForEvent("task.updated", {
          where: (payload) =>
            payload.task_id === task.task_id && payload.primary_session_id === primary.session_id,
        });
        if (local) {
          await observer.sessionTabBySessionId(old.session_id).click({ button: "right" });
          await testPage.getByRole("menuitem", { name: "Delete", exact: true }).click();
          await testPage.getByTestId("session-delete-confirm").click();
        } else {
          await apiClient.saveUserSettings({ enable_preview_on_click: true });
          await desktopTestPage.goto(
            `/?workflowId=${seedData.workflowId}&taskId=${task.task_id}&sessionId=${old.session_id}`,
          );
          await desktopTestPage.bringToFront();
          const preview = desktopTestPage.getByTestId("task-preview-panel");
          await expect(preview.locator('[data-testid="session-chat"]:visible')).toHaveAttribute(
            "data-session-id",
            old.session_id,
          );
          await preview
            .getByTestId(`preview-session-tab-${old.session_id}`)
            .click({ button: "right" });
          await desktopTestPage.getByRole("menuitem", { name: "Delete", exact: true }).click();
          await desktopTestPage.getByTestId("session-delete-confirm").click();
        }
        await Promise.all([removed, promoted, failed.promise]);
        await dwell(
          testPage,
          500,
          "negative-assertion",
          "Failed absence reads must retain the predecessor until independent proof",
        );
        if (local) {
          await expect(observer.activeChat()).toHaveCount(0);
          await expect(observer.sessionTabBySessionId(old.session_id)).toHaveCount(0);
          await desktopTestPage.bringToFront();
        } else {
          await waitForActiveTaskSession(testPage, task.task_id, old.session_id);
        }
        expect((await sessionReadState(apiClient, primary.session_id)).cursor).toBeNull();
        fresh = true;
        await testPage.bringToFront();
        await waitForActiveTaskSession(testPage, task.task_id, primary.session_id);
        await expect(observer.activeChat()).toHaveAttribute("data-session-id", primary.session_id);
        await expect
          .poll(async () => (await sessionReadState(apiClient, primary.session_id)).cursor)
          .toBe(tail);
        expect(traffic.markReads).toContainEqual({
          sessionId: primary.session_id,
          messageId: tail,
        });
        expect(positiveReads).toBeGreaterThan(0);
        await expect(observer.sessionTabBySessionId(old.session_id)).toHaveCount(0);
        expect((await sessionReadState(apiClient, other.session_id)).cursor).toBeNull();
        expect(traffic.agentStarts()).toEqual([]);
        await testPage.screenshot({
          path: test
            .info()
            .outputPath(`membership-proof-foreground-${local ? "local" : "remote"}.png`),
        });
      } finally {
        await testPage.unroute(pattern, routeMembership);
      }
    });
  }

  // @covers AC-TASKS-TASK-UNREAD-DOT-001.6, AC-TASKS-TASK-UNREAD-DOT-001.17
  test("opens a Command Panel conversation after a settled same-task removal", async ({
    testPage,
    desktopTestPage,
    apiClient,
    seedData,
  }) => {
    await expectConversationRouteAfterRemoval(testPage, desktopTestPage, apiClient, seedData);
    await testPage.screenshot({ path: test.info().outputPath("explicit-route-after-removal.png") });
  });

  // @covers AC-TASKS-TASK-UNREAD-DOT-001.6, AC-TASKS-TASK-UNREAD-DOT-001.12
  test("opens a hidden explicit conversation through real Go boot", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const closeBehavior = (await apiClient.getUserSettings()).settings.agent_tab_close_behavior;
    try {
      await apiClient.saveUserSettings({ agent_tab_close_behavior: "hide_panel" });
      const opened = await expectBootConversationRoute(testPage, apiClient, seedData);
      await testPage.screenshot({ path: test.info().outputPath("boot-explicit-conversation.png") });
      const session = new SessionPage(testPage);
      await session.addPanelButton().click();
      await testPage.getByRole("menuitem", { name: "Plan", exact: true }).click();
      await expect(session.activeChat()).toHaveCount(0);
      const traffic = observeReadTraffic(testPage);
      const later = await apiClient.seedSessionMessage(opened.requestedId, {
        type: "message",
        content: "Plan must survive consumed boot focus",
      });
      await dwell(testPage, 500, "negative-assertion", "Consumed boot focus must not steal Plan");
      await expect(session.activeChat()).toHaveCount(0);
      expect((await sessionReadState(apiClient, opened.requestedId)).cursor).toBe(
        opened.requestedTail,
      );
      expect(traffic.markReads.filter((row) => row.messageId === later.messageId)).toEqual([]);
    } finally {
      await apiClient.saveUserSettings({ agent_tab_close_behavior: closeBehavior });
    }
  });

  test("opens an inactive maximized conversation through real Go boot", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await expectBootConversationRoute(testPage, apiClient, seedData, true);
    await testPage.screenshot({ path: test.info().outputPath("boot-maximized-conversation.png") });
  });

  test("opens a parked split conversation through real Go boot", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await expectBootConversationRoute(testPage, apiClient, seedData, "parked");
    await testPage.screenshot({ path: test.info().outputPath("boot-parked-conversation.png") });
  });

  test("opens a hidden never-snapshotted conversation through real Go boot", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const closeBehavior = (await apiClient.getUserSettings()).settings.agent_tab_close_behavior;
    try {
      await apiClient.saveUserSettings({ agent_tab_close_behavior: "hide_panel" });
      await expectBootConversationRoute(testPage, apiClient, seedData, "hidden");
      await testPage.screenshot({ path: test.info().outputPath("boot-hidden-maximized.png") });
    } finally {
      await apiClient.saveUserSettings({ agent_tab_close_behavior: closeBehavior });
    }
  });

  test("restores a vertically split conversation through real Go boot", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await expectBootConversationRoute(testPage, apiClient, seedData, "tree");
    await testPage.screenshot({ path: test.info().outputPath("boot-tree-restore.png") });
  });

  test("restores a legacy maximize selection through real Go boot", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await expectBootConversationRoute(testPage, apiClient, seedData, "legacy");
    await testPage.screenshot({ path: test.info().outputPath("boot-legacy-restore.png") });
  });

  for (const [mode, moveOutcome] of [
    ["hidden", undefined],
    ["failed-workflow", "failed"],
    ["agentless-workflow", "agentless"],
    ["skip-prompt-workflow", "skip"],
  ] as const) {
    // @covers AC-TASKS-TASK-UNREAD-DOT-001.6, AC-TASKS-TASK-UNREAD-DOT-001.12
    test(`opens an explicit conversation route across ${mode}`, async ({
      testPage,
      desktopTestPage,
      apiClient,
      seedData,
    }) => {
      const closeBehavior = (await apiClient.getUserSettings()).agent_tab_close_behavior;
      try {
        if (mode === "hidden")
          await apiClient.saveUserSettings({ agent_tab_close_behavior: "hide_panel" });
        await expectConversationRouteAfterRemoval(testPage, desktopTestPage, apiClient, seedData, {
          hideTarget: mode === "hidden",
          moveOutcome,
        });
        await testPage.screenshot({ path: test.info().outputPath(`explicit-route-${mode}.png`) });
      } finally {
        await apiClient.saveUserSettings({ agent_tab_close_behavior: closeBehavior });
      }
    });
  }

  // @covers AC-TASKS-TASK-UNREAD-DOT-001.6, AC-TASKS-TASK-UNREAD-DOT-001.17
  test("uses the post-swap primary across independent initial HTTP legs", async ({
    testPage,
    desktopTestPage,
    apiClient,
    seedData,
  }) => {
    await expectPrimarySwapDuringRouteAdmission(testPage, desktopTestPage, apiClient, seedData);
    await testPage.screenshot({ path: test.info().outputPath("primary-snapshot-reconciled.png") });
  });

  // @covers AC-TASKS-TASK-UNREAD-DOT-001.6, AC-TASKS-TASK-UNREAD-DOT-001.17
  test("uses a primary created after the initial membership snapshot", async ({
    testPage,
    desktopTestPage,
    apiClient,
    seedData,
  }) => {
    await expectPrimarySwapDuringRouteAdmission(testPage, desktopTestPage, apiClient, seedData, {
      createAfterMembership: true,
    });
    await testPage.screenshot({ path: test.info().outputPath("created-primary-reconciled.png") });
  });

  // @covers AC-TASKS-TASK-UNREAD-DOT-001.6, AC-TASKS-TASK-UNREAD-DOT-001.17
  test("keeps a proven conversation despite unrelated primary refresh failure", async ({
    testPage,
    desktopTestPage,
    apiClient,
    seedData,
  }) => {
    await expectPrimarySwapDuringRouteAdmission(testPage, desktopTestPage, apiClient, seedData, {
      explicitMember: true,
    });
    await testPage.screenshot({ path: test.info().outputPath("explicit-member-primary-skew.png") });
  });

  for (const winner of [
    "proven-chat",
    "explicit-sibling",
    "independent-changes",
    "independent-plan",
  ] as const) {
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
        type: winner === "independent-plan" ? "user_message" : "message",
        content: "Unread neighboring output after returning to A",
      });
      const primaryTail =
        winner === "independent-plan"
          ? await apiClient.seedSessionMessage(primary.session_id, {
              type: "message",
              content: "Unread recipient behind an independently selected Plan",
            })
          : null;
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
          if (winner === "independent-plan") {
            await session.addPanelButton().click();
            await testPage.getByRole("menuitem", { name: "Plan", exact: true }).click();
          }
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
        if (winner === "independent-plan") {
          await expect(session.activeChat()).toHaveCount(0);
          expect((await sessionReadState(apiClient, primary.session_id)).cursor).toBeNull();
          await expect
            .poll(() => taskUnreadSessionIds(apiClient, seedData.workspaceId, task.task_id))
            .toEqual([primary.session_id]);
          const taskRow = testPage.getByTestId("sidebar-task-item").filter({ hasText: task.title });
          await expect(taskRow.getByRole("img", { name: TASK_UNREAD_DOT_LABEL })).toHaveCount(1);
          await session.sessionTabBySessionId(primary.session_id).click();
          await expect
            .poll(async () => (await sessionReadState(apiClient, primary.session_id)).cursor)
            .toBe(primaryTail!.messageId);
          await expect(taskRow.getByRole("img", { name: TASK_UNREAD_DOT_LABEL })).toHaveCount(0);
        } else if (winner !== "explicit-sibling") {
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
  test("hydrates a Preview-to-workbench route when initial membership is retired", async ({
    testPage,
    desktopTestPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.saveUserSettings({ enable_preview_on_click: true });
    const { task, old, primary, other } = await seedRemovalChoiceTask(apiClient, seedData);
    await apiClient.seedTaskSession(task.task_id, {
      sessionId: other.session_id,
      agentProfileId: seedData.agentProfileId,
      state: "COMPLETED",
      completedAt: new Date().toISOString(),
    });
    await apiClient.seedTaskSession(task.task_id, {
      sessionId: old.session_id,
      agentProfileId: seedData.agentProfileId,
      state: "WAITING_FOR_INPUT",
    });
    await apiClient.setPrimarySession(old.session_id);
    await apiClient.seedTaskSession(task.task_id, {
      sessionId: old.session_id,
      agentProfileId: seedData.agentProfileId,
      state: "FAILED",
      completedAt: new Date().toISOString(),
      errorMessage: "Removed conversation",
    });
    for (const page of [testPage, desktopTestPage]) {
      await page.goto(`/?taskId=${task.task_id}&sessionId=${old.session_id}`);
      await expect(
        page.getByTestId("task-preview-panel").locator('[data-testid="session-chat"]:visible'),
      ).toHaveAttribute("data-session-id", old.session_id);
    }
    const gate = await holdInitialMembership(testPage, task.task_id, old.session_id);
    const taskGate = await holdInitialTaskIdentity(testPage, task.task_id, old.session_id);
    const reads = observeReadTraffic(testPage);
    const primaryTail = await newestStoredMessageId(apiClient, primary.session_id);
    try {
      const started = await testPage.evaluate(() => performance.timeOrigin);
      await testPage
        .getByTestId("task-preview-panel")
        .getByRole("button", { name: "Open full page" })
        .click();
      await Promise.all([gate.queued, taskGate.queued]);
      await dwell(
        testPage,
        1500,
        "negative-assertion",
        "Initial route membership must settle before acknowledgement",
      );
      expect((await sessionReadState(apiClient, old.session_id)).cursor).toBeNull();
      const preview = desktopTestPage.getByTestId("task-preview-panel");
      await preview.getByTestId(`preview-session-tab-${old.session_id}`).click({ button: "right" });
      await desktopTestPage.getByRole("menuitem", { name: "Delete", exact: true }).click();
      await desktopTestPage.getByTestId("session-delete-confirm").click();
      await waitForActiveTaskSession(testPage, task.task_id, primary.session_id);
      const membership = await apiClient.listTaskSessions(task.task_id);
      expect(membership.sessions.map((row) => row.id)).toEqual([
        other.session_id,
        primary.session_id,
      ]);
      expect(membership.sessions.find((row) => row.is_primary)?.id).toBe(primary.session_id);
      gate.release();
      taskGate.release();
      const session = new SessionPage(testPage);
      await expect(session.activeChat()).toHaveAttribute("data-session-id", primary.session_id);
      await expect(
        session.activeChat().getByText(REMOVAL_PRIMARY_TEXT, { exact: true }),
      ).toBeVisible();
      await expect
        .poll(async () => (await sessionReadState(apiClient, primary.session_id)).cursor)
        .toBe(primaryTail);
      expect(await testPage.evaluate(() => performance.timeOrigin)).toBe(started);
      await dwell(
        testPage,
        1500,
        "negative-assertion",
        "The newer completed sibling was not selected",
      );
      expect((await sessionReadState(apiClient, other.session_id)).cursor).toBeNull();
      expect(reads.markReads.filter((read) => read.sessionId === other.session_id)).toEqual([]);
      await testPage.screenshot({
        path: test.info().outputPath("initial-membership-recovered.png"),
      });
    } finally {
      await gate.dispose();
      await taskGate.dispose();
    }
  });

  for (const parked of [
    "live",
    "visible",
    "hidden",
    "overlay",
    "rebuilding",
    "inactive",
  ] as const) {
    test(`keeps a remote removal's unowned split neighbor unread (parked: ${parked})`, async ({
      testPage,
      desktopTestPage,
      apiClient,
      seedData,
    }) => {
      const hidden = ["hidden", "overlay", "rebuilding"].includes(parked);
      await apiClient.saveUserSettings({
        enable_preview_on_click: true,
        agent_tab_close_behavior: "hide_panel",
      });
      const { task, old, primary, other } = await seedRemovalChoiceTask(
        apiClient,
        seedData,
        parked !== "live",
      );
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
      if (hidden) {
        await session.sessionTabCloseButton(primary.session_id).click();
        await expect(session.sessionTabBySessionId(primary.session_id)).toHaveCount(0);
      } else if (parked !== "inactive") {
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
      }
      await expect(chat(other.session_id)).toHaveCount(0);
      await expect
        .poll(async () => (await sessionReadState(apiClient, other.session_id)).cursor)
        .not.toBeNull();
      const before = (await sessionReadState(apiClient, other.session_id)).cursor;
      const tail = await apiClient.seedSessionMessage(other.session_id, {
        type: "message",
        content: "Unseen split neighbor after returning to the removed view",
      });
      await expect
        .poll(async () => (await sessionReadState(apiClient, primary.session_id)).cursor)
        .not.toBeNull();
      const hiddenCursor = (await sessionReadState(apiClient, primary.session_id)).cursor;
      const hiddenTail =
        hidden || parked === "inactive"
          ? await apiClient.seedSessionMessage(primary.session_id, {
              type: "message",
              content: "Hidden primary must survive parked reconciliation unread",
            })
          : null;
      expect((await sessionReadState(apiClient, other.session_id)).cursor).toBe(before);
      const frameGate =
        parked === "rebuilding" ? await holdParkedLayoutFrames(testPage, old.session_id) : null;
      const maximize = () =>
        parked === "overlay"
          ? testPage
              .locator(".dv-tabs-and-actions-container")
              .filter({ has: session.sessionTabBySessionId(other.session_id) })
              .getByTestId("dockview-maximize-btn")
              .click()
          : session.clickMaximize("Files");
      const expectVisiblePrimary = async (afterNeighbor = false) => {
        if (afterNeighbor && parked === "inactive") return;
        await expect(
          chat(primary.session_id).getByText(REMOVAL_PRIMARY_TEXT, { exact: true }),
        ).toBeVisible();
        if (!afterNeighbor && parked === "inactive")
          await expect
            .poll(async () => (await sessionReadState(apiClient, primary.session_id)).cursor)
            .toBe(hiddenTail!.messageId);
      };
      try {
        if (parked !== "live") {
          await maximize();
          if (parked === "overlay") await expect(chat(old.session_id)).toBeVisible();
          else await expect(session.sessionTabBySessionId(old.session_id)).toHaveCount(0);
          if (frameGate) await expect.poll(frameGate.count).toBeGreaterThan(0);
        }
        expect((await sessionReadState(apiClient, other.session_id)).cursor).toBe(before);
        const documentStart = await testPage.evaluate(() => performance.timeOrigin);
        await desktopTestPage.goto(`/?taskId=${task.task_id}&sessionId=${old.session_id}`);
        const preview = desktopTestPage.getByTestId("task-preview-panel");
        await expect(preview.locator('[data-testid="session-chat"]:visible')).toHaveAttribute(
          "data-session-id",
          old.session_id,
        );
        await preview
          .getByTestId(`preview-session-tab-${old.session_id}`)
          .click({ button: "right" });
        await desktopTestPage.getByRole("menuitem", { name: "Delete", exact: true }).click();
        const removed = watcher.waitForEvent("session.removed", {
          where: (payload) => payload.session_id === old.session_id,
        });
        await desktopTestPage.getByTestId("session-delete-confirm").click();
        await removed;
        await waitForActiveTaskSession(testPage, task.task_id, primary.session_id);
        await frameGate?.release();
        await waitForTaskSessionsSettled(testPage, task.task_id);
        expect((await sessionReadState(apiClient, other.session_id)).cursor).toBe(before);
        if (parked !== "live") await maximize();
        await expect(session.sessionTabBySessionId(old.session_id)).toHaveCount(0);
        if (!hidden) {
          await expectVisiblePrimary();
        } else {
          await expect(session.sessionTabBySessionId(primary.session_id)).toHaveCount(0);
          expect((await sessionReadState(apiClient, primary.session_id)).cursor).toBe(hiddenCursor);
          await expect
            .poll(() => taskUnreadSessionIds(apiClient, seedData.workspaceId, task.task_id))
            .toContain(primary.session_id);
        }
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
        await testPage.screenshot({
          path: test.info().outputPath("remote-split-neighbor-unread.png"),
        });
        if (hidden) {
          await session.addPanelButton().click();
          await session.sessionReopenItem(primary.session_id).click();
          await expect
            .poll(async () => (await sessionReadState(apiClient, primary.session_id)).cursor)
            .toBe(hiddenTail!.messageId);
        }
        await session.sessionTabBySessionId(other.session_id).click();
        await expect(chat(other.session_id)).toBeVisible();
        await expect
          .poll(async () => (await sessionReadState(apiClient, other.session_id)).cursor)
          .toBe(tail.messageId);
        if (!hidden) await expectVisiblePrimary(true);
        expect(await testPage.evaluate(() => performance.timeOrigin)).toBe(documentStart);
      } finally {
        await frameGate?.dispose();
      }
    });
  }

  // @covers AC-TASKS-TASK-UNREAD-DOT-001.6, AC-TASKS-TASK-UNREAD-DOT-001.12
  test("keeps a visit-local hide made while maximized until explicit reopen", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.saveUserSettings({ agent_tab_close_behavior: "hide_panel" });
    const { task, old, other } = await seedRemovalChoiceTask(apiClient, seedData);
    const session = new SessionPage(testPage);
    const traffic = observeReadTraffic(testPage);
    const header = () =>
      testPage
        .locator(".dv-tabs-and-actions-container")
        .filter({ has: session.sessionTabBySessionId(old.session_id) });
    await testPage.goto(`/t/${task.task_id}`);
    await session.sessionTabBySessionId(other.session_id).click();
    await expect
      .poll(async () => (await sessionReadState(apiClient, other.session_id)).cursor)
      .not.toBeNull();
    await session.sessionTabBySessionId(old.session_id).click();
    const before = (await sessionReadState(apiClient, other.session_id)).cursor;
    const tail = await apiClient.seedSessionMessage(other.session_id, {
      type: "message",
      content: "Visit-local hidden output",
    });
    await header().getByTestId("dockview-maximize-btn").click();
    await session.sessionTabCloseButton(other.session_id).click();
    await expect(session.sessionTabBySessionId(other.session_id)).toHaveCount(0);
    await header().getByTestId("dockview-maximize-btn").click();
    await expect(session.sessionTabBySessionId(other.session_id)).toHaveCount(0);
    await dwell(
      testPage,
      1500,
      "negative-assertion",
      "Restoring a layout must not reopen hidden output",
    );
    expect((await sessionReadState(apiClient, other.session_id)).cursor).toBe(before);
    expect(
      traffic.markReads.filter(
        (read) => read.sessionId === other.session_id && read.messageId === tail.messageId,
      ),
    ).toEqual([]);
    await testPage.screenshot({ path: test.info().outputPath("visit-local-hidden-restore.png") });
    await session.addPanelButton().click();
    await session.sessionReopenItem(other.session_id).click();
    await expect
      .poll(async () => (await sessionReadState(apiClient, other.session_id)).cursor)
      .toBe(tail.messageId);
  });

  // @covers AC-TASKS-TASK-UNREAD-DOT-001.6, AC-TASKS-TASK-UNREAD-DOT-001.12
  for (const restore of [false, true]) {
    test(`keeps a hidden primary owned through unrelated hide and layout restore: ${restore}`, async ({
      testPage,
      apiClient,
      seedData,
    }) => {
      await apiClient.saveUserSettings({ agent_tab_close_behavior: "hide_panel" });
      const { task, old, primary, other } = await seedRemovalChoiceTask(
        apiClient,
        seedData,
        restore,
      );
      const different = await apiClient.seedTaskSession(task.task_id, {
        sessionId: randomUUID(),
        agentProfileId: seedData.agentProfileId,
        state: "WAITING_FOR_INPUT",
      });
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
        await waitForActiveTaskSession(testPage, task.task_id, null);
        if (restore) {
          const toggle = testPage.getByTestId("task-right-panels-toggle");
          await expect(toggle).toBeEnabled();
          const expanded = await toggle.getAttribute("aria-expanded");
          await toggle.click();
          await expect(toggle).toBeEnabled();
          await expect(toggle).toHaveAttribute("aria-expanded", String(expanded !== "true"));
          await dwell(
            testPage,
            1500,
            "negative-assertion",
            "Right-pane rebuild retains the deletion gap",
          );
          await waitForActiveTaskSession(testPage, task.task_id, null);
          expect((await sessionReadState(apiClient, other.session_id)).cursor).toBe(before);
          await expect(session.activeChat()).toHaveCount(0);
          await toggle.click();
          await expect(toggle).toBeEnabled();
          await session.clickMaximize("Files");
          await expect(session.sessionTabBySessionId(other.session_id)).toHaveCount(0);
          await session.clickMaximize("Files");
          await expect(session.sessionTabBySessionId(other.session_id)).toBeVisible();
          await expect(session.activeChat()).toHaveCount(0);
        }
        proof.release();
        await waitForActiveTaskSession(testPage, task.task_id, primary.session_id);
        await waitForTaskSessionsSettled(testPage, task.task_id);
        await session.sessionTabCloseButton(different.session_id).click();
        await expect(session.sessionTabBySessionId(different.session_id)).toHaveCount(0);
        if (restore) {
          const currentStep = (await apiClient.getTask(task.task_id)).workflow_step_id;
          const targetStep = seedData.steps.find((step) => step.id !== currentStep);
          if (!targetStep) throw new Error("Missing workflow destination");
          const pattern = `**/api/v1/tasks/${task.task_id}/move`;
          await testPage.route(
            pattern,
            (route) => route.fulfill({ status: 500, json: { error: "Injected workflow failure" } }),
            { times: 1 },
          );
          const failed = testPage.waitForResponse(
            (response) =>
              response.request().method() === "POST" &&
              new URL(response.url()).pathname === `/api/v1/tasks/${task.task_id}/move`,
          );
          await testPage.getByTestId(`workflow-step-${targetStep.name}`).hover();
          await testPage.getByTestId("workflow-step-move-here").click();
          expect((await failed).status()).toBe(500);
          await expect(testPage.getByText("Failed to move task", { exact: true })).toBeVisible();
          const toggle = testPage.getByTestId("task-right-panels-toggle");
          await expect(toggle).toBeEnabled();
          await toggle.click();
          await expect(toggle).toBeEnabled();
        }
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
        await expect
          .poll(() => taskUnreadSessionIds(apiClient, seedData.workspaceId, task.task_id))
          .toContain(other.session_id);
        const taskRow = testPage.getByTestId("sidebar-task-item").filter({ hasText: task.title });
        await expect(taskRow.getByRole("img", { name: TASK_UNREAD_DOT_LABEL })).toHaveCount(1);
        await testPage.screenshot({
          path: test.info().outputPath("hidden-primary-owned-empty.png"),
        });
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
  }

  // @covers AC-TASKS-TASK-UNREAD-DOT-001.6, AC-TASKS-TASK-UNREAD-DOT-001.12
  test("does not reopen a hidden primary when another client deletes the last visible chat", async ({
    testPage,
    desktopTestPage,
    apiClient,
    seedData,
  }) => {
    const { settings } = await apiClient.getUserSettings();
    const previewEnabled = settings.enable_preview_on_click === true;
    await apiClient.saveUserSettings({
      agent_tab_close_behavior: "hide_panel",
      enable_preview_on_click: true,
    });
    const { task, old, primary, other } = await seedRemovalChoiceTask(apiClient, seedData);
    await apiClient.setPrimarySession(old.session_id);
    const watcher = watchWs(testPage);
    const traffic = observeReadTraffic(testPage);
    const session = new SessionPage(testPage);
    try {
      await testPage.goto(`/t/${task.task_id}`);
      await expect(session.activeChat()).toHaveAttribute("data-session-id", old.session_id);
      await session.sessionTabBySessionId(other.session_id).click();
      await session.sessionTabBySessionId(old.session_id).click();
      const promoted = watcher.waitForEvent("task.updated", {
        where: (payload) =>
          payload.task_id === task.task_id && payload.primary_session_id === primary.session_id,
      });
      await apiClient.setPrimarySession(primary.session_id);
      await promoted;
      await session.sessionTabCloseButton(other.session_id).click();
      await session.sessionTabCloseButton(primary.session_id).click();
      await expect(session.sessionTabBySessionId(primary.session_id)).toHaveCount(0);
      await expect(session.activeChat()).toHaveAttribute("data-session-id", old.session_id);
      const before = (await sessionReadState(apiClient, primary.session_id)).cursor;
      const tail = await apiClient.seedSessionMessage(primary.session_id, {
        type: "message",
        content: "Hidden recipient remains unread after direct removal",
      });
      const documentStart = await testPage.evaluate(() => performance.timeOrigin);
      // Preview is read-only for cursors; the deleting client must not read C itself.
      await desktopTestPage.goto(`/?taskId=${task.task_id}&sessionId=${old.session_id}`);
      const preview = desktopTestPage.getByTestId("task-preview-panel");
      await expect(preview.locator('[data-testid="session-chat"]:visible')).toHaveAttribute(
        "data-session-id",
        old.session_id,
      );
      await preview.getByTestId(`preview-session-tab-${old.session_id}`).click({ button: "right" });
      await desktopTestPage.getByRole("menuitem", { name: "Delete", exact: true }).click();
      const removed = watcher.waitForEvent("session.removed", {
        where: (payload) => payload.session_id === old.session_id,
      });
      await desktopTestPage
        .getByTestId("session-delete-confirm-popover")
        .getByTestId("session-delete-confirm")
        .click();
      await removed;
      await waitForActiveTaskSession(testPage, task.task_id, primary.session_id);
      await waitForTaskSessionsSettled(testPage, task.task_id);
      await dwell(
        testPage,
        1500,
        "negative-assertion",
        "A hidden proven recipient requires explicit reopen even after direct remote deletion",
      );
      await expect(session.sessionTabBySessionId(primary.session_id)).toHaveCount(0);
      await expect(session.activeChat()).toHaveCount(0);
      expect((await sessionReadState(apiClient, primary.session_id)).cursor).toBe(before);
      expect(
        traffic.markReads.filter(
          (read) => read.sessionId === primary.session_id && read.messageId === tail.messageId,
        ),
      ).toEqual([]);
      await expect
        .poll(() => taskUnreadSessionIds(apiClient, seedData.workspaceId, task.task_id))
        .toContain(primary.session_id);
      const taskRow = testPage.getByTestId("sidebar-task-item").filter({ hasText: task.title });
      await expect(taskRow.getByRole("img", { name: TASK_UNREAD_DOT_LABEL })).toHaveCount(1);
      await testPage.screenshot({
        path: test.info().outputPath("remote-hidden-primary-unread.png"),
      });
      await session.addPanelButton().click();
      await session.sessionReopenItem(primary.session_id).click();
      await expect(session.activeChat()).toHaveAttribute("data-session-id", primary.session_id);
      await expect
        .poll(async () => (await sessionReadState(apiClient, primary.session_id)).cursor)
        .toBe(tail.messageId);
      await expect
        .poll(() => taskUnreadSessionIds(apiClient, seedData.workspaceId, task.task_id))
        .toEqual([]);
      await expect(taskRow.getByRole("img", { name: TASK_UNREAD_DOT_LABEL })).toHaveCount(0);
      expect(await testPage.evaluate(() => performance.timeOrigin)).toBe(documentStart);
    } finally {
      await apiClient.saveUserSettings({ enable_preview_on_click: previewEnabled });
    }
  });

  // @covers AC-TASKS-TASK-UNREAD-DOT-001.6, AC-TASKS-TASK-UNREAD-DOT-001.11
  test("keeps a terminal passthrough fallback unread until explicit selection", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { task, old, primary } = await seedRemovalChoiceTask(apiClient, seedData);
    const fallback = await apiClient.seedAutomationTaskSession(task.task_id, "COMPLETED", false, {
      isPassthrough: true,
    });
    await apiClient.setPrimarySession(old.session_id);
    const proof = await holdRemovalMembership(testPage, task.task_id, old.session_id);
    const watcher = watchWs(testPage);
    const traffic = observeReadTraffic(testPage);
    const session = new SessionPage(testPage);
    try {
      await testPage.goto(`/t/${task.task_id}`);
      await expect(session.activeChat()).toHaveAttribute("data-session-id", old.session_id);
      await expect(session.sessionTabBySessionId(fallback.id)).toBeVisible();
      expect((await sessionReadState(apiClient, fallback.id)).cursor).toBeNull();
      const promoted = watcher.waitForEvent("task.updated", {
        where: (payload) =>
          payload.task_id === task.task_id && payload.primary_session_id === primary.session_id,
      });
      await apiClient.setPrimarySession(primary.session_id);
      await promoted;
      proof.arm();
      await session.sessionTabBySessionId(old.session_id).click({ button: "right" });
      await session.contextMenuItem("Delete").click();
      const acknowledged = watcher.waitForResponse("session.delete");
      await testPage.getByTestId("session-delete-confirm").click();
      await acknowledged;
      await proof.queued;
      await waitForActiveTaskSession(testPage, task.task_id, null);
      const tail = await apiClient.seedSessionMessage(fallback.id, {
        type: "message",
        content: "Terminal passthrough output delivered behind an owned gap",
      });
      await dwell(
        testPage,
        1500,
        "negative-assertion",
        "A terminal passthrough recovery transcript cannot acknowledge its hidden fallback",
      );
      expect((await sessionReadState(apiClient, fallback.id)).cursor).toBeNull();
      expect(
        traffic.markReads.filter(
          (read) => read.sessionId === fallback.id && read.messageId === tail.messageId,
        ),
      ).toEqual([]);
      proof.release();
      await waitForActiveTaskSession(testPage, task.task_id, primary.session_id);
      await session.sessionTabBySessionId(fallback.id).click();
      await expect(session.activeChat()).toHaveAttribute("data-session-id", fallback.id);
      await expect
        .poll(async () => (await sessionReadState(apiClient, fallback.id)).cursor)
        .toBe(tail.messageId);
      await new KanbanPage(testPage).goto();
      await apiClient.saveUserSettings({ enable_preview_on_click: true });
      const previewTail = await apiClient.seedSessionMessage(fallback.id, {
        type: "message",
        content: "Passthrough preview is not a read acknowledgement",
      });
      await testPage.goto(`/?taskId=${task.task_id}&sessionId=${fallback.id}`);
      const preview = testPage.getByTestId("task-preview-panel");
      await expect(preview.locator('[data-testid="session-chat"]:visible')).toHaveAttribute(
        "data-session-id",
        fallback.id,
      );
      await expect(
        preview.getByText("Passthrough preview is not a read acknowledgement", { exact: true }),
      ).toBeVisible();
      await dwell(
        testPage,
        1500,
        "negative-assertion",
        "Preview must preserve its read-only policy for a terminal passthrough transcript",
      );
      expect((await sessionReadState(apiClient, fallback.id)).cursor).toBe(tail.messageId);
      await testPage.goto(`/t/${task.task_id}`);
      await expect(session.activeChat()).toHaveAttribute("data-session-id", primary.session_id);
      await session.sessionTabBySessionId(fallback.id).click();
      await expect
        .poll(async () => (await sessionReadState(apiClient, fallback.id)).cursor)
        .toBe(previewTail.messageId);
      await testPage.screenshot({ path: test.info().outputPath("passthrough-explicit-read.png") });
    } finally {
      await proof.dispose();
    }
  });
});
