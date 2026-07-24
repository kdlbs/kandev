// Mobile companion to message-timestamp-tooltip.spec.ts. Native title
// tooltips never fire on touch (no hover event), so coarse pointers get a
// tap-to-open Drawer that surfaces the same absolute time instead (see
// MessageTimestamp in message-actions.tsx).
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const SEEDED_MESSAGE = "Tooltip regression fixture message";

test.describe("Chat message timestamp tooltip (mobile)", () => {
  test("tapping the relative timestamp opens a drawer with the full absolute time", async ({
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
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const chat = session.activeChat();
    await expect(chat.getByText(SEEDED_MESSAGE)).toBeVisible({ timeout: 15_000 });

    const timestamp = chat.locator("time[datetime]").last();
    await expect(timestamp).toBeVisible();
    const dateTimeAttr = await timestamp.getAttribute("datetime");

    // Compute the expected absolute time in the browser's own context so the
    // comparison isn't sensitive to the test runner's locale/timezone.
    const expectedAbsoluteTime = await testPage.evaluate(
      (iso) => new Date(iso as string).toLocaleString(),
      dateTimeAttr,
    );

    const trigger = chat.getByTestId("message-timestamp-trigger").last();
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
