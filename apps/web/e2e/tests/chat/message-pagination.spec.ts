// Chat message pagination — the first stored prompt is the visible transcript
// start even when older internal rows remain on the backend.
import { test, expect } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";
import {
  EAGER_HISTORY_PROMPT_MARKER,
  INITIAL_PROMPT_MARKER,
  PRE_PROMPT_MARKER,
  RECENT_AGENT_MARKER,
  TASK_DESCRIPTION_MARKER,
  readStandaloneMessageTop,
  seedCollapsedMessageHistory,
  seedToolHeavyOpeningHistory,
  scrollToOldestLoadedEdge,
  watchOlderMessageRequests,
} from "./message-pagination-helpers";

test.describe("@chat message pagination", () => {
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

    await expect(chat.getByText(TASK_DESCRIPTION_MARKER, { exact: true })).toBeVisible();
    await expect(chat.getByText(EAGER_HISTORY_PROMPT_MARKER, { exact: true })).toHaveCount(0);
    expect(olderRequests).toHaveLength(0);
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

    const { taskId } = await seedCollapsedMessageHistory(
      apiClient,
      seedData,
      "message-pagination-preserves-prepend-anchor",
      { promptOutsideInitialWindow: true },
    );

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    const chat = session.activeChat();
    const list = chat.locator(".chat-message-list");

    await expect(chat.getByText(TASK_DESCRIPTION_MARKER, { exact: true })).toBeVisible();
    await expect(chat.getByText(INITIAL_PROMPT_MARKER, { exact: true })).toHaveCount(0);

    for (let attempt = 0; attempt < 10; attempt += 1) {
      const edge = await scrollToOldestLoadedEdge(list, RECENT_AGENT_MARKER);
      expect(Number.isFinite(edge.rowTop)).toBe(true);

      await expect
        .poll(
          async () =>
            (await chat.getByText(INITIAL_PROMPT_MARKER, { exact: true }).count()) === 1 ||
            (await list.evaluate((element) => element.scrollHeight)) > edge.scrollHeight,
          {
            timeout: 15_000,
            intervals: [300],
            message: "Loading older pages until initial prompt",
          },
        )
        .toBe(true);

      const afterLoadTop = await readStandaloneMessageTop(list, RECENT_AGENT_MARKER);
      expect(Math.abs(afterLoadTop - edge.rowTop)).toBeLessThanOrEqual(8);
      if ((await chat.getByText(INITIAL_PROMPT_MARKER, { exact: true }).count()) === 1) break;
    }

    await expect(chat.getByText(INITIAL_PROMPT_MARKER, { exact: true })).toBeVisible();
    await expect(chat.getByText(TASK_DESCRIPTION_MARKER, { exact: true })).toHaveCount(0);
    await expect(chat.getByTestId("load-older-messages")).toHaveCount(0);
  });
});
