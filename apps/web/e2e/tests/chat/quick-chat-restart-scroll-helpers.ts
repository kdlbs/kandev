import { type Locator, type Page } from "@playwright/test";
import { expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import {
  openQuickChatSetup,
  selectAgentIfNeeded,
  startQuickChatFromSetup,
  waitForSessionSettledBaseline,
} from "./quick-chat-helpers";
import { openMobileQuickChatSetup } from "./quick-chat-saved-prompt-delivery-helpers";

export const RESTART_SCROLL_OPENING_PROMPT = "/e2e:simple-message";
export const RESTART_SCROLL_REPLY_PREFIX = "Quick Chat restart reply";
export const RESTART_SCROLL_REPLY_COUNT = 120;
export const RESTART_SCROLL_LATEST_REPLY = `${RESTART_SCROLL_REPLY_PREFIX} ${RESTART_SCROLL_REPLY_COUNT}`;
export const RESTART_SCROLL_RESUME_DELAY = "30s";

export type RestartScrollFixture = {
  taskId: string;
  sessionId: string;
  latestReply: string;
  profileId: string | null;
};

async function createDelayedResumeProfile(
  apiClient: ApiClient,
  delay: string,
): Promise<{ id: string; name: string }> {
  const { agents } = await apiClient.listAgents();
  const mockAgent = agents.find((agent) => agent.name === "mock-agent");
  if (!mockAgent) throw new Error("mock-agent not found while creating a delayed resume profile");
  const name = `Quick Chat restart scroll ${Date.now()}`;
  const profile = await apiClient.createAgentProfile(mockAgent.id, name, {
    model: "mock-fast",
    cli_passthrough: false,
    env_vars: [{ key: "E2E_MOCK_AGENT_RESUME_DELAY", value: delay }],
  });
  return { id: profile.id, name };
}

/**
 * Opens an ordinary Quick Chat and persists more messages than the latest
 * history window, so every reopen must reconcile cached rows or a bounded
 * refresh with the bottom placement. The optional resume delay keeps the
 * restored agent visibly starting long enough to assert its status geometry.
 */
export async function createRestartScrollFixture(params: {
  page: Page;
  apiClient: ApiClient;
  mobile: boolean;
  resumeDelay?: string;
}): Promise<RestartScrollFixture> {
  const { page, apiClient, mobile, resumeDelay } = params;
  let profileId: string | null = null;
  let profileName: string | null = null;
  if (resumeDelay) {
    const profile = await createDelayedResumeProfile(apiClient, resumeDelay);
    profileId = profile.id;
    profileName = profile.name;
  }

  const dialog = mobile ? await openMobileQuickChatSetup(page) : await openQuickChatSetup(page);
  await selectAgentIfNeeded(dialog, page, profileName ?? undefined);
  const started = await startQuickChatFromSetup(dialog, page, RESTART_SCROLL_OPENING_PROMPT);
  await waitForSessionSettledBaseline(apiClient, started.task_id, started.session_id);
  await apiClient.seedAgentMessages(
    started.session_id,
    RESTART_SCROLL_REPLY_COUNT,
    RESTART_SCROLL_REPLY_PREFIX,
  );
  await expect
    .poll(
      async () =>
        (await apiClient.listSessionMessages(started.session_id)).messages.some(
          (message) => message.content === RESTART_SCROLL_LATEST_REPLY,
        ),
      {
        timeout: 30_000,
        message: "seeded Quick Chat restart history was not persisted",
      },
    )
    .toBe(true);

  await closeQuickChat(page, dialog, mobile);

  return {
    taskId: started.task_id,
    sessionId: started.session_id,
    latestReply: RESTART_SCROLL_LATEST_REPLY,
    profileId,
  };
}

export async function cleanupRestartScrollFixture(
  apiClient: ApiClient,
  fixture: RestartScrollFixture,
): Promise<void> {
  if (fixture.profileId) {
    await apiClient.deleteAgentProfile(fixture.profileId, true).catch(() => undefined);
  }
}

export async function openExistingQuickChat(page: Page, mobile: boolean): Promise<Locator> {
  const dialog = page.getByRole("dialog", { name: "Quick Chat" });
  const alreadyOpen = await dialog.isVisible({ timeout: 500 }).catch(() => false);
  if (!alreadyOpen) {
    if (mobile) {
      await page.getByTestId("app-nav-trigger").tap();
      await page.getByTestId("mobile-quick-chat-button").tap();
    } else {
      const modifier = process.platform === "darwin" ? "Meta" : "Control";
      await page.keyboard.press(`${modifier}+Shift+q`);
    }
  }
  await expect(dialog).toBeVisible({ timeout: 10_000 });
  return dialog;
}

export async function openQuickChatConversation(
  page: Page,
  sessionId: string,
  mobile: boolean,
): Promise<Locator> {
  const dialog = await openExistingQuickChat(page, mobile);
  const tab = dialog.locator(`[data-tab-reference="conversation:${sessionId}"]`);
  await expect(tab).toBeVisible({ timeout: 15_000 });
  if (mobile) await tab.tap();
  else await tab.click();
  return dialog;
}

export async function closeQuickChat(page: Page, dialog: Locator, mobile: boolean): Promise<void> {
  if (mobile) {
    await dialog.getByTestId("quick-chat-close").tap();
  } else {
    await page.keyboard.press("Escape");
  }
  await expect(dialog).toBeHidden({ timeout: 10_000 });
}

export function quickChatScroller(dialog: Locator): Locator {
  return dialog.getByTestId("quick-chat-messages").locator(".chat-message-list").first();
}

export function quickChatLatestReply(dialog: Locator, latestReply: string): Locator {
  return dialog.getByTestId("quick-chat-messages").getByText(latestReply, { exact: true }).first();
}

export async function expectScrollerAtBottom(
  scroller: Locator,
  message = "Quick Chat transcript should rest at its newest message",
  timeout = 15_000,
): Promise<void> {
  await expect
    .poll(
      async () =>
        scroller.evaluate(
          (element) => element.scrollHeight - element.scrollTop - element.clientHeight,
        ),
      { timeout, message },
    )
    .toBeLessThan(10);
}

function expectBoxInsideViewport(
  contentBox: { x: number; y: number; width: number; height: number },
  viewportBox: { x: number; y: number; width: number; height: number },
): void {
  expect(contentBox.y).toBeGreaterThanOrEqual(viewportBox.y - 1);
  expect(contentBox.y + contentBox.height).toBeLessThanOrEqual(
    viewportBox.y + viewportBox.height + 1,
  );
}

export async function expectLatestReplyAtBottom(
  dialog: Locator,
  scroller: Locator,
  latestReply: string,
  timeout = 30_000,
): Promise<void> {
  const reply = quickChatLatestReply(dialog, latestReply);
  await expect(reply).toBeVisible({ timeout });
  const [replyBox, scrollerBox] = await Promise.all([reply.boundingBox(), scroller.boundingBox()]);
  expect(replyBox).not.toBeNull();
  expect(scrollerBox).not.toBeNull();
  if (!replyBox || !scrollerBox) throw new Error("Quick Chat latest reply geometry is unavailable");
  expectBoxInsideViewport(replyBox, scrollerBox);
  await expectScrollerAtBottom(scroller);
}

/**
 * Waits for the active restore status and proves its complete bounds remain in
 * the transcript viewport while the run is still starting.
 */
export async function expectActiveResumeStatusInViewport(
  dialog: Locator,
  scroller: Locator,
  timeout = 30_000,
): Promise<Locator> {
  const status = scroller
    .getByRole("status")
    .filter({ hasText: /starting|resuming/i })
    .first();
  await expect(status).toBeVisible({ timeout });
  const [statusBox, scrollerBox] = await Promise.all([
    status.boundingBox(),
    scroller.boundingBox(),
  ]);
  expect(statusBox).not.toBeNull();
  expect(scrollerBox).not.toBeNull();
  if (!statusBox || !scrollerBox) {
    throw new Error("Quick Chat resume status geometry is unavailable");
  }
  expectBoxInsideViewport(statusBox, scrollerBox);
  return status;
}

export async function expectNoPromptReplay(
  apiClient: ApiClient,
  sessionId: string,
  prompt: string,
): Promise<void> {
  const { messages } = await apiClient.listSessionMessages(sessionId);
  const matchingPrompts = messages.filter(
    (message) => message.author_type === "user" && message.content === prompt,
  );
  expect(matchingPrompts).toHaveLength(1);
  const latestReplies = messages.filter(
    (message) => message.author_type === "agent" && message.content === RESTART_SCROLL_LATEST_REPLY,
  );
  expect(latestReplies).toHaveLength(1);
}

/** Proves the restored conversation still belongs to the original task/session. */
export async function expectRestoredSessionIdentity(
  apiClient: ApiClient,
  fixture: RestartScrollFixture,
): Promise<void> {
  const { session } = await apiClient.getTaskSession(fixture.sessionId);
  expect(session.id).toBe(fixture.sessionId);
  expect(session.task_id).toBe(fixture.taskId);
}

async function waitForLayoutFrames(page: Page, frames: number): Promise<void> {
  await page.evaluate(async (frameCount) => {
    for (let index = 0; index < frameCount; index += 1) {
      await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
    }
  }, frames);
}

/**
 * Proves a held latest-window response is still outstanding. With chat motion
 * enabled, the transcript carries the overflow-anchor override while history
 * refresh is pending.
 */
export async function expectHistoryRefreshPending(scroller: Locator): Promise<void> {
  await expect(scroller).toHaveClass(/\[overflow-anchor:auto\]/);
}

/**
 * Proves the held response was applied: the client clears
 * `historyRefreshPending`, which removes the transcript's overflow-anchor
 * override. Two frames then let the final placement/layout effects settle.
 */
export async function expectHistoryRefreshSettled(page: Page, scroller: Locator): Promise<void> {
  await expect(scroller).toHaveClass(/\[overflow-anchor:none\]/, { timeout: 15_000 });
  await waitForLayoutFrames(page, 2);
}

/** Delivers a real upward wheel gesture and returns the settled scroll offset. */
export async function wheelReaderUp(page: Page, scroller: Locator): Promise<number> {
  await scroller.hover();
  const initialScrollTop = await scroller.evaluate((element) => element.scrollTop);
  await page.mouse.wheel(0, -400);
  let previous = initialScrollTop;
  let settled = initialScrollTop;
  await expect
    .poll(
      async () => {
        const current = await scroller.evaluate((element) => element.scrollTop);
        const isSettled = current < initialScrollTop - 100 && current === previous;
        previous = current;
        settled = current;
        return isSettled;
      },
      { message: "reader wheel should settle above the Quick Chat bottom" },
    )
    .toBe(true);
  return settled;
}

/** Delivers a real upward touch gesture through CDP for the mobile project. */
export async function touchReaderUp(page: Page, scroller: Locator): Promise<number> {
  const client = await page.context().newCDPSession(page);
  const initialScrollTop = await scroller.evaluate((element) => element.scrollTop);
  try {
    const box = await scroller.boundingBox();
    if (!box) throw new Error("Quick Chat transcript has no reading viewport");
    const point = { x: box.x + box.width / 2, y: box.y + box.height / 4 };
    await client.send("Input.dispatchTouchEvent", {
      type: "touchStart",
      touchPoints: [point],
    });
    await client.send("Input.dispatchTouchEvent", {
      type: "touchMove",
      touchPoints: [{ ...point, y: point.y + box.height / 2 }],
    });
    await client.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
  } finally {
    await client.detach();
  }
  let previous = initialScrollTop;
  let settled = initialScrollTop;
  await expect
    .poll(
      async () => {
        const current = await scroller.evaluate((element) => element.scrollTop);
        const isSettled = current < initialScrollTop - 50 && current === previous;
        previous = current;
        settled = current;
        return isSettled;
      },
      { message: "reader touch should settle above the Quick Chat bottom" },
    )
    .toBe(true);
  return settled;
}

export async function expectReaderPositionPreserved(
  scroller: Locator,
  expectedScrollTop: number,
  message: string,
  tolerance = 20,
): Promise<void> {
  await expect
    .poll(
      async () =>
        Math.abs((await scroller.evaluate((element) => element.scrollTop)) - expectedScrollTop),
      {
        timeout: 15_000,
        message,
      },
    )
    .toBeLessThanOrEqual(tolerance);
}

/** Returns to the newest message with the same input modality the reader used. */
export async function readerReturnToBottom(
  page: Page,
  scroller: Locator,
  mobile: boolean,
): Promise<void> {
  if (mobile) {
    const client = await page.context().newCDPSession(page);
    try {
      const box = await scroller.boundingBox();
      if (!box) throw new Error("Quick Chat transcript has no reading viewport");
      const point = { x: box.x + box.width / 2, y: box.y + (box.height * 3) / 4 };
      await client.send("Input.dispatchTouchEvent", {
        type: "touchStart",
        touchPoints: [point],
      });
      await client.send("Input.dispatchTouchEvent", {
        type: "touchMove",
        touchPoints: [{ ...point, y: box.y + box.height / 4 }],
      });
      await client.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
    } finally {
      await client.detach();
    }
  } else {
    await scroller.hover();
    await page.mouse.wheel(0, 5000);
  }
  await expectScrollerAtBottom(scroller, "returning to the newest message should resume follow");
}
