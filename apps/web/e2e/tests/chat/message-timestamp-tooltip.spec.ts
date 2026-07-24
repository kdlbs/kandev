// Regression guard: hovering a chat message's relative timestamp ("5m ago")
// must reveal the full absolute time via the native browser tooltip, backed
// by an HTML <time title="..."> element (see message-actions.tsx).
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const SEEDED_MESSAGE = "Tooltip regression fixture message";

test.describe("Chat message timestamp tooltip", () => {
  test("shows the full absolute time as the title of the relative timestamp", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Timestamp Tooltip", {
      description: "seeded timestamp tooltip fixture",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "IDLE",
    });
    await apiClient.seedSessionMessage(sessionId, {
      type: "message",
      content: SEEDED_MESSAGE,
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const chat = session.activeChat();
    await expect(chat.getByText(SEEDED_MESSAGE)).toBeVisible({ timeout: 15_000 });

    // Scope to the seeded message's own row so a later message/footer with
    // its own <time> can't satisfy the assertions without the seeded row.
    const messageRow = chat
      .locator("[data-agent-message-body][data-message-id]")
      .filter({ hasText: SEEDED_MESSAGE })
      .locator("xpath=..");
    const timestamp = messageRow.locator("time[datetime]");
    await timestamp.hover();
    await testPage.waitForTimeout(1500);
    await prCapture.screenshot("message-relative-timestamp-hover", {
      caption:
        "Chat message footer with the relative timestamp hovered. The native " +
        "browser tooltip revealing the full absolute time is browser chrome, " +
        "not page content, so it isn't visible in a static capture — its " +
        "exact value is asserted below instead.",
    });
    const dateTimeAttr = await timestamp.getAttribute("datetime");
    const titleAttr = await timestamp.getAttribute("title");
    expect(dateTimeAttr).toBeTruthy();

    // Compute the expected absolute time in the browser's own context so the
    // comparison isn't sensitive to the test runner's locale/timezone.
    const expectedTitle = await testPage.evaluate(
      (iso) => new Date(iso as string).toLocaleString(),
      dateTimeAttr,
    );
    expect(titleAttr).toBe(expectedTitle);
    // The tooltip must be the full timestamp, not a repeat of the relative label.
    expect(titleAttr).not.toMatch(/ago$/);
  });
});
