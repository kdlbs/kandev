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
import {
  newestStoredMessageId,
  observeReadTraffic,
  sessionReadState,
  taskUnreadSessionIds,
  TASK_UNREAD_DOT_LABEL,
} from "./unread-divider";
import { withToolFileTask } from "./tool-file-opening";
import { waitForSessionState } from "./session";

export async function waitForIndexedConversation(
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

export async function selectCommandTask(page: Page, query: string, conversation = false) {
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

async function createSkipRouteSeed(api: ApiClient, seed: SeedData): Promise<SeedData> {
  const workflow = await api.createWorkflow(seed.workspaceId, `Skip route ${randomUUID()}`);
  const source = await api.createWorkflowStep(workflow.id, "Source", 0, { is_start_step: true });
  const destination = await api.createWorkflowStep(workflow.id, "Verify", 1);
  await api.updateWorkflowStep(destination.id, {
    prompt: "This entry prompt must be skipped",
    events: { on_enter: [{ type: "auto_start_agent" }] },
  });
  return { ...seed, workflowId: workflow.id, startStepId: source.id };
}

export async function expectConversationRouteAfterRemoval(
  page: Page,
  actor: Page,
  api: ApiClient,
  seed: SeedData,
  options: {
    hideTarget?: boolean;
    rememberPlan?: boolean;
    moveOutcome?: "failed" | "agentless" | "skip";
  } = {},
) {
  await api.saveUserSettings({ enable_preview_on_click: true });
  const fixtureSeed = options.moveOutcome === "skip" ? await createSkipRouteSeed(api, seed) : seed;
  const { task, old, primary, other } = await seedRemovalChoiceTask(
    api,
    fixtureSeed,
    options.hideTarget,
  );
  if (options.moveOutcome === "agentless" || options.moveOutcome === "skip") {
    for (const session of [primary, other])
      await api.seedTaskSession(task.task_id, {
        sessionId: session.session_id,
        agentProfileId: seed.agentProfileId,
        state: "COMPLETED",
        completedAt: new Date().toISOString(),
      });
    await api.setPrimarySession(primary.session_id);
  }
  const viewer = new SessionPage(page);
  await page.goto(`/t/${task.task_id}?sessionId=${old.session_id}`);
  await expect(viewer.activeChat()).toHaveAttribute("data-session-id", old.session_id);
  await actor.goto(
    `/?workflowId=${fixtureSeed.workflowId}&taskId=${task.task_id}&sessionId=${old.session_id}`,
  );
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
  const returnQuery = `return${randomUUID().replaceAll("-", "")}`;
  if (options.rememberPlan) {
    const message = await api.seedSessionMessage(primary.session_id, {
      type: "message",
      content: returnQuery,
    });
    await waitForIndexedConversation(api, seed.workspaceId, returnQuery, {
      sessionId: primary.session_id,
      messageId: message.messageId,
    });
  }
  const primaryTail = await newestStoredMessageId(api, primary.session_id);
  await expect
    .poll(async () => (await sessionReadState(api, primary.session_id)).cursor)
    .toBe(primaryTail);
  if (options.rememberPlan) {
    await page.getByTestId("mobile-sessions-pill").tap();
    await page.getByTestId(`mobile-session-row-${other.session_id}`).tap();
    await expect(viewer.activeChat()).toHaveAttribute("data-session-id", other.session_id);
    await page
      .getByTestId("session-mobile-bottom-nav")
      .getByRole("button", { name: "Plan", exact: true })
      .tap();
    await expect(viewer.activeChat()).toHaveCount(0);
    await selectCommandTask(page, returnQuery, true);
    await expect(viewer.activeChat()).toHaveAttribute("data-session-id", primary.session_id);
  }
  const otherBefore = (await sessionReadState(api, other.session_id)).cursor;
  if (options.hideTarget) {
    await viewer.sessionTabCloseButton(other.session_id).click();
    await expect(viewer.sessionTabBySessionId(other.session_id)).toHaveCount(0);
  }
  const query = `surviving${randomUUID().replaceAll("-", "")}`;
  const tail = await api.seedSessionMessage(other.session_id, { type: "message", content: query });
  expect((await sessionReadState(api, other.session_id)).cursor).toBe(otherBefore);
  await waitForIndexedConversation(api, seed.workspaceId, query, {
    sessionId: other.session_id,
    messageId: tail.messageId,
  });
  const reads = observeReadTraffic(page);
  const started = await page.evaluate(() => performance.timeOrigin);
  if (options.moveOutcome)
    await selectConversationAcrossMove(page, api, {
      taskId: task.task_id,
      retainedId: primary.session_id,
      query,
      outcome: options.moveOutcome,
    });
  else await selectCommandTask(page, query, true);
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
  expect(reads.agentStarts()).toEqual([]);
  if (options.rememberPlan) {
    await page
      .getByTestId("session-mobile-bottom-nav")
      .getByRole("button", { name: "Plan", exact: true })
      .tap();
    const repeatQuery = `repeat${randomUUID().replaceAll("-", "")}`;
    const after = await api.seedSessionMessage(other.session_id, {
      type: "message",
      content: repeatQuery,
    });
    await dwell(page, 500, "negative-assertion", "Consumed explicit focus must not reopen Chat");
    await expect(viewer.activeChat()).toHaveCount(0);
    expect((await sessionReadState(api, other.session_id)).cursor).toBe(tail.messageId);
    expect(reads.markReads.filter((read) => read.messageId === after.messageId)).toEqual([]);
    await expect
      .poll(() => taskUnreadSessionIds(api, seed.workspaceId, task.task_id))
      .toEqual([other.session_id]);
    await page.getByTestId("mobile-task-picker-trigger").tap();
    const picker = page.getByRole("dialog");
    const row = picker.getByTestId("sidebar-task-item").filter({ hasText: "Owned removal choice" });
    await expect(row.getByRole("img", { name: TASK_UNREAD_DOT_LABEL })).toBeVisible();
    expect((await sessionReadState(api, other.session_id)).cursor).toBe(tail.messageId);
    await expect(viewer.activeChat()).toHaveCount(0);
    await page.keyboard.press("Escape");
    await expect(picker).toHaveCount(0);
    await waitForIndexedConversation(api, seed.workspaceId, repeatQuery, {
      sessionId: other.session_id,
      messageId: after.messageId,
    });
    await selectCommandTask(page, repeatQuery, true);
    await expect(viewer.activeChat()).toHaveAttribute("data-session-id", other.session_id);
    await expect
      .poll(async () => (await sessionReadState(api, other.session_id)).cursor)
      .toBe(after.messageId);
    expect((await sessionReadState(api, primary.session_id)).cursor).toBe(primaryTail);
    expect(reads.agentStarts()).toEqual([]);
    await expect.poll(() => taskUnreadSessionIds(api, seed.workspaceId, task.task_id)).toEqual([]);
  }
  if (options.hideTarget) {
    await viewer.sessionTabCloseButton(other.session_id).click();
    await expect(viewer.sessionTabBySessionId(other.session_id)).toHaveCount(0);
    const repeatQuery = `hidden${randomUUID().replaceAll("-", "")}`;
    const after = await api.seedSessionMessage(other.session_id, {
      type: "message",
      content: repeatQuery,
    });
    await waitForIndexedConversation(api, seed.workspaceId, repeatQuery, {
      sessionId: other.session_id,
      messageId: after.messageId,
    });
    await selectCommandTask(page, repeatQuery, true);
    await expect(viewer.activeChat()).toHaveAttribute("data-session-id", other.session_id);
    await expect
      .poll(async () => (await sessionReadState(api, other.session_id)).cursor)
      .toBe(after.messageId);
    expect((await sessionReadState(api, primary.session_id)).cursor).toBe(primaryTail);
    expect(reads.agentStarts()).toEqual([]);
    expect(await page.evaluate(() => performance.timeOrigin)).toBe(started);
  }
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

async function seedRoutePrimary(api: ApiClient, seed: SeedData, taskId: string) {
  const primary = await api.seedTaskSession(taskId, {
    sessionId: randomUUID(),
    agentProfileId: seed.agentProfileId,
    state: "WAITING_FOR_INPUT",
  });
  const tail = await api.seedSessionMessage(primary.session_id, {
    type: "message",
    content: "New authoritative primary conversation",
  });
  return { primary, tail };
}

async function openPrimaryPreview(actor: Page, taskId: string, sessionId: string) {
  await actor.goto(`/?taskId=${taskId}&sessionId=${sessionId}`);
  await expect(
    actor.getByTestId("task-preview-panel").locator('[data-testid="session-chat"]:visible'),
  ).toHaveAttribute("data-session-id", sessionId);
}

async function selectConversationAcrossMove(
  page: Page,
  api: ApiClient,
  entry: {
    taskId: string;
    retainedId: string;
    query: string;
    outcome: "failed" | "agentless" | "skip";
  },
) {
  const gate = await holdTaskIdentityDispatch(page, entry.taskId);
  const releaseFailure = Promise.withResolvers<void>();
  const moveQueued = Promise.withResolvers<void>();
  const phone = (page.viewportSize()?.width ?? 1280) < 768;
  let moveWork: Promise<void> | undefined;
  const pattern = `**/api/v1/tasks/${entry.taskId}/move`;
  await page.route(
    pattern,
    (route) => {
      moveWork = (async () => {
        moveQueued.resolve();
        await releaseFailure.promise;
        if (entry.outcome === "failed")
          await route.fulfill({ status: 500, json: { error: "Injected workflow failure" } });
        else {
          if (entry.outcome === "skip")
            expect(route.request().postDataJSON().entry_options).toEqual({
              skip_step_prompt: true,
            });
          const response = await route.fetch();
          expect(response.ok()).toBe(true);
          expect((await response.json()).workflow_focus_disposition).toBe("none");
          await route.fulfill({ response });
        }
      })();
      return moveWork;
    },
    { times: 1 },
  );
  try {
    await selectCommandTask(page, entry.query, true);
    await gate.queued;
    const task = await api.getTask(entry.taskId);
    const stepId = task.workflow_step_id;
    const workflowId = task.workflow_id;
    if (!workflowId) throw new Error("Missing workflow identity");
    const destination = (await api.listWorkflowSteps(workflowId)).steps.find(
      (step) =>
        step.id !== stepId &&
        (entry.outcome === "failed" ||
          (entry.outcome === "skip"
            ? step.events?.on_enter?.some((action) => action.type === "auto_start_agent")
            : !step.events?.on_enter?.some((action) => action.type === "auto_start_agent"))),
    );
    if (!destination) throw new Error("Missing workflow destination");
    if (phone) {
      await page.getByTestId("mobile-task-picker-trigger").tap();
      await page
        .getByRole("dialog", { name: "Tasks", exact: true })
        .getByTestId("sidebar-task-item")
        .filter({ hasText: task.title })
        .getByRole("button", { name: "Task actions", exact: true })
        .tap();
      await page.getByTestId("task-context-move-to").tap();
      await page.getByTestId(`task-context-step-${destination.id}`).tap();
    } else {
      await page.getByTestId(`workflow-step-${destination.name}`).hover();
      if (entry.outcome === "skip") {
        await page.getByTestId("workflow-step-move-options-trigger").click();
        await page.getByTestId("workflow-move-skip-step-prompt").check();
      }
      await page.getByTestId("workflow-step-move-here").click();
    }
    await moveQueued.promise;
    const loaded = page.waitForResponse(
      (response) => new URL(response.url()).pathname === `/api/v1/tasks/${entry.taskId}`,
    );
    gate.release();
    expect((await loaded).ok()).toBe(true);
    await expect(page.locator("main > [aria-busy]")).toHaveAttribute("aria-busy", "false");
    await waitForActiveTaskSession(page, entry.taskId, entry.retainedId);
    releaseFailure.resolve();
    await moveWork;
    if (entry.outcome === "failed")
      await expect(page.getByText("Failed to move task", { exact: true })).toBeVisible();
    if (phone) await page.keyboard.press("Escape");
  } finally {
    releaseFailure.resolve();
    await moveWork;
    await gate.dispose();
    await page.unroute(pattern);
  }
}

export async function expectPrimarySwapDuringRouteAdmission(
  page: Page,
  actor: Page,
  api: ApiClient,
  seed: SeedData,
  options: { createAfterMembership?: boolean; explicitMember?: boolean } = {},
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
  let prepared = options.createAfterMembership
    ? null
    : await seedRoutePrimary(api, seed, task.task_id);
  const requested = options.explicitMember ? prepared : null;
  let query = title;
  if (requested) {
    query = `explicitmember${randomUUID().replaceAll("-", "")}`;
    requested.tail = await api.seedSessionMessage(requested.primary.session_id, {
      type: "message",
      content: query,
    });
    await waitForIndexedConversation(api, seed.workspaceId, query, {
      sessionId: requested.primary.session_id,
      messageId: requested.tail.messageId,
    });
  }
  await api.seedSessionMessage(previous.session_id, {
    type: "message",
    content: "Former primary conversation",
  });
  await api.setPrimarySession(previous.session_id);
  if (prepared) await openPrimaryPreview(actor, task.task_id, prepared.primary.session_id);
  await new KanbanPage(page).goto(seed.workspaceId);
  const reads = observeReadTraffic(page);
  const gate = await holdTaskIdentityDispatch(page, task.task_id);
  try {
    const started = await page.evaluate(() => performance.timeOrigin);
    const membership = page.waitForResponse(
      (response) => new URL(response.url()).pathname === `/api/v1/tasks/${task.task_id}/sessions`,
    );
    await selectCommandTask(page, query, !!requested);
    await gate.queued;
    const old = (await (await membership).json()) as {
      sessions: Array<{ id: string; is_primary: boolean }>;
    };
    expect(old.sessions.find((session) => session.is_primary)?.id).toBe(previous.session_id);
    if (!prepared || requested) {
      if (!prepared) expect(old.sessions.map((row) => row.id)).toEqual([previous.session_id]);
      prepared = await seedRoutePrimary(api, seed, task.task_id);
      await openPrimaryPreview(actor, task.task_id, prepared.primary.session_id);
    }
    const { primary } = prepared;
    const preview = actor.getByTestId("task-preview-panel");
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
    const failedRefresh = `**/api/v1/tasks/${task.task_id}/sessions*`;
    if (requested)
      await page.route(
        failedRefresh,
        (route) =>
          route.fulfill({ status: 503, json: { error: "Primary-only refresh unavailable" } }),
        { times: 1 },
      );
    const taskResponse = page.waitForResponse(
      (response) => new URL(response.url()).pathname === `/api/v1/tasks/${task.task_id}`,
    );
    gate.release();
    expect(
      ((await (await taskResponse).json()) as { primary_session_id: string }).primary_session_id,
    ).toBe(primary.session_id);
    const target = requested ?? prepared;
    await expectReadInRoute(page, api, {
      targetId: target.primary.session_id,
      targetTail: target.tail.messageId,
      targetText: requested ? query : "New authoritative primary conversation",
      unreadIds: requested ? [previous.session_id, primary.session_id] : [previous.session_id],
      reads,
      started,
    });
  } finally {
    await gate.dispose();
  }
}

async function expectReadInRoute(
  page: Page,
  api: ApiClient,
  entry: {
    targetId: string;
    targetTail: string;
    targetText: string;
    unreadIds: string[];
    reads: ReturnType<typeof observeReadTraffic>;
    started: number;
  },
) {
  const viewer = new SessionPage(page);
  await expect(viewer.activeChat()).toHaveAttribute("data-session-id", entry.targetId);
  await expect(viewer.activeChat().getByText(entry.targetText, { exact: true })).toBeVisible();
  await expect
    .poll(async () => (await sessionReadState(api, entry.targetId)).cursor)
    .toBe(entry.targetTail);
  await dwell(page, 1500, "negative-assertion", "Unrequested route siblings must remain unread");
  for (const id of entry.unreadIds) {
    expect((await sessionReadState(api, id)).cursor).toBeNull();
    expect(entry.reads.markReads.filter((read) => read.sessionId === id)).toEqual([]);
  }
  expect(await page.evaluate(() => performance.timeOrigin)).toBe(entry.started);
}

/** The coarse tablet's file tab is a different owner from phone Plan/Dockview. */
export async function expectTabletConversationRoute(
  context: Parameters<typeof withToolFileTask>[0],
  assertDot: boolean,
  screenshotPath: string,
) {
  const { testPage: page, apiClient: api, seedData: seed } = context;
  await page.setViewportSize({ width: 820, height: 1180 });
  const reads = observeReadTraffic(page);
  expect(await page.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(true);
  await withToolFileTask(context, "read", async (session) => {
    await expect(page.getByTestId("tablet-task-layout")).toBeVisible();
    const taskId = new URL(page.url()).pathname.split("/").at(-1)!;
    const sessionId = (await session.activeChat().getAttribute("data-session-id"))!;
    await waitForSessionState(api, {
      taskId,
      sessionId,
      expectedState: "WAITING_FOR_INPUT",
      message: "The tool-file task must settle before explicit navigation",
    });
    const sibling = await api.seedTaskSession(taskId, {
      sessionId: randomUUID(),
      agentProfileId: seed.agentProfileId,
      state: "WAITING_FOR_INPUT",
    });
    const beforeSibling = (await sessionReadState(api, sibling.session_id)).cursor;
    const query = `tablet${randomUUID().replaceAll("-", "")}`;
    const first = await api.seedSessionMessage(sessionId, { type: "message", content: query });
    await waitForIndexedConversation(api, seed.workspaceId, query, {
      sessionId,
      messageId: first.messageId,
    });
    await selectCommandTask(page, query, true);
    await expect(page).toHaveURL(new RegExp(`sessionId=${sessionId}`));
    await expect
      .poll(async () => (await sessionReadState(api, sessionId)).cursor)
      .toBe(first.messageId);
    const link = session.activeChat().locator("button[title]").filter({ hasText: "Dockerfile" });
    await expect(link).toHaveCount(1);
    await link.tap();
    const fileTab = page.getByRole("tab", { name: "Dockerfile", exact: true });
    await expect(fileTab).toHaveAttribute("data-state", "active");
    await expect(session.activeChat()).toHaveCount(0);
    const initialStarts = reads.agentStarts().length;
    const repeat = `tabletrepeat${randomUUID().replaceAll("-", "")}`;
    const tail = await api.seedSessionMessage(sessionId, { type: "message", content: repeat });
    await waitForIndexedConversation(api, seed.workspaceId, repeat, {
      sessionId,
      messageId: tail.messageId,
    });
    await expect
      .poll(() => taskUnreadSessionIds(api, seed.workspaceId, taskId))
      .toEqual([sessionId]);
    await dwell(
      page,
      500,
      "negative-assertion",
      "Tablet file view must not acknowledge unseen Chat",
    );
    expect((await sessionReadState(api, sessionId)).cursor).toBe(first.messageId);
    expect(reads.markReads.filter((read) => read.messageId === tail.messageId)).toEqual([]);
    if (assertDot) {
      const row = session.sidebarTaskItem("Tool file read");
      await expect(row.getByRole("img", { name: TASK_UNREAD_DOT_LABEL })).toBeVisible();
    }
    await page.screenshot({ path: screenshotPath });
    await selectCommandTask(page, repeat, true);
    await expect(session.activeChat()).toHaveAttribute("data-session-id", sessionId);
    await expect(page.getByRole("tab", { name: "Chat", exact: true })).toHaveAttribute(
      "data-state",
      "active",
    );
    await page.screenshot({ path: screenshotPath.replace(".png", "-chat.png") });
    await expect
      .poll(async () => (await sessionReadState(api, sessionId)).cursor)
      .toBe(tail.messageId);
    expect((await sessionReadState(api, sibling.session_id)).cursor).toBe(beforeSibling);
    expect(reads.markReads.map((read) => read.sessionId)).not.toContain(sibling.session_id);
    expect(reads.agentStarts().slice(initialStarts)).toEqual([]);
    await fileTab.tap();
    const later = await api.seedSessionMessage(sessionId, {
      type: "message",
      content: "Passive tablet output",
    });
    await dwell(
      page,
      500,
      "negative-assertion",
      "Consumed tablet focus must not steal a later file choice",
    );
    await expect(fileTab).toHaveAttribute("data-state", "active");
    expect((await sessionReadState(api, sessionId)).cursor).toBe(tail.messageId);
    expect(reads.markReads.filter((read) => read.messageId === later.messageId)).toEqual([]);
    await page.reload();
    await expect(fileTab).toBeVisible();
    await dwell(
      page,
      500,
      "negative-assertion",
      "Restored file tabs must not displace explicit Chat",
    );
    await expect(session.activeChat()).toHaveAttribute("data-session-id", sessionId);
    await expect
      .poll(async () => (await sessionReadState(api, sessionId)).cursor)
      .toBe(later.messageId);
    await page.screenshot({ path: screenshotPath.replace(".png", "-boot.png") });
  });
}
