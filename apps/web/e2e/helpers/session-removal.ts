import { randomUUID } from "node:crypto";
import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { SessionPage } from "../pages/session-page";
import { watchWs } from "./causal-waits";

export async function seedStoppedRemovalTask(api: ApiClient, seed: SeedData) {
  const task = await api.seedTask(seed.workspaceId, "Stored session removal", {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
  });
  const old = await api.seedTaskSession(task.task_id, {
    sessionId: randomUUID(),
    agentProfileId: seed.agentProfileId,
    state: "FAILED",
    completedAt: new Date().toISOString(),
    errorMessage: "Last stored conversation",
  });
  await api.seedSessionMessage(old.session_id, {
    type: "error",
    content: "Last stored conversation",
  });
  await api.setPrimarySession(old.session_id);
  return { task, old };
}

export async function expectLastStoppedSessionRemoval(
  observerPage: Page,
  deletingPage: Page,
  api: ApiClient,
  seed: SeedData,
): Promise<void> {
  const { task, old } = await seedStoppedRemovalTask(api, seed);
  const observer = new SessionPage(observerPage);
  const deletingClient = new SessionPage(deletingPage);
  const watcher = watchWs(observerPage);
  await observerPage.goto(`/t/${task.task_id}`);
  await expect(observer.activeChat()).toHaveAttribute("data-session-id", old.session_id);
  await deletingPage.goto(`/t/${task.task_id}`);
  await expect(deletingClient.activeChat()).toHaveAttribute("data-session-id", old.session_id);
  const before = await api.listTaskSessions(task.task_id);
  expect(before.sessions.map((row) => ({ id: row.id, state: row.state }))).toEqual([
    { id: old.session_id, state: "FAILED" },
  ]);
  const documentStart = await observerPage.evaluate(() => performance.timeOrigin);
  const removed = watcher.waitForEvent("session.removed", {
    where: (payload) => payload.session_id === old.session_id,
  });
  await deletingClient.sessionTabBySessionId(old.session_id).click({ button: "right" });
  await deletingClient.contextMenuItem("Delete").click();
  const confirmation = deletingPage.getByTestId("session-delete-confirm-popover");
  await expect(confirmation).toBeVisible();
  await confirmation.getByTestId("session-delete-confirm").click();
  await removed;
  await expect.poll(async () => (await api.listTaskSessions(task.task_id)).sessions).toEqual([]);
  await expect(
    observerPage.locator(`[data-testid="session-chat"][data-session-id="${old.session_id}"]`),
  ).toHaveCount(0);
  await expect(observer.sessionTabBySessionId(old.session_id)).toHaveCount(0);
  await expect(observerPage.getByText("Last stored conversation", { exact: true })).toHaveCount(0);
  expect(await observerPage.evaluate(() => performance.timeOrigin)).toBe(documentStart);
}

async function holdPromotion(page: Page, taskId: string, removedId: string, targetId: string) {
  let released = false;
  let removed = false;
  let zeroPrimaryReads = 0;
  const held: Array<() => void> = [];
  await page.routeWebSocket(/\/ws(?:\?|$)/, (socket) => {
    const upstream = socket.connectToServer();
    socket.onMessage((data) => upstream.send(data));
    upstream.onMessage((data) => {
      const frame = JSON.parse(data.toString());
      if (frame.action === "session.removed" && frame.payload?.session_id === removedId) {
        removed = true;
      }
      if (
        !released &&
        frame.payload?.task_id === taskId &&
        frame.payload?.primary_session_id === targetId
      ) {
        held.push(() => socket.send(data));
      } else {
        socket.send(data);
      }
    });
  });
  await page.route(`**/api/v1/tasks/${taskId}/sessions*`, async (route) => {
    const response = await route.fetch();
    const body = await response.json();
    if (
      !released &&
      removed &&
      !body.sessions.some((row: { id: string }) => row.id === removedId)
    ) {
      zeroPrimaryReads += 1;
      await route.fulfill({
        response,
        json: {
          ...body,
          sessions: body.sessions.map((row: object) => ({ ...row, is_primary: false })),
        },
      });
    } else {
      await route.fulfill({ response });
    }
  });
  return {
    reads: () => zeroPrimaryReads,
    held: () => held.length,
    release: () => {
      released = true;
      for (const send of held.splice(0)) send();
    },
  };
}

export async function expectLatePromotionAfterRemoval(
  observerPage: Page,
  deletingPage: Page,
  api: ApiClient,
  seed: SeedData,
  requireEmptyView = false,
): Promise<void> {
  const task = await api.seedTask(seed.workspaceId, "Remote delayed primary promotion", {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
  });
  const old = await api.seedTaskSession(task.task_id, {
    sessionId: randomUUID(),
    agentProfileId: seed.agentProfileId,
    state: "FAILED",
    completedAt: new Date().toISOString(),
    errorMessage: "Stored failure before handoff",
  });
  const next = await api.seedTaskSession(task.task_id, {
    sessionId: randomUUID(),
    agentProfileId: seed.agentProfileId,
    state: "WAITING_FOR_INPUT",
  });
  const text = "Promoted surviving conversation";
  await api.seedSessionMessage(next.session_id, { type: "message", content: text });
  await api.setPrimarySession(old.session_id);
  const gate = await holdPromotion(observerPage, task.task_id, old.session_id, next.session_id);
  try {
    const observer = new SessionPage(observerPage);
    const deletingClient = new SessionPage(deletingPage);
    await observerPage.goto(`/t/${task.task_id}`);
    await expect(observer.activeChat()).toHaveAttribute("data-session-id", old.session_id);
    await deletingPage.goto(`/t/${task.task_id}`);
    await expect(deletingClient.activeChat()).toHaveAttribute("data-session-id", old.session_id);
    const documentStart = await observerPage.evaluate(() => performance.timeOrigin);
    await deletingClient.sessionTabBySessionId(old.session_id).click({ button: "right" });
    await deletingClient.contextMenuItem("Delete").click();
    const confirmation = deletingPage.getByTestId("session-delete-confirm-popover");
    await expect(confirmation).toBeVisible();
    await confirmation.getByTestId("session-delete-confirm").click();
    await expect.poll(gate.reads).toBeGreaterThan(0);
    await expect(
      observerPage.locator(`[data-testid="session-chat"][data-session-id="${old.session_id}"]`),
    ).toHaveCount(0);
    await expect.poll(gate.held).toBeGreaterThan(0);
    expect(
      (await api.listTaskSessions(task.task_id)).sessions.map((row) => ({
        id: row.id,
        primary: row.is_primary,
      })),
    ).toEqual([{ id: next.session_id, primary: true }]);
    if (requireEmptyView) {
      await expect(
        observerPage.locator('[data-testid="session-chat"][data-session-id]:visible'),
      ).toHaveCount(0);
    }
    gate.release();
    await expect(observer.activeChat()).toHaveAttribute("data-session-id", next.session_id);
    await expect(observer.activeChat().getByText(text, { exact: true })).toBeVisible();
    await expect(observer.sessionTabBySessionId(old.session_id)).toHaveCount(0);
    expect(await observerPage.evaluate(() => performance.timeOrigin)).toBe(documentStart);
  } finally {
    gate.release();
  }
}
