import { test, expect, type Page } from "../../fixtures/test-base";
import { attachGatewayTrafficCapture, type GatewayTrafficFrame } from "../../helpers/ws-traffic";
import { dwell } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";

const RUNNING_WINDOW_MS = 12_000;

function sentMessageListCount(frames: readonly GatewayTrafficFrame[], sessionId: string): number {
  return frames.filter(
    (frame) =>
      frame.direction === "sent" &&
      frame.action === "message.list" &&
      frame.sessionId === sessionId,
  ).length;
}

async function setDocumentVisibility(page: Page, state: "visible" | "hidden"): Promise<void> {
  await page.evaluate((next) => {
    Object.defineProperty(document, "visibilityState", { configurable: true, get: () => next });
    Object.defineProperty(document, "hidden", { configurable: true, get: () => next === "hidden" });
    document.dispatchEvent(new Event("visibilitychange"));
  }, state);
}

test.describe("running message backfill visibility", () => {
  // @covers AC-UI-HIDDEN-RUNNING-BACKFILL-001.1
  // @covers AC-UI-HIDDEN-RUNNING-BACKFILL-001.2
  test("pauses the running refresh while the document is hidden", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const capture = attachGatewayTrafficCapture(testPage);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Hidden running backfill",
      seedData.agentProfileId,
      {
        description: "/slow 60s",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await expect
      .poll(async () => (await apiClient.listTaskSessions(task.id)).sessions[0]?.state ?? null, {
        timeout: 30_000,
        message: "session did not start running",
      })
      .toBe("RUNNING");
    const sessionId = (await apiClient.listTaskSessions(task.id)).sessions[0].id;

    await testPage.goto(`/t/${task.id}`);
    await new SessionPage(testPage).waitForLoad();
    await expect
      .poll(() => sentMessageListCount(capture.frames, sessionId), {
        timeout: 15_000,
        message: "visible running session never refreshed its messages",
      })
      .toBeGreaterThan(1);

    await setDocumentVisibility(testPage, "hidden");
    const hiddenBaseline = sentMessageListCount(capture.frames, sessionId);
    await dwell(
      testPage,
      RUNNING_WINDOW_MS,
      "negative-assertion",
      "the absence of interval ticks has no event to wait on",
    );
    expect(sentMessageListCount(capture.frames, sessionId)).toBe(hiddenBaseline);

    await setDocumentVisibility(testPage, "visible");
    await expect
      .poll(() => sentMessageListCount(capture.frames, sessionId), {
        timeout: 5_000,
        message: "foreground refresh did not run after the document became visible",
      })
      .toBeGreaterThan(hiddenBaseline);
    const foregroundBaseline = sentMessageListCount(capture.frames, sessionId);
    await expect
      .poll(() => sentMessageListCount(capture.frames, sessionId), {
        timeout: 15_000,
        message: "running refresh did not resume after the document became visible",
      })
      .toBeGreaterThan(foregroundBaseline);
    expect((await apiClient.listTaskSessions(task.id)).sessions[0]?.state).toBe("RUNNING");
  });
});
