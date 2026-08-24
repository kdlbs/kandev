// Chat message pagination — upward scrolling walks a collapsed conversation
// all the way back to the first stored prompt without repeated button actions.
// Covers the native transcript renderer and its prepend scroll anchoring.
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  INITIAL_PROMPT_MARKER,
  RECENT_AGENT_MARKER,
  TASK_DESCRIPTION_MARKER,
  readStandaloneMessageTop,
  seedCollapsedMessageHistory,
  scrollToOldestLoadedEdge,
} from "./message-pagination-helpers";

test.describe("@chat message pagination", () => {
  test("upward scrolling reaches the initial prompt through collapsed history", async ({
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

    await expect(chat.getByText(TASK_DESCRIPTION_MARKER, { exact: true })).toBeVisible();
    await expect(chat.getByText(INITIAL_PROMPT_MARKER, { exact: true })).toHaveCount(0);

    // Each upward gesture reaches the current oldest loaded edge. Older pages
    // may only extend a collapsed activity row, so keep scrolling until the
    // stored prompt appears. The row-position assertion belongs to each load:
    // it verifies prepend anchoring without treating the user's next upward
    // gesture as an anchoring regression.
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
