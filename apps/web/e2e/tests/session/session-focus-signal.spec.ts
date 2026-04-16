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
    // all frames keyed by URL to avoid mixing up the gateway socket with any
    // other (e.g. shell terminal) sockets.
    const sentByUrl = new Map<string, string[]>();
    testPage.on("websocket", (ws) => {
      const url = ws.url();
      sentByUrl.set(url, sentByUrl.get(url) ?? []);
      ws.on("framesent", (event) => {
        const data = typeof event.payload === "string" ? event.payload : event.payload?.toString();
        if (!data) return;
        sentByUrl.get(url)?.push(data);
      });
    });

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
    // Wait for the agent's reply so we know the session has fully booted and
    // the page-level focus effect has had a chance to run.
    await expect(session.idleInput()).toBeVisible({ timeout: 45_000 });

    // Find the gateway socket by looking for one that ever sent a session.subscribe.
    const allFrames = () => Array.from(sentByUrl.values()).flat();
    const frames = allFrames();
    const subscribeCount = frames.filter((f) => f.includes('"action":"session.subscribe"')).length;
    const focusCount = frames.filter((f) => f.includes('"action":"session.focus"')).length;

    expect(subscribeCount, "expected at least one session.subscribe frame").toBeGreaterThan(0);
    expect(focusCount, "expected at least one session.focus frame from task page").toBeGreaterThan(
      0,
    );

    // Navigate away from the task — useTaskFocus cleanup should fire unfocus.
    await testPage.goto("/");
    // Give React's effect cleanup + WS send a moment to flush.
    await testPage.waitForTimeout(500);

    const framesAfter = allFrames();
    const unfocusCount = framesAfter.filter((f) => f.includes('"action":"session.unfocus"')).length;
    expect(
      unfocusCount,
      "expected session.unfocus frame after navigating away from task page",
    ).toBeGreaterThan(0);
  });
});
