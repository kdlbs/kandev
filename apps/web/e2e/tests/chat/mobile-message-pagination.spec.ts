import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  INITIAL_PROMPT_MARKER,
  RECENT_AGENT_MARKER,
  TASK_DESCRIPTION_MARKER,
  readStandaloneMessageTop,
  seedCollapsedMessageHistory,
} from "./message-pagination-helpers";

test.describe("Mobile chat message pagination", () => {
  test.describe.configure({ timeout: 180_000 });

  test("reaches the initial prompt through collapsed history by upward scrolling", async ({
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

    await expect(chat.getByText(TASK_DESCRIPTION_MARKER, { exact: true })).toBeVisible();
    await expect(chat.getByText(INITIAL_PROMPT_MARKER, { exact: true })).toHaveCount(0);

    const recentRowTopAtOldestLoadedEdge = await list.evaluate((element, marker) => {
      element.scrollTop = 0;
      element.dispatchEvent(new Event("scroll", { bubbles: true }));
      const row = Array.from(element.querySelectorAll<HTMLElement>("[id^='msg-']")).find(
        (candidate) => candidate.textContent?.includes(marker),
      );
      return row?.getBoundingClientRect().top ?? Number.NaN;
    }, RECENT_AGENT_MARKER);
    expect(Number.isFinite(recentRowTopAtOldestLoadedEdge)).toBe(true);

    await expect
      .poll(() => chat.getByText(INITIAL_PROMPT_MARKER, { exact: true }).count(), {
        timeout: 60_000,
        intervals: [300],
        message: "Loading mobile history until initial prompt",
      })
      .toBe(1);

    await expect(chat.getByText(INITIAL_PROMPT_MARKER, { exact: true })).toBeVisible();
    await expect(chat.getByText(TASK_DESCRIPTION_MARKER, { exact: true })).toHaveCount(0);
    await expect(chat.getByTestId("load-older-messages")).toHaveCount(0);

    const recentRowTopAfterPagination = await readStandaloneMessageTop(list, RECENT_AGENT_MARKER);
    expect(Math.abs(recentRowTopAfterPagination - recentRowTopAtOldestLoadedEdge)).toBeLessThan(8);
  });
});
