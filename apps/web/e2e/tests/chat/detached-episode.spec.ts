// Detached-episode status records render localized copy on desktop.
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const DETACHED = "The turn ended while Kandev was unreachable.";
const PAUSED = "The turn was paused because Kandev stayed unreachable beyond the offline budget.";

test.describe("Detached episode status records (desktop)", () => {
  test("renders the detached and budget-pause notices", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Detached Episode desktop", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, { state: "IDLE" });
    await apiClient.seedSessionMessage(sessionId, {
      type: "status",
      content: "fallback detached",
      metadata: { kind: "turn_ended_detached", episode: "stream-1:1" },
    });
    await apiClient.seedSessionMessage(sessionId, {
      type: "status",
      content: "fallback paused",
      metadata: { kind: "paused_offline_budget", episode: "stream-1:2" },
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const chat = session.activeChat();
    await expect(chat.getByText(DETACHED, { exact: true })).toBeVisible({ timeout: 15_000 });
    await expect(chat.getByText(PAUSED, { exact: true })).toBeVisible();
  });
});
