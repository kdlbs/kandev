import { test, expect } from "../../fixtures/test-base";
import type { APIResponse, Route } from "@playwright/test";
import { KanbanPage } from "../../pages/kanban-page";
import { watchWs } from "../../helpers/causal-waits";
import {
  holdDeletionTraffic,
  holdRemovalMembership,
  observeRemovalSelection,
  REMOVAL_OTHER_TEXT,
  REMOVAL_PRIMARY_TEXT,
  seedRemovalChoiceTask,
} from "../../helpers/session-removal";
import { waitForActiveTaskSession, waitForTaskSessionsSettled } from "../../helpers/session-store";
import {
  createWorkflowSessionFocusScenario,
  waitForNewWorkflowSession,
} from "../workflow/workflow-session-focus-helpers";

test.describe("Preview owned deletion", () => {
  test.describe.configure({ timeout: 90_000 });
  let baseline:
    | {
        enable_preview_on_click: boolean;
        prevent_auto_start_agent_on_open: boolean;
      }
    | undefined;
  test.beforeEach(async ({ testPage, apiClient }) => {
    baseline = undefined;
    await new KanbanPage(testPage).goto();
    const { settings } = await apiClient.getUserSettings();
    baseline = {
      enable_preview_on_click: settings.enable_preview_on_click === true,
      prevent_auto_start_agent_on_open: settings.prevent_auto_start_agent_on_open === true,
    };
    await apiClient.saveUserSettings({
      enable_preview_on_click: true,
      prevent_auto_start_agent_on_open: true,
    });
  });
  test.afterEach(async ({ apiClient }) => {
    if (baseline) await apiClient.saveUserSettings(baseline);
  });

  for (const winner of ["proof", "explicit-tab", "plan"] as const) {
    // @covers AC-TASKS-TASK-UNREAD-DOT-001.6
    // @covers AC-TASKS-TASK-UNREAD-DOT-001.11
    test(`keeps the Preview deletion owned by ${winner}`, async ({
      testPage,
      apiClient,
      seedData,
    }) => {
      const { task, old, other, primary } = await seedRemovalChoiceTask(apiClient, seedData);
      const proof = await holdRemovalMembership(testPage, task.task_id, old.session_id);
      const watcher = watchWs(testPage);
      await testPage.goto(`/?taskId=${task.task_id}&sessionId=${old.session_id}`);
      const preview = testPage.getByTestId("task-preview-panel");
      const chat = preview.locator('[data-testid="session-chat"]:visible');
      await expect(chat).toHaveAttribute("data-session-id", old.session_id);
      await waitForTaskSessionsSettled(testPage, task.task_id);
      const history = await observeRemovalSelection(testPage);
      const documentStart = await testPage.evaluate(() => performance.timeOrigin);
      try {
        proof.arm();
        await preview
          .getByTestId(`preview-session-tab-${old.session_id}`)
          .click({ button: "right" });
        await testPage.getByRole("menuitem", { name: "Delete", exact: true }).click();
        const confirmation = testPage.getByTestId("session-delete-confirm-popover");
        await expect(confirmation).toBeVisible();
        const acknowledged = watcher.waitForResponse("session.delete");
        await confirmation.getByTestId("session-delete-confirm").click();
        await acknowledged;
        await proof.queued;
        const stored = await apiClient.listTaskSessions(task.task_id);
        expect(stored.sessions.filter((row) => row.is_primary).map((row) => row.id)).toEqual([
          primary.session_id,
        ]);
        expect(stored.sessions.some((row) => row.id === old.session_id)).toBe(false);
        if (winner === "proof") {
          const samples = await history.read();
          expect(samples.map((sample) => sample.sessionId)).not.toContain(other.session_id);
          expect(samples.map((sample) => sample.pinnedSessionId)).not.toContain(other.session_id);
          expect(new URL(testPage.url()).searchParams.get("sessionId")).toBeNull();
          await expect(chat).toHaveCount(0);
        } else if (winner === "explicit-tab") {
          await preview.getByTestId(`preview-session-tab-${other.session_id}`).click();
          await expect(chat).toHaveAttribute("data-session-id", other.session_id);
        } else {
          await preview.getByTestId("preview-plan-tab").click();
          await expect(preview.getByTestId("preview-plan-tab")).toHaveAttribute(
            "data-state",
            "active",
          );
        }
        proof.release();
        if (winner === "plan") {
          await waitForActiveTaskSession(testPage, task.task_id, primary.session_id);
          await waitForTaskSessionsSettled(testPage, task.task_id);
          await expect(preview.getByTestId("preview-plan-tab")).toHaveAttribute(
            "data-state",
            "active",
          );
        } else {
          const expected = winner === "proof" ? primary.session_id : other.session_id;
          await expect(chat).toHaveAttribute("data-session-id", expected);
          await expect(
            chat.getByText(winner === "proof" ? REMOVAL_PRIMARY_TEXT : REMOVAL_OTHER_TEXT, {
              exact: true,
            }),
          ).toBeVisible();
          await expect
            .poll(() => new URL(testPage.url()).searchParams.get("sessionId"))
            .toBe(expected);
          if (winner === "proof") {
            const samples = await history.read();
            expect(samples.map((sample) => sample.sessionId)).not.toContain(other.session_id);
            expect(samples.map((sample) => sample.pinnedSessionId)).not.toContain(other.session_id);
          }
        }
        expect(await testPage.evaluate(() => performance.timeOrigin)).toBe(documentStart);
        await testPage.screenshot({
          path: test.info().outputPath(`preview-removal-${winner}.png`),
        });
      } finally {
        await proof.dispose();
        await history.stop();
      }
    });
  }

  // @covers AC-TASKS-TASK-UNREAD-DOT-001.6
  // @covers AC-TASKS-TASK-UNREAD-DOT-001.11
  test("keeps a newer explicit workflow move across the held deletion proof", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const scenario = await createWorkflowSessionFocusScenario(apiClient, seedData, "new");
    const { settings } = await apiClient.getUserSettings();
    const oldFilter = settings.workflow_filter_id ?? null;
    const proof = await holdRemovalMembership(
      testPage,
      scenario.task.id,
      scenario.secondarySessionId,
    );
    const moveUrl = `**/api/v1/tasks/${scenario.task.id}/move`;
    const moveRelease = Promise.withResolvers<void>();
    const backendMove = Promise.withResolvers<APIResponse>();
    let moving: Promise<void> | undefined;
    const holdMove = (route: Route) => {
      moving = (async () => {
        const response = await route.fetch();
        backendMove.resolve(response);
        await moveRelease.promise;
        await route.fulfill({ response });
      })();
      return moving;
    };
    await testPage.route(moveUrl, holdMove);
    const watcher = watchWs(testPage);
    let history: Awaited<ReturnType<typeof observeRemovalSelection>> | undefined;
    try {
      await apiClient.saveUserSettings({ workflow_filter_id: scenario.workflow.id });
      await testPage.goto(`/?taskId=${scenario.task.id}&sessionId=${scenario.secondarySessionId}`);
      const preview = testPage.getByTestId("task-preview-panel");
      const chat = preview.locator('[data-testid="session-chat"]:visible');
      await expect(chat).toHaveAttribute("data-session-id", scenario.secondarySessionId);
      await waitForTaskSessionsSettled(testPage, scenario.task.id);
      history = await observeRemovalSelection(testPage);
      proof.arm();
      await preview
        .getByTestId(`preview-session-tab-${scenario.secondarySessionId}`)
        .click({ button: "right" });
      await testPage.getByRole("menuitem", { name: "Delete", exact: true }).click();
      const acknowledged = watcher.waitForResponse("session.delete");
      await testPage.getByTestId("session-delete-confirm").click();
      await acknowledged;
      await proof.queued;
      await preview.getByTestId("workflow-stepper-minimal").hover();
      await testPage
        .getByTestId(`workflow-step-disclosure-move-${scenario.destination.id}`)
        .click();
      const committed = await backendMove.promise;
      expect(committed.ok()).toBe(true);
      expect((await committed.json()).workflow_entry_identity).toMatch(/^entry:/);
      proof.release();
      await waitForTaskSessionsSettled(testPage, scenario.task.id);
      expect((await history.read()).map((sample) => sample.sessionId)).not.toContain(
        scenario.sourceSessionId,
      );
      moveRelease.resolve();
      const destination = await waitForNewWorkflowSession(apiClient, scenario.task.id, [
        scenario.sourceSessionId,
        scenario.secondarySessionId,
      ]);
      await expect(chat).toHaveAttribute("data-session-id", destination);
      await expect
        .poll(() => new URL(testPage.url()).searchParams.get("sessionId"))
        .toBe(destination);
      expect(
        (await apiClient.getTask(scenario.task.id)).metadata?.workflow_session_route?.phase,
      ).toBe("committed");
      await testPage.screenshot({ path: test.info().outputPath("preview-removal-workflow.png") });
    } finally {
      proof.release();
      moveRelease.resolve();
      await moving;
      await testPage.unroute(moveUrl, holdMove);
      await proof.dispose();
      await history?.stop();
      await apiClient.saveUserSettings({ workflow_filter_id: oldFilter });
    }
  });

  // @covers AC-TASKS-TASK-UNREAD-DOT-001.11
  test("keeps the removed Preview tab selected after dispatch under its current proof", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { task, old, primary, other } = await seedRemovalChoiceTask(apiClient, seedData);
    const deletion = await holdDeletionTraffic(testPage, old.session_id);
    const proof = await holdRemovalMembership(testPage, task.task_id, old.session_id);
    await testPage.goto(`/?taskId=${task.task_id}&sessionId=${other.session_id}`);
    const preview = testPage.getByTestId("task-preview-panel");
    const chat = preview.locator('[data-testid="session-chat"]:visible');
    await expect(chat).toHaveAttribute("data-session-id", other.session_id);
    await waitForTaskSessionsSettled(testPage, task.task_id);
    const history = await observeRemovalSelection(testPage);
    const documentStart = await testPage.evaluate(() => performance.timeOrigin);
    try {
      proof.arm();
      await preview.getByTestId(`preview-session-tab-${old.session_id}`).click({ button: "right" });
      await testPage.getByRole("menuitem", { name: "Delete", exact: true }).click();
      await testPage.getByTestId("session-delete-confirm").click();
      await deletion.requestQueued;
      await preview.getByTestId(`preview-session-tab-${old.session_id}`).click();
      await expect(chat).toHaveAttribute("data-session-id", old.session_id);
      const beforeCommit = (await history.read()).length;
      deletion.dispatch();
      await deletion.acknowledgementQueued;
      deletion.acknowledge();
      await proof.queued;
      await expect(chat).toHaveCount(0);
      expect(
        (await apiClient.listTaskSessions(task.task_id)).sessions.some(
          (row) => row.id === old.session_id,
        ),
      ).toBe(false);
      proof.release();
      await expect(chat).toHaveAttribute("data-session-id", primary.session_id);
      await expect(chat.getByText(REMOVAL_PRIMARY_TEXT, { exact: true })).toBeVisible();
      await expect
        .poll(() => new URL(testPage.url()).searchParams.get("sessionId"))
        .toBe(primary.session_id);
      expect(
        (await history.read()).slice(beforeCommit).map((sample) => sample.sessionId),
      ).not.toContain(other.session_id);
      expect(await testPage.evaluate(() => performance.timeOrigin)).toBe(documentStart);
    } finally {
      deletion.dispatch();
      deletion.acknowledge();
      await proof.dispose();
      await history.stop();
    }
  });

  // @covers AC-TASKS-TASK-UNREAD-DOT-001.6
  for (const validLink of [false, true]) {
    test(`resolves initial Preview membership (valid non-primary link: ${validLink})`, async ({
      testPage,
      apiClient,
      seedData,
    }) => {
      const { task, old, primary, other } = await seedRemovalChoiceTask(apiClient, seedData);
      await apiClient.wsRequest("session.delete", { session_id: old.session_id });
      const members = (await apiClient.listTaskSessions(task.task_id)).sessions;
      expect(members.some((row) => row.id === old.session_id)).toBe(false);
      expect(members.filter((row) => row.is_primary).map((row) => row.id)).toEqual([
        primary.session_id,
      ]);
      const requested = validLink ? other.session_id : old.session_id;
      await testPage.goto(`/?taskId=${task.task_id}&sessionId=${requested}`);
      const preview = testPage.getByTestId("task-preview-panel");
      await expect(preview).toBeVisible();
      await waitForTaskSessionsSettled(testPage, task.task_id);
      const history = await observeRemovalSelection(testPage);
      try {
        const expected = validLink ? other.session_id : primary.session_id;
        const chat = preview.locator('[data-testid="session-chat"]:visible');
        await expect(chat).toHaveAttribute("data-session-id", expected);
        await expect(
          chat.getByText(validLink ? REMOVAL_OTHER_TEXT : REMOVAL_PRIMARY_TEXT, { exact: true }),
        ).toBeVisible();
        await expect
          .poll(() => new URL(testPage.url()).searchParams.get("sessionId"))
          .toBe(expected);
        expect((await history.read()).map((sample) => sample.pinnedSessionId)).not.toContain(
          old.session_id,
        );
        await testPage.screenshot({
          path: test.info().outputPath(`preview-initial-membership-${validLink}.png`),
        });
      } finally {
        await history.stop();
      }
    });
  }
});
