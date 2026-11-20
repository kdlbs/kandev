import { randomUUID } from "node:crypto";
import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { SessionPage } from "../pages/session-page";
import { watchWs } from "./causal-waits";
import type { AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";

export const REMOVAL_PRIMARY_TEXT = "Proven primary conversation";
export const REMOVAL_OTHER_TEXT = "Unproven neighboring conversation";

export async function seedRemovalChoiceTask(
  api: ApiClient,
  seed: SeedData,
  prepareEnvironment = false,
) {
  const task = prepareEnvironment
    ? await api.createTask(seed.workspaceId, "Owned removal choice", {
        prepare_session: true,
        agent_profile_id: seed.agentProfileId,
        workflow_id: seed.workflowId,
        workflow_step_id: seed.startStepId,
        repository_ids: [seed.repositoryId],
      })
    : await api.seedTask(seed.workspaceId, "Owned removal choice", {
        workflow_id: seed.workflowId,
        workflow_step_id: seed.startStepId,
      });
  const taskId = "task_id" in task ? task.task_id : task.id;
  const preparedId = "session_id" in task ? task.session_id : undefined;
  if (prepareEnvironment && !preparedId) throw new Error("Removal fixture has no prepared session");
  const now = Date.now();
  const old = await api.seedTaskSession(taskId, {
    sessionId: preparedId ?? randomUUID(),
    agentProfileId: seed.agentProfileId,
    state: "FAILED",
    startedAt: new Date(now - 1000).toISOString(),
    completedAt: new Date(now - 500).toISOString(),
    errorMessage: "Removed conversation",
  });
  const primary = await api.seedTaskSession(taskId, {
    sessionId: randomUUID(),
    agentProfileId: seed.agentProfileId,
    state: "WAITING_FOR_INPUT",
    startedAt: new Date(now - 4000).toISOString(),
  });
  const other = await api.seedTaskSession(taskId, {
    sessionId: randomUUID(),
    agentProfileId: seed.agentProfileId,
    state: "WAITING_FOR_INPUT",
    startedAt: new Date(now - 2000).toISOString(),
  });
  await api.seedSessionMessage(old.session_id, { type: "error", content: "Removed conversation" });
  await api.seedSessionMessage(primary.session_id, {
    type: "message",
    content: REMOVAL_PRIMARY_TEXT,
  });
  await api.seedSessionMessage(other.session_id, { type: "message", content: REMOVAL_OTHER_TEXT });
  await api.setPrimarySession(primary.session_id);
  return { task: { task_id: taskId, title: task.title }, old, primary, other };
}

function membershipContains(body: unknown, sessionId: string): boolean {
  if (!body || typeof body !== "object" || !("sessions" in body) || !Array.isArray(body.sessions))
    throw new Error("Expected a complete session membership response");
  const rows: unknown[] = body.sessions;
  return rows.some((row) => {
    if (!row || typeof row !== "object" || !("id" in row) || typeof row.id !== "string")
      throw new Error("Expected a stored session identity");
    return row.id === sessionId;
  });
}

/** Hold authentic task responses without fabricating identity or membership. */
async function holdTaskResponse(
  page: Page,
  pattern: string,
  shouldHold: (body: unknown) => boolean,
) {
  let armed = false;
  let released = false;
  const gate = Promise.withResolvers<void>();
  const queued = Promise.withResolvers<void>();
  const running = new Set<Promise<void>>();
  const handle = async (route: import("@playwright/test").Route) => {
    const response = await route.fetch();
    const body: unknown = await response.json();
    if (armed && !released && shouldHold(body)) {
      queued.resolve();
      await gate.promise;
    }
    await route.fulfill({ response });
  };
  const handler = (route: import("@playwright/test").Route) => {
    const work = handle(route).finally(() => running.delete(work));
    running.add(work);
    return work;
  };
  await page.route(pattern, handler);
  return {
    arm: () => {
      armed = true;
    },
    queued: queued.promise,
    release: () => {
      released = true;
      gate.resolve();
    },
    dispose: async () => {
      released = true;
      gate.resolve();
      while (running.size) await Promise.all([...running]);
      await page.unroute(pattern, handler);
    },
  };
}

export function holdRemovalMembership(page: Page, taskId: string, removedId: string) {
  return holdTaskResponse(
    page,
    `**/api/v1/tasks/${taskId}/sessions*`,
    (body) => !membershipContains(body, removedId),
  );
}

export async function holdInitialMembership(page: Page, taskId: string, staleSessionId?: string) {
  const gate = await holdTaskResponse(
    page,
    `**/api/v1/tasks/${taskId}/sessions*`,
    (body) => !staleSessionId || membershipContains(body, staleSessionId),
  );
  gate.arm();
  return gate;
}

export async function holdInitialTaskIdentity(page: Page, taskId: string, previousPrimary: string) {
  const gate = await holdTaskResponse(page, `**/api/v1/tasks/${taskId}`, (body) => {
    if (!body || typeof body !== "object" || !("primary_session_id" in body))
      throw new Error("Expected a stored task identity");
    return body.primary_session_id === previousPrimary;
  });
  gate.arm();
  return gate;
}

/** Hold native restore callbacks after the predecessor is parked, without changing app state. */
export async function holdParkedLayoutFrames(page: Page, sessionId: string) {
  await page.evaluate((id) => {
    const bridge = window as Window & {
      __dockviewApi__?: { getPanel: (id: string) => unknown };
      __layoutFrameGate__?: { release: () => void; count: () => number };
    };
    const api = bridge.__dockviewApi__;
    if (!api?.getPanel(`session:${id}`)) throw new Error("Visible predecessor missing");
    const request = window.requestAnimationFrame.bind(window);
    const pending: Array<() => void> = [];
    window.requestAnimationFrame = (callback) =>
      request((time) => {
        if (!api.getPanel(`session:${id}`)) pending.push(() => callback(time));
        else callback(time);
      });
    bridge.__layoutFrameGate__ = {
      count: () => pending.length,
      release: () => {
        window.requestAnimationFrame = request;
        bridge.__layoutFrameGate__ = undefined;
        for (const callback of pending.splice(0)) callback();
      },
    };
  }, sessionId);
  const release = () =>
    page.evaluate(() => {
      (
        window as Window & { __layoutFrameGate__?: { release: () => void } }
      ).__layoutFrameGate__?.release();
    });
  return {
    count: () =>
      page.evaluate(
        () =>
          (
            window as Window & { __layoutFrameGate__?: { count: () => number } }
          ).__layoutFrameGate__?.count() ?? 0,
      ),
    release,
    dispose: release,
  };
}

type SelectionSample = { sessionId: string | null; pinnedSessionId: string | null };
type RemovalWindow = Window & {
  __KANDEV_E2E_STORE__?: StoreApi<AppState>;
  __KANDEV_E2E_REMOVAL_SELECTION__?: { samples: SelectionSample[]; stop: () => void };
};

/** Observe real selection publications, including a wrong transient pin. */
export async function observeRemovalSelection(page: Page) {
  await page.evaluate(() => {
    const bridge = window as RemovalWindow;
    const store = bridge.__KANDEV_E2E_STORE__;
    if (!store) throw new Error("E2E store bridge missing");
    const sample = (state: AppState): SelectionSample => ({
      sessionId: state.tasks.activeSessionId,
      pinnedSessionId: state.tasks.pinnedSessionId,
    });
    const samples = [sample(store.getState())];
    const stop = store.subscribe((state, previous) => {
      if (state.tasks !== previous.tasks) samples.push(sample(state));
    });
    bridge.__KANDEV_E2E_REMOVAL_SELECTION__ = { samples, stop };
  });
  return {
    read: () =>
      page.evaluate(() => {
        const bridge = window as RemovalWindow;
        return bridge.__KANDEV_E2E_REMOVAL_SELECTION__?.samples ?? [];
      }),
    snapshot: () =>
      page.evaluate(() => {
        const store = (window as RemovalWindow).__KANDEV_E2E_STORE__;
        if (!store) throw new Error("E2E store bridge missing");
        const state = store.getState();
        return {
          navigationRevision: state.taskRemoval.navigationRevision,
          workflowRequest: state.workflowSessionFocus.request,
        };
      }),
    stop: () =>
      page.evaluate(() => {
        const bridge = window as RemovalWindow;
        bridge.__KANDEV_E2E_REMOVAL_SELECTION__?.stop();
        delete bridge.__KANDEV_E2E_REMOVAL_SELECTION__;
      }),
  };
}

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

/** Delay authentic delete dispatch/ACK while forwarding all unrelated frames. */
export async function holdDeletionTraffic(page: Page, sessionId: string) {
  const requestQueued = Promise.withResolvers<void>();
  const acknowledgementQueued = Promise.withResolvers<void>();
  const requests: Array<() => void> = [];
  const acknowledgements: Array<() => void> = [];
  let requestId: string | undefined;
  let dispatchReleased = false;
  let acknowledgementReleased = false;
  await page.routeWebSocket(/\/ws(?:\?|$)/, (socket) => {
    const upstream = socket.connectToServer();
    socket.onMessage((data) => {
      const frame = JSON.parse(data.toString());
      if (frame.action === "session.delete" && frame.payload?.session_id === sessionId) {
        requestId = frame.id;
        if (!dispatchReleased) {
          requests.push(() => upstream.send(data));
          requestQueued.resolve();
          return;
        }
      }
      upstream.send(data);
    });
    upstream.onMessage((data) => {
      const frame = JSON.parse(data.toString());
      if (requestId && frame.id === requestId && !acknowledgementReleased) {
        acknowledgements.push(() => socket.send(data));
        acknowledgementQueued.resolve();
        return;
      }
      socket.send(data);
    });
  });
  return {
    requestQueued: requestQueued.promise,
    acknowledgementQueued: acknowledgementQueued.promise,
    dispatch: () => {
      dispatchReleased = true;
      for (const send of requests.splice(0)) send();
    },
    acknowledge: () => {
      acknowledgementReleased = true;
      for (const send of acknowledgements.splice(0)) send();
    },
  };
}
