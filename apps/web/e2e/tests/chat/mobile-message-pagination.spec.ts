import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  INITIAL_PROMPT_MARKER,
  PRE_PROMPT_MARKER,
  TASK_DESCRIPTION_MARKER,
  seedCollapsedMessageHistory,
  scrollToOldestLoadedEdge,
} from "./message-pagination-helpers";

test.describe("Mobile chat message pagination", () => {
  test.describe.configure({ timeout: 180_000 });

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
});
