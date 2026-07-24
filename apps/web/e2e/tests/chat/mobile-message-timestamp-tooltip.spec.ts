// Mobile companion to message-timestamp-tooltip.spec.ts. Native title
// tooltips never fire on touch (no hover event), so mobile always shows the
// message action bar — including the <time title> timestamp — without
// needing a hover. This only captures that always-visible layout for the PR;
// the absolute-time <time title>/dateTime assertions live in the desktop spec.
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const SEEDED_MESSAGE = "Tooltip regression fixture message";

test.describe("Chat message timestamp tooltip (mobile)", () => {
  test("shows the timestamp in the always-visible mobile action bar", async ({
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
    await expect(timestamp).toHaveAttribute("title", /.+/);

    await prCapture.screenshot("message-relative-timestamp-mobile", {
      caption:
        "Mobile chat: the action bar (and its <time title> timestamp) is " +
        "always visible — touch has no hover, so the tooltip trigger is " +
        "moot here, but the same dateTime/title attributes are present.",
    });
  });
});
