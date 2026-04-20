import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

/**
 * Verifies the session.focus WS protocol added for focus-gated git polling.
 *
 * - Sidebar cards subscribe but do NOT focus -> only get slow polling.
 * - Opening a task details page sends session.focus -> backend lifts to fast.
 *
 * We assert at the WS frame level (cheaper, more deterministic than asserting
 * polling cadence in agentctl).
 *
 * Note: unfocus (cleanup on unmount) is covered by the useTaskFocus unit test.
 * E2E can't reliably test it because Playwright's page.goto is a full-page
 * navigation that tears down the WS before React effects clean up.
 */
test.describe("Session focus signal", () => {
  test("task details page sends subscribe and focus", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(60_000);

    // Capture sent WS frames from the moment the page opens.
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

    // Visit the task page — this triggers useTaskFocus(sessionId) on mount.
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 45_000 });

    // Poll for the expected WS frames. The page mount + auto-start + WS
    // connect sequence races, so useTaskFocus may fire a few hundred ms
    // after the agent goes idle. expect.poll handles this gracefully.
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
  });
});
