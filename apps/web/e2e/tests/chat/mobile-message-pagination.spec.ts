import { test, expect } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";
import {
  EAGER_HISTORY_PROMPT_MARKER,
  INITIAL_PROMPT_MARKER,
  PRE_PROMPT_MARKER,
  RECENT_AGENT_MARKER,
  SHORT_PAGE_BOUNDARY_MARKER,
  TASK_DESCRIPTION_MARKER,
  VISIBLE_PAGE_MARKER,
  readMessageRowTopById,
  readStandaloneMessageTop,
  seedCollapsedMessageHistory,
  seedShortBoundaryPageHistory,
  seedToolHeavyOpeningHistory,
  seedVisibleMessageHistory,
  scrollToOldestLoadedEdge,
  scrollUpSlightly,
  watchOlderMessageRequests,
} from "./message-pagination-helpers";

test.describe("Mobile chat message pagination", () => {
  test.describe.configure({ timeout: 180_000 });

  test("does not load older history while opening a task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { taskId, sessionId } = await seedToolHeavyOpeningHistory(
      apiClient,
      seedData,
      "mobile-message-pagination-does-not-eager-load",
    );
    const olderRequests = watchOlderMessageRequests(testPage, sessionId);

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    await dwell(testPage, 500, "negative-assertion", "observe mobile pagination after open");
    const chat = session.activeChat();

    await expect(chat.getByText(TASK_DESCRIPTION_MARKER, { exact: true })).toBeVisible();
    await expect(chat.getByText(EAGER_HISTORY_PROMPT_MARKER, { exact: true })).toHaveCount(0);
    expect(olderRequests).toHaveLength(0);
  });

  test("loads one visible page per upward reach without cascading", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { taskId, sessionId } = await seedVisibleMessageHistory(
      apiClient,
      seedData,
      "mobile-message-pagination-does-not-cascade",
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
        { timeout: 15_000, intervals: [100], message: "One older mobile page loaded" },
      )
      .toBe(true);
    const afterLoadTop = await readMessageRowTopById(list, edge.rowId!);
    expect(Math.abs(afterLoadTop - edge.rowTop)).toBeLessThanOrEqual(8);
    await dwell(testPage, 750, "negative-assertion", "observe mobile pagination cascade");
    expect(olderRequests).toHaveLength(1);
  });

  test("retries a short boundary page on the next upward movement", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { taskId, sessionId } = await seedShortBoundaryPageHistory(
      apiClient,
      seedData,
      "mobile-message-pagination-retries-short-page",
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
    await expect.poll(() => olderRequests.length).toBe(1);
    const heightAfterShortPage = await list.evaluate((element) => element.scrollHeight);
    expect(heightAfterShortPage - edge.scrollHeight).toBeGreaterThan(0);
    expect(heightAfterShortPage - edge.scrollHeight).toBeLessThan(200);
    await dwell(testPage, 500, "negative-assertion", "observe mobile short-page stop");
    expect(olderRequests).toHaveLength(1);

    expect(await scrollUpSlightly(list)).toBeGreaterThan(0);
    await expect.poll(() => olderRequests.length, { timeout: 15_000 }).toBe(2);
  });

  test("hides the older control when only hidden pre-prompt rows remain", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { taskId } = await seedCollapsedMessageHistory(
      apiClient,
      seedData,
      "mobile-message-pagination-scrolls-to-start",
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
    const { taskId, sessionId } = await seedCollapsedMessageHistory(
      apiClient,
      seedData,
      "mobile-message-pagination-preserves-prepend-anchor",
      { promptOutsideInitialWindow: true },
    );
    const olderRequests = watchOlderMessageRequests(testPage, sessionId);

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    const chat = session.activeChat();
    const list = chat.locator(".chat-message-list");

    await expect(chat.getByText(TASK_DESCRIPTION_MARKER, { exact: true })).toBeVisible();
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
