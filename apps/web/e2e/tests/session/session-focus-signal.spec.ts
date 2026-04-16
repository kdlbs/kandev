import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

/**
 * Verifies the session.focus / session.unfocus WS protocol added for the
 * focus-gated git polling work.
 *
 * - Sidebar cards subscribe but do NOT focus → only get slow polling.
 * - Opening a task details page sends session.focus → backend lifts to fast.
 * - Leaving the task details page sends session.unfocus → backend drops back.
 *
 * We assert at the WS frame level (cheaper, more deterministic than asserting
 * polling cadence in agentctl).
 */
test.describe("Session focus signal", () => {
  test("task details page sends focus and unfocus", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(60_000);

    // Capture sent WS frames from the moment the page opens. Playwright's
    // websocket event fires for every WS connection the page opens; we keep
    // every frame so we can poll for specific actions to appear.
    const sentFrames: string[] = [];
    testPage.on("websocket", (ws) => {
      ws.on("framesent", (event) => {
        const data = typeof event.payload === "string" ? event.payload : event.payload?.toString();
        if (data) sentFrames.push(data);
      });
    });

    const countFrames = (action: string) =>
      sentFrames.filter((f) => f.includes(`"action":"${action}"`)).length;

    const task = await apiClient.createTask(seedData.workspaceId, "Focus Signal Task", {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      agent_profile_id: seedData.agentProfileId,
    });

    // Visit the task page — this triggers useTaskFocus(sessionId) on mount,
    // which fires session.focus once both the WS is connected and the session
    // ID has propagated into the store via the auto-start hook.
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    // Wait for the agent's reply so we know the session has fully booted.
    await expect(session.idleInput()).toBeVisible({ timeout: 45_000 });

    // The page mount + auto-start + WS connect sequence races each other and
    // useTaskFocus only fires once both effectiveSessionId and connectionStatus
    // settle, which can be a few hundred ms after the agent goes idle. Poll
    // rather than asserting on a single snapshot.
    await expect
      .poll(() => countFrames("session.subscribe"), {
        message: "expected at least one session.subscribe frame",
        timeout: 10_000,
      })
      .toBeGreaterThan(0);

    await expect
      .poll(() => countFrames("session.focus"), {
        message: "expected at least one session.focus frame from task page",
        timeout: 10_000,
      })
      .toBeGreaterThan(0);

    // Navigate away — useTaskFocus cleanup should fire unfocus.
    await testPage.goto("/");

    await expect
      .poll(() => countFrames("session.unfocus"), {
        message: "expected session.unfocus frame after navigating away",
        timeout: 10_000,
      })
      .toBeGreaterThan(0);
  });
});
