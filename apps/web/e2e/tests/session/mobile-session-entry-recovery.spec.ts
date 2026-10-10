import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { openTaskSession } from "../../helpers/session";
import {
  createSettledHistoryTask,
  routeSessionEntryRecovery,
} from "../../helpers/session-entry-recovery";

test.describe("mobile session entry recovery", () => {
  test.describe.configure({ retries: 0 });

  test("keeps cached access while history recovers with phone-sized controls", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(150_000);
    const proxy = await routeSessionEntryRecovery(testPage);
    const task = await createSettledHistoryTask(
      apiClient,
      seedData,
      `Mobile history recovery ${Date.now()}`,
    );

    proxy.holdResponses("message.list", { sessionId: task.session_id });

    const session = await openTaskSession(testPage, task.id);
    const chat = session.activeChat();
    const historyNotice = chat.getByTestId("session-history-unavailable");
    await expect
      .poll(() => proxy.heldResponseCount("message.list"), {
        timeout: 30_000,
        message: "Waiting for the session history response to be held",
      })
      .toBeGreaterThan(0);
    await expect(chat.getByTestId("chat-input-editor-shell")).toBeVisible();
    await expect(historyNotice).toHaveCount(0);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile session history recovery");

    proxy.releaseHeldResponses("message.list");
    await expect(chat).toContainText("simple mock response", { timeout: 30_000 });
    expect(proxy.heldResponseCount("message.list")).toBeGreaterThan(0);
  });

  test("recovers from a history request failure with phone-sized controls", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(150_000);
    const proxy = await routeSessionEntryRecovery(testPage);
    const task = await createSettledHistoryTask(
      apiClient,
      seedData,
      `Mobile history failure recovery ${Date.now()}`,
    );

    proxy.failResponses("message.list", "Injected history read failure");

    const session = await openTaskSession(testPage, task.id);
    const chat = session.activeChat();
    const historyNotice = chat.getByTestId("session-history-unavailable");
    await expect(historyNotice).toBeVisible({ timeout: 45_000 });

    const retry = historyNotice.getByTestId("session-history-retry");
    const retryBox = await retry.boundingBox();
    expect(retryBox?.height).toBeGreaterThanOrEqual(44);
    const detailsSummary = historyNotice.getByTestId("session-history-details-summary");
    const detailsBox = await detailsSummary.boundingBox();
    expect(detailsBox?.height).toBeGreaterThanOrEqual(44);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile session history failure recovery");

    await expect.poll(() => proxy.pendingRequestCount("message.list")).toBe(0);
    proxy.allowResponses("message.list");
    const requestsBeforeRetry = proxy.requestCount("message.list");
    await retry.click();
    await expect
      .poll(() => proxy.requestCount("message.list"))
      .toBeGreaterThan(requestsBeforeRetry);
    await expect(historyNotice).toHaveCount(0);
    await expect(chat).toContainText("simple mock response", { timeout: 30_000 });
    expect(proxy.requestCount("message.list")).toBeGreaterThanOrEqual(2);
    expect(proxy.failedResponseCount("message.list")).toBeGreaterThan(0);
  });
});
