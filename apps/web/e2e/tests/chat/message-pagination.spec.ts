// Chat message pagination — the first stored prompt is the visible transcript
// start even when older internal rows remain on the backend.
import { test, expect } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";
import {
  DEEP_PROMPT_MARKER,
  EAGER_HISTORY_PROMPT_MARKER,
  INITIAL_PROMPT_MARKER,
  PRE_PROMPT_MARKER,
  RECENT_AGENT_MARKER,
  SHORT_PAGE_BOUNDARY_MARKER,
  TASK_DESCRIPTION_MARKER,
  TEXT_BATCH_ANCHOR_MARKER,
  TEXT_BATCH_MARKER,
  VISIBLE_PAGE_MARKER,
  LONG_HISTORY_TAIL_MARKER,
  RESTORED_SESSION_OLDER_MARKER,
  RESTORED_SESSION_TAIL_MARKER,
  readMessageRowTopById,
  readStandaloneMessageTop,
  seedCollapsedMessageHistory,
  seedLongMessageHistory,
  seedRestoredInactiveSessionHistory,
  seedShortBoundaryPageHistory,
  seedTextSparseMessageHistory,
  seedToolHeavyOpeningHistory,
  seedVisibleMessageHistory,
  scrollToOldestLoadedEdge,
  suppressChatPaginationIntersections,
  watchOlderMessageRequests,
} from "./message-pagination-helpers";

test.describe("@chat message pagination", () => {
  test("restored inactive session resumes pagination from hard-top input", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(180_000);
    // @covers AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.14
    // Reproduce the restored-panel race deterministically: the browser keeps
    // the hidden chat mounted but does not deliver a fresh sentinel entry when
    // its geometry later becomes visible.
    await suppressChatPaginationIntersections(testPage);
    const { taskId, primarySessionId, targetSessionId } = await seedRestoredInactiveSessionHistory(
      apiClient,
      seedData,
      "message-pagination-restored-inactive-session",
    );
    const olderRequests = watchOlderMessageRequests(testPage, targetSessionId);

    await testPage.goto(`/t/${taskId}`);
    let session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.sessionTabBySessionId(primarySessionId).click();
    await expect(session.sessionTabBySessionId(targetSessionId)).toBeVisible();

    await testPage.reload();
    session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.sessionTabBySessionId(primarySessionId)).toBeVisible();
    await expect(session.sessionTabBySessionId(targetSessionId)).toBeVisible();
    const restoredChat = testPage
      .locator('[data-testid="session-chat"]')
      .filter({ hasText: RESTORED_SESSION_TAIL_MARKER });
    await expect(restoredChat).toBeAttached();
    olderRequests.length = 0;

    await session.sessionTabBySessionId(targetSessionId).click();
    const chat = session.activeChat();
    const list = chat.locator(".chat-message-list");
    await list.evaluate((element) => {
      element.scrollTop = 0;
    });
    expect(await list.evaluate((element) => element.scrollTop)).toBe(0);
    await list.dispatchEvent("wheel", { deltaY: -120 });
    const olderMarker = chat.getByText(RESTORED_SESSION_OLDER_MARKER, { exact: true });
    await expect(olderMarker).toBeVisible({
      timeout: 15_000,
    });
    await expect.poll(() => olderRequests.length).toBeGreaterThan(0);
    expect(new URL(olderRequests[0]).searchParams.get("before")).toBeTruthy();
    await olderMarker.scrollIntoViewIfNeeded();
    await prCapture.screenshot("restored-chat-hard-top-pagination", {
      caption: "Restored secondary chat after hard-top input loads its older history",
    });
  });

  test("does not load older history while opening a task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);

    const { taskId, sessionId } = await seedToolHeavyOpeningHistory(
      apiClient,
      seedData,
      "message-pagination-does-not-eager-load",
    );
    const olderRequests = watchOlderMessageRequests(testPage, sessionId);

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    await dwell(testPage, 500, "negative-assertion", "observe background pagination after open");
    const chat = session.activeChat();

    await expect(chat.getByText(TASK_DESCRIPTION_MARKER, { exact: true })).toHaveCount(0);
    await expect(chat.getByText(EAGER_HISTORY_PROMPT_MARKER, { exact: true })).toHaveCount(0);
    expect(olderRequests).toHaveLength(0);
  });

  test("continues through more than twenty older pages in one upward reach", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);

    const { taskId, sessionId } = await seedLongMessageHistory(
      apiClient,
      seedData,
      "message-pagination-long-history",
    );
    const olderRequests = watchOlderMessageRequests(testPage, sessionId);

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    const chat = session.activeChat();
    const list = chat.locator(".chat-message-list");

    await expect(chat.getByText(DEEP_PROMPT_MARKER, { exact: true })).toHaveCount(0);
    const edge = await scrollToOldestLoadedEdge(list, LONG_HISTORY_TAIL_MARKER);
    expect(Number.isFinite(edge.rowTop)).toBe(true);
    // Isolate prepend stability from the browser settling the synthetic edge scroll.
    await expect.poll(() => olderRequests.length).toBeGreaterThan(0);
    const anchoredTop = await readStandaloneMessageTop(list, LONG_HISTORY_TAIL_MARKER);

    await expect
      .poll(() => olderRequests.length, {
        timeout: 30_000,
        intervals: [100],
        message: "Long history reaches the deep stored prompt",
      })
      .toBeGreaterThan(20);
    await expect(chat.getByText(DEEP_PROMPT_MARKER, { exact: true })).toBeVisible({
      timeout: 30_000,
    });

    const afterLoadTop = await readStandaloneMessageTop(list, LONG_HISTORY_TAIL_MARKER);
    expect(Math.abs(afterLoadTop - anchoredTop)).toBeLessThanOrEqual(8);
    await expect(chat.getByText(TASK_DESCRIPTION_MARKER, { exact: true })).toHaveCount(0);
    await expect(chat.getByTestId("load-older-messages")).toHaveCount(0);
  });

  test("loads one visible page per upward reach without cascading", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);

    const { taskId, sessionId } = await seedVisibleMessageHistory(
      apiClient,
      seedData,
      "message-pagination-does-not-cascade",
    );
    const olderRequests = watchOlderMessageRequests(testPage, sessionId);

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    const list = session.activeChat().locator(".chat-message-list");
    const edge = await scrollToOldestLoadedEdge(list, VISIBLE_PAGE_MARKER);
    expect(edge.rowId).not.toBeNull();
    expect(Number.isFinite(edge.rowTop)).toBe(true);

    await expect
      .poll(
        async () =>
          olderRequests.length === 1 &&
          (await list.evaluate((element) => element.scrollHeight)) > edge.scrollHeight,
        { timeout: 15_000, intervals: [100], message: "One older visible page loaded" },
      )
      .toBe(true);
    const afterLoadTop = await readMessageRowTopById(list, edge.rowId!);
    expect(Math.abs(afterLoadTop - edge.rowTop)).toBeLessThanOrEqual(8);
    await dwell(testPage, 750, "negative-assertion", "observe visible-page pagination cascade");
    expect(olderRequests).toHaveLength(1);
  });

  test("continues across a short boundary page and stops outside preload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const { taskId, sessionId } = await seedShortBoundaryPageHistory(
      apiClient,
      seedData,
      "message-pagination-retries-short-page",
    );
    const olderRequests = watchOlderMessageRequests(testPage, sessionId);

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    const chat = session.activeChat();
    const list = chat.locator(".chat-message-list");
    const edge = await scrollToOldestLoadedEdge(list, VISIBLE_PAGE_MARKER);

    await expect(chat.getByText(SHORT_PAGE_BOUNDARY_MARKER, { exact: true })).toBeVisible();
    await expect.poll(() => olderRequests.length).toBe(2);
    const heightAfterShortPage = await list.evaluate((element) => element.scrollHeight);
    expect(heightAfterShortPage - edge.scrollHeight).toBeGreaterThan(0);
    await dwell(testPage, 500, "negative-assertion", "observe short-page pagination stop");
    expect(olderRequests).toHaveLength(2);
  });

  test("counts text parts instead of tool activity per upward reach", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const { taskId, sessionId } = await seedTextSparseMessageHistory(
      apiClient,
      seedData,
      "message-pagination-counts-text-parts",
    );
    const olderRequests = watchOlderMessageRequests(testPage, sessionId);

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    const chat = session.activeChat();
    const list = chat.locator(".chat-message-list");

    await expect(chat.getByText(TEXT_BATCH_MARKER, { exact: true })).toHaveCount(0);
    const edge = await scrollToOldestLoadedEdge(list, TEXT_BATCH_ANCHOR_MARKER);
    expect(Number.isFinite(edge.rowTop)).toBe(true);

    await expect.poll(() => olderRequests.length).toBe(2);
    await expect(chat.getByText(TEXT_BATCH_MARKER, { exact: true })).toBeVisible();
    const afterLoadTop = await readStandaloneMessageTop(list, TEXT_BATCH_ANCHOR_MARKER);
    expect(Math.abs(afterLoadTop - edge.rowTop)).toBeLessThanOrEqual(8);
    await expect(chat.getByTestId("load-older-messages")).toHaveCount(0);
  });

  test("hides the older control when only hidden pre-prompt rows remain", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);

    const { taskId } = await seedCollapsedMessageHistory(
      apiClient,
      seedData,
      "message-pagination-scrolls-to-start",
    );

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    const chat = session.activeChat();
    const list = chat.locator(".chat-message-list");

    await expect(chat.getByText(INITIAL_PROMPT_MARKER, { exact: true })).toBeVisible();
    await expect(chat.getByText(TASK_DESCRIPTION_MARKER, { exact: true })).toHaveCount(0);
    await expect(chat.getByText(PRE_PROMPT_MARKER, { exact: false })).toHaveCount(0);
    await expect(chat.getByTestId("load-older-messages")).toHaveCount(0);

    const edge = await scrollToOldestLoadedEdge(list, INITIAL_PROMPT_MARKER);
    expect(Number.isFinite(edge.rowTop)).toBe(true);
    await expect(chat.getByText(PRE_PROMPT_MARKER, { exact: false })).toHaveCount(0);
    await expect(chat.getByTestId("load-older-messages")).toHaveCount(0);
  });

  test("preserves the prepend anchor while reaching the first prompt", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);

    const { taskId, sessionId } = await seedCollapsedMessageHistory(
      apiClient,
      seedData,
      "message-pagination-preserves-prepend-anchor",
      { promptOutsideInitialWindow: true },
    );
    const olderRequests = watchOlderMessageRequests(testPage, sessionId);

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    const chat = session.activeChat();
    const list = chat.locator(".chat-message-list");

    await expect(chat.getByText(TASK_DESCRIPTION_MARKER, { exact: true })).toHaveCount(0);
    await expect(chat.getByText(INITIAL_PROMPT_MARKER, { exact: true })).toHaveCount(0);

    const edge = await scrollToOldestLoadedEdge(list, RECENT_AGENT_MARKER);
    expect(Number.isFinite(edge.rowTop)).toBe(true);
    await expect(chat.getByText(INITIAL_PROMPT_MARKER, { exact: true })).toBeVisible({
      timeout: 15_000,
    });

    const afterLoadTop = await readStandaloneMessageTop(list, RECENT_AGENT_MARKER);
    expect(Math.abs(afterLoadTop - edge.rowTop)).toBeLessThanOrEqual(8);
    expect(olderRequests.length).toBeGreaterThan(1);

    await expect(chat.getByText(INITIAL_PROMPT_MARKER, { exact: true })).toBeVisible();
    await expect(chat.getByText(TASK_DESCRIPTION_MARKER, { exact: true })).toHaveCount(0);
    await expect(chat.getByTestId("load-older-messages")).toHaveCount(0);
  });
});
