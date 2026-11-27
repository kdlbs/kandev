// Mobile companion: tap the timestamp to open a drawer with its counterpart.
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
const CREATED_AT = new Date(Date.now() - 60_000).toISOString();

test.use({ locale: "en-US", timezoneId: "UTC" });

const SEEDED_MESSAGE = "Tooltip regression fixture message";

test.describe("Chat message timestamp tooltip (mobile)", () => {
  test("tapping the relative timestamp opens a drawer with the absolute short time", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Timestamp Tooltip Mobile", {
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
    // its own <time>/trigger can't satisfy the assertions without the
    // seeded row.
    const messageRow = chat
      .locator("[data-agent-message-body][data-message-id]")
      .filter({ hasText: SEEDED_MESSAGE })
      .locator("xpath=..");
    const timestamp = messageRow.locator("time[datetime]");
    await expect(timestamp).toBeVisible();
    const dateTimeAttr = await timestamp.getAttribute("datetime");

    const expectedAbsoluteTime = await testPage.evaluate(
      (iso) =>
        new Intl.DateTimeFormat("en-US", { dateStyle: "short", timeStyle: "short" }).format(
          new Date(iso as string),
        ),
      dateTimeAttr,
    );

    const trigger = messageRow.getByTestId("message-timestamp-trigger");
    await expect(trigger).toBeVisible();

    const drawer = testPage.getByTestId("message-timestamp-drawer");
    await expect(drawer).toHaveCount(0);
    await trigger.tap();
    await expect(drawer).toBeVisible({ timeout: 5_000 });
    await expect(drawer.getByText(expectedAbsoluteTime, { exact: true })).toBeVisible();

    await prCapture.screenshot("message-relative-timestamp-mobile-drawer", {
      caption:
        "Mobile chat: tapping the relative timestamp opens a drawer that " +
        "reveals the full absolute time, since native title tooltips never " +
        "fire on touch.",
    });
  });
});
