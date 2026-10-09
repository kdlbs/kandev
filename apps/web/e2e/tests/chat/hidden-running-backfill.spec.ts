import { test, expect, type Page } from "../../fixtures/test-base";
import { attachGatewayTrafficCapture, type GatewayTrafficFrame } from "../../helpers/ws-traffic";
import { dwell } from "../../helpers/causal-waits";
import { waitForSessionDone, waitForSessionState } from "../../helpers/session";
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
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");
    const sessionId = task.session_id;
    await waitForSessionDone(apiClient, task.id, sessionId, "Initial conversation turn settled");
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(
      session.chat.getByText("This is a simple mock response for e2e testing.", { exact: true }),
    ).toBeVisible();
    await expect(
      session.chat.locator('[data-placeholder="Continue working on the task..."]'),
    ).toBeVisible();
    const editor = session.chat.locator(".tiptap.ProseMirror:visible").first();
    await expect(editor).toBeEditable();
    // Establish history before a quiet turn so startup and tool reads cannot
    // be mistaken for ticks from the running backfill timer.
    await editor.fill("/sleep 60");
    await session.submitMessageWithKeyboard(editor);
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId,
      expectedState: "RUNNING",
      message: "Quiet turn did not start running",
      timeout: 30_000,
    });
    const visibleBaseline = sentMessageListCount(capture.frames, sessionId);
    await expect
      .poll(() => sentMessageListCount(capture.frames, sessionId), {
        timeout: 15_000,
        message: "visible running session never refreshed its messages",
      })
      .toBeGreaterThan(visibleBaseline);

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
