// Hovering a transcript timestamp exposes its absolute-short counterpart.
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { dwell } from "../../helpers/causal-waits";
const CREATED_AT = "2026-06-20T10:15:00Z";

test.use({ locale: "en-US", timezoneId: "UTC" });

const SEEDED_MESSAGE = "Tooltip regression fixture message";

test.describe("Chat message timestamp tooltip", () => {
  test("shows the absolute short time as the relative timestamp title", async ({
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
      createdAt: CREATED_AT,
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
    await dwell(
      testPage,
      1500,
      "browser-chrome",
      "Chrome's native title-tooltip delay is browser chrome rather than DOM, so there is nothing in the page to observe; kept so the capture below shows the same hover dwell a real user would see",
    );
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

    const expectedTitle = await testPage.evaluate(
      (iso) =>
        new Intl.DateTimeFormat("en-US", { dateStyle: "short", timeStyle: "short" }).format(
          new Date(iso as string),
        ),
      dateTimeAttr,
    );
    const expectedLabel = await testPage.evaluate(
      (iso) =>
        new Intl.DateTimeFormat("en-US", {
          year: "numeric",
          month: "numeric",
          day: "numeric",
        }).format(new Date(iso as string)),
      dateTimeAttr,
    );
    expect(titleAttr).toBe(expectedTitle);
    await expect(timestamp).toHaveText(expectedLabel);
    expect(titleAttr).not.toBe(expectedLabel);
  });
});
