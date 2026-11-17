import { expect, test } from "../fixtures/test-base";
import type { Page } from "@playwright/test";
import type { AppState } from "../../lib/state/store";

async function waitForStorageWarning(page: Page, sessionId: string) {
  await page.waitForFunction((id) => {
    const state = (
      window as Window & { __KANDEV_E2E_STORE__?: { getState: () => AppState } }
    ).__KANDEV_E2E_STORE__?.getState();
    return (
      state?.tasks.activeSessionId === id &&
      state.taskSessions.items[id]?.runtime_storage_health?.state === "warning"
    );
  }, sessionId);
}

// @covers AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-004.1 through 004.4
export function storageNoticeDismissalScenario() {
  test("dismisses Storage per task across reload and task switching", async ({
    testPage: page,
    apiClient,
    seedData,
    prCapture,
  }, testInfo) => {
    const tasks: { task: { id: string }; session: { session_id: string } }[] = [];
    for (const title of ["Dismiss storage", "Other storage task"]) {
      const task = await apiClient.createTask(seedData.workspaceId, title, {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        agent_profile_id: seedData.agentProfileId,
        repository_ids: [seedData.repositoryId],
      });
      const session = await apiClient.seedTaskSession(task.id, {
        state: "WAITING_FOR_INPUT",
        agentProfileId: seedData.agentProfileId,
      });
      tasks.push({ task, session });
    }
    let notify: (payload: Record<string, unknown>) => void;
    await page.routeWebSocket(/\/ws$/, (socket) => {
      const server = socket.connectToServer();
      notify = (payload) =>
        socket.send(
          JSON.stringify({
            id: "storage-dismissal-fixture",
            type: "notification",
            action: "task_session.runtime_storage_changed",
            payload,
          }),
        );
      socket.onMessage((data) => server.send(data));
      server.onMessage((data) => socket.send(data));
    });
    // Supply health on every detail response, including reloads and task switches.
    await page.route("**/api/v1/task-sessions/*", async (route) => {
      const response = await route.fetch();
      const match = tasks.find(({ session }) => route.request().url().endsWith(session.session_id));
      if (!match || !response.ok()) return route.fulfill({ response });
      const body = await response.json();
      const session = body.session ?? body;
      session.runtime_storage_health = {
        state: "warning",
        location: "local_runtime",
        storage_kind: "journal",
        total_bytes: 1000,
        available_bytes: 100,
        epoch: 1,
        revision: 1,
        observed_at: new Date().toISOString(),
      };
      await route.fulfill({ response, json: body });
    });
    await page.goto(`/t/${tasks[0].task.id}`);
    const notice = page.getByTestId("session-delivery-storage-notice");
    await expect(notice).toBeVisible();
    const editor = page.getByTestId("chat-input-editor");
    await editor.fill("Preserve my draft");
    const dismiss = notice.getByRole("button", { name: "Dismiss", exact: true });
    const mobile = testInfo.project.name === "mobile-chrome";
    for (const width of mobile ? [393] : [767, 768, 1280]) {
      await page.setViewportSize({ width, height: 900 });
      const box = (await dismiss.boundingBox())!;
      const row = (await notice.boundingBox())!;
      if (mobile || width < 768) {
        expect(box.width).toBeGreaterThanOrEqual(44);
        expect(box.height).toBeGreaterThanOrEqual(44);
      }
      expect(box.x + box.width).toBeLessThanOrEqual(row.x + row.width);
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      ).toBe(true);
    }
    await prCapture.screenshot(`storage-dismiss-${testInfo.project.name}`, {
      caption: "Storage row with a visible dismiss control.",
    });
    if (mobile) await dismiss.tap();
    else {
      await notice.getByTestId("session-delivery-storage-details").click();
      await page.keyboard.press("Tab");
      await expect(dismiss).toBeFocused();
      await page.keyboard.press("Enter");
    }
    await expect(notice).toBeHidden();
    await expect(editor).toHaveText("Preserve my draft");
    await page.reload();
    await waitForStorageWarning(page, tasks[0].session.session_id);
    await expect(editor).toBeVisible();
    await expect(notice).toBeHidden();
    await page.goto(`/t/${tasks[1].task.id}`);
    await expect(notice).toBeVisible();
    await page.goto(`/t/${tasks[0].task.id}`);
    await waitForStorageWarning(page, tasks[0].session.session_id);
    await expect(editor).toBeVisible();
    await expect(notice).toBeHidden();

    const quick = await apiClient.startQuickChat(
      seedData.workspaceId,
      seedData.agentProfileId,
      "Quick storage dismissal",
    );
    tasks.push({ task: { id: quick.task_id }, session: { session_id: quick.session_id } });
    await page.goto("/");
    await page.keyboard.press(`${process.platform === "darwin" ? "Meta" : "Control"}+Shift+q`);
    const dialog = page.getByRole("dialog", { name: "Quick Chat", exact: true });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByTestId("chat-input-editor")).toBeVisible();
    notify!({
      task_id: quick.task_id,
      session_id: quick.session_id,
      state: "warning",
      location: "executor_runtime",
      executor_label: "Remote storage executor with a long name",
      storage_kind: "workspace",
      total_bytes: 1000,
      available_bytes: 100,
      epoch: Number.MAX_SAFE_INTEGER,
      revision: 1,
      observed_at: new Date().toISOString(),
    });
    const quickNotice = dialog.getByTestId("session-delivery-storage-notice");
    await expect(quickNotice).toBeVisible();
    const quickDismiss = quickNotice.getByRole("button", { name: "Dismiss", exact: true });
    const quickBox = (await quickDismiss.boundingBox())!;
    const quickRow = (await quickNotice.boundingBox())!;
    expect(quickBox.x + quickBox.width).toBeLessThanOrEqual(quickRow.x + quickRow.width);
    if (mobile) {
      expect(quickBox.width).toBeGreaterThanOrEqual(44);
      expect(quickBox.height).toBeGreaterThanOrEqual(44);
    }
    await prCapture.screenshot(`quick-storage-dismiss-${testInfo.project.name}`, {
      caption: "Quick Chat remote storage notice with a long executor label and dismissal.",
    });
    await quickDismiss.click();
    await expect(quickNotice).toBeHidden();
  });
}
