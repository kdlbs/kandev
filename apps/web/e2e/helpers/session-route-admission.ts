import { randomUUID } from "node:crypto";
import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import type { ConversationSearchResponse } from "../../lib/types/conversation-search";
import { SessionPage } from "../pages/session-page";
import { KanbanPage } from "../pages/kanban-page";
import { dwell } from "./causal-waits";
import { seedRemovalChoiceTask } from "./session-removal";
import { waitForActiveTaskSession } from "./session-store";
import { newestStoredMessageId, observeReadTraffic, sessionReadState } from "./unread-divider";

async function waitForIndexedConversation(
  api: ApiClient,
  workspaceId: string,
  query: string,
  expected: { sessionId: string; messageId: string },
) {
  const params = new URLSearchParams({ query, limit: "20" });
  await expect
    .poll(
      async () => {
        const response = await api.rawRequest(
          "GET",
          `/api/v1/workspaces/${workspaceId}/tasks/conversations?${params}`,
        );
        expect(response.ok).toBe(true);
        const result = (await response.json()) as ConversationSearchResponse;
        return (
          result.state === "ready" &&
          result.tasks.some(
            (task) =>
              task.conversation_match?.session_id === expected.sessionId &&
              task.conversation_match.message_id === expected.messageId,
          )
        );
      },
      { timeout: 30_000, message: "Waiting for the seeded conversation to be indexed" },
    )
    .toBe(true);
}

async function selectCommandTask(page: Page, query: string, conversation = false) {
  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await page.keyboard.press(`${modifier}+k`);
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  await dialog.getByRole("combobox").fill(query);
  const option = dialog.getByRole("option").filter({ hasText: query });
  await expect(option).toHaveCount(1);
  if (conversation) await expect(option).toHaveAttribute("data-conversation", "true");
  if ((page.viewportSize()?.width ?? 1280) < 768) await option.tap();
  else await option.click();
}

export async function expectConversationRouteAfterRemoval(
  page: Page,
  actor: Page,
  api: ApiClient,
  seed: SeedData,
) {
  await api.saveUserSettings({ enable_preview_on_click: true });
  const { task, old, primary, other } = await seedRemovalChoiceTask(api, seed);
  const viewer = new SessionPage(page);
  await page.goto(`/t/${task.task_id}?sessionId=${old.session_id}`);
  await expect(viewer.activeChat()).toHaveAttribute("data-session-id", old.session_id);
  await actor.goto(`/?taskId=${task.task_id}&sessionId=${old.session_id}`);
  const preview = actor.getByTestId("task-preview-panel");
  await expect(preview.locator('[data-testid="session-chat"]:visible')).toHaveAttribute(
    "data-session-id",
    old.session_id,
  );
  await preview.getByTestId(`preview-session-tab-${old.session_id}`).click({ button: "right" });
  await actor.getByRole("menuitem", { name: "Delete", exact: true }).click();
  await actor.getByTestId("session-delete-confirm").click();
  await waitForActiveTaskSession(page, task.task_id, primary.session_id);
  await expect(viewer.activeChat()).toHaveAttribute("data-session-id", primary.session_id);
  const primaryTail = await newestStoredMessageId(api, primary.session_id);
  await expect
    .poll(async () => (await sessionReadState(api, primary.session_id)).cursor)
    .toBe(primaryTail);
  const query = `surviving${randomUUID().replaceAll("-", "")}`;
  const tail = await api.seedSessionMessage(other.session_id, { type: "message", content: query });
  expect((await sessionReadState(api, other.session_id)).cursor).toBeNull();
  await waitForIndexedConversation(api, seed.workspaceId, query, {
    sessionId: other.session_id,
    messageId: tail.messageId,
  });
  const reads = observeReadTraffic(page);
  const started = await page.evaluate(() => performance.timeOrigin);
  await selectCommandTask(page, query, true);
  await expect(page).toHaveURL(new RegExp(`/t/${task.task_id}\\?sessionId=${other.session_id}`));
  await waitForActiveTaskSession(page, task.task_id, other.session_id);
  await expect(viewer.activeChat()).toHaveAttribute("data-session-id", other.session_id);
  await expect(viewer.activeChat().getByText(query, { exact: true })).toBeVisible();
  await expect
    .poll(async () => (await sessionReadState(api, other.session_id)).cursor)
    .toBe(tail.messageId);
  await dwell(
    page,
    500,
    "negative-assertion",
    "Explicit conversation navigation must not re-read the sibling",
  );
  expect((await sessionReadState(api, primary.session_id)).cursor).toBe(primaryTail);
  expect(reads.markReads.filter((read) => read.sessionId === primary.session_id)).toEqual([]);
  expect(await page.evaluate(() => performance.timeOrigin)).toBe(started);
}

/** Hold request dispatch, not a captured DTO: the released GET snapshots the new primary. */
async function holdTaskIdentityDispatch(page: Page, taskId: string) {
  const gate = Promise.withResolvers<void>();
  const queued = Promise.withResolvers<void>();
  const pattern = `**/api/v1/tasks/${taskId}`;
  const running = new Set<Promise<void>>();
  const handle = async (route: import("@playwright/test").Route) => {
    queued.resolve();
    await gate.promise;
    const response = await route.fetch();
    await route.fulfill({ response });
  };
  const handler = (route: import("@playwright/test").Route) => {
    const work = handle(route).finally(() => running.delete(work));
    running.add(work);
    return work;
  };
  await page.route(pattern, handler);
  return {
    queued: queued.promise,
    release: () => gate.resolve(),
    dispose: async () => {
      gate.resolve();
      while (running.size) await Promise.all([...running]);
      await page.unroute(pattern, handler);
    },
  };
}

export async function expectPrimarySwapDuringRouteAdmission(
  page: Page,
  actor: Page,
  api: ApiClient,
  seed: SeedData,
) {
  await api.saveUserSettings({ enable_preview_on_click: true });
  const title = `primaryswap${randomUUID().replaceAll("-", "")}`;
  const task = await api.seedTask(seed.workspaceId, title, {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
  });
  const previous = await api.seedTaskSession(task.task_id, {
    sessionId: randomUUID(),
    agentProfileId: seed.agentProfileId,
    state: "WAITING_FOR_INPUT",
  });
  const primary = await api.seedTaskSession(task.task_id, {
    sessionId: randomUUID(),
    agentProfileId: seed.agentProfileId,
    state: "WAITING_FOR_INPUT",
  });
  await api.seedSessionMessage(previous.session_id, {
    type: "message",
    content: "Former primary conversation",
  });
  const tail = await api.seedSessionMessage(primary.session_id, {
    type: "message",
    content: "New authoritative primary conversation",
  });
  await api.setPrimarySession(previous.session_id);
  await actor.goto(`/?taskId=${task.task_id}&sessionId=${primary.session_id}`);
  const preview = actor.getByTestId("task-preview-panel");
  await expect(preview.locator('[data-testid="session-chat"]:visible')).toHaveAttribute(
    "data-session-id",
    primary.session_id,
  );
  await new KanbanPage(page).goto(seed.workspaceId);
  const reads = observeReadTraffic(page);
  const gate = await holdTaskIdentityDispatch(page, task.task_id);
  try {
    const started = await page.evaluate(() => performance.timeOrigin);
    const membership = page.waitForResponse(
      (response) => new URL(response.url()).pathname === `/api/v1/tasks/${task.task_id}/sessions`,
    );
    await selectCommandTask(page, title);
    await gate.queued;
    const old = (await (await membership).json()) as {
      sessions: Array<{ id: string; is_primary: boolean }>;
    };
    expect(old.sessions.find((session) => session.is_primary)?.id).toBe(previous.session_id);
    await preview
      .getByTestId(`preview-session-tab-${primary.session_id}`)
      .click({ button: "right" });
    await actor.getByRole("menuitem", { name: "Set as Primary", exact: true }).click();
    await expect
      .poll(
        async () =>
          (await api.listTaskSessions(task.task_id)).sessions.find((session) => session.is_primary)
            ?.id,
      )
      .toBe(primary.session_id);
    expect((await sessionReadState(api, previous.session_id)).cursor).toBeNull();
    expect((await sessionReadState(api, primary.session_id)).cursor).toBeNull();
    const taskResponse = page.waitForResponse(
      (response) => new URL(response.url()).pathname === `/api/v1/tasks/${task.task_id}`,
    );
    gate.release();
    expect(
      ((await (await taskResponse).json()) as { primary_session_id: string }).primary_session_id,
    ).toBe(primary.session_id);
    const viewer = new SessionPage(page);
    await expect(viewer.activeChat()).toHaveAttribute("data-session-id", primary.session_id);
    await expect(
      viewer.activeChat().getByText("New authoritative primary conversation", { exact: true }),
    ).toBeVisible();
    await expect
      .poll(async () => (await sessionReadState(api, primary.session_id)).cursor)
      .toBe(tail.messageId);
    await dwell(
      page,
      1500,
      "negative-assertion",
      "The old membership primary must never acquire a read cursor",
    );
    expect((await sessionReadState(api, previous.session_id)).cursor).toBeNull();
    expect(reads.markReads.filter((read) => read.sessionId === previous.session_id)).toEqual([]);
    expect(await page.evaluate(() => performance.timeOrigin)).toBe(started);
  } finally {
    await gate.dispose();
  }
}
