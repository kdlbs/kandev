import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";
import { attachGatewayTrafficCapture } from "../../helpers/ws-traffic";

async function prepareMonitorTask(apiClient: ApiClient, seedData: SeedData, title: string) {
  const task = await apiClient.createTask(seedData.workspaceId, title, {
    agent_profile_id: seedData.agentProfileId,
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  const prepared = await apiClient.launchSession({
    task_id: task.id,
    agent_profile_id: seedData.agentProfileId,
    executor_profile_id: seedData.worktreeExecutorProfileId,
    workflow_step_id: seedData.startStepId,
    prompt: "",
    intent: "prepare",
    launch_workspace: true,
  });

  await expect
    .poll(async () => (await apiClient.getTaskEnvironment(task.id))?.status ?? null, {
      timeout: 60_000,
      message: "prepared Monitor task environment did not become ready",
    })
    .toBe("ready");

  return { task, sessionId: prepared.session_id };
}

async function waitForMonitorSubscription(
  capture: ReturnType<typeof attachGatewayTrafficCapture>,
  sessionId: string,
) {
  await expect
    .poll(
      () =>
        capture.frames.some(
          (frame) =>
            frame.direction === "sent" &&
            frame.action === "session.subscribe" &&
            frame.sessionId === sessionId,
        ),
      { timeout: 10_000, message: "Monitor page must subscribe before the script starts" },
    )
    .toBe(true);
  await expect
    .poll(
      () =>
        capture.frames.some(
          (frame) =>
            frame.direction === "received" &&
            frame.action === "session.subscribe" &&
            frame.sessionId === sessionId,
        ),
      { timeout: 30_000, message: "Monitor page did not acknowledge its session subscription" },
    )
    .toBe(true);
}

// All Monitor scenarios drive the kandev backend with the mock-agent's new
// e2e:monitor_* directives, which reproduce claude-agent-acp's wire format
// without depending on the real Claude Code SDK. The kandev ACP adapter
// recognizes the registration banner, parses task-notification envelopes,
// strips the model's "Human:" echoes, and tracks live monitor state — those
// behaviours are unit-tested in the backend; this file asserts the full
// pipeline lands the right thing in the chat UI.

test.describe("Claude-acp Monitor tool", () => {
  test("renders watching card, accumulates events, hides envelope text", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    // Three monitor events fire over ~3s. The agent then ends the monitor and
    // produces a final assistant message so the turn completes deterministically.
    const script = [
      'e2e:monitor_start("task-1", "gh pr checks --watch")',
      "e2e:delay(200)",
      'e2e:monitor_event("task-1", "queued: lint")',
      "e2e:delay(200)",
      'e2e:monitor_event("task-1", "in_progress: lint")',
      "e2e:delay(200)",
      'e2e:monitor_event("task-1", "success: lint")',
      'e2e:monitor_end("task-1")',
      'e2e:message("watching done")',
    ].join("\n");

    const { task, sessionId } = await prepareMonitorTask(apiClient, seedData, "Monitor watching");

    const capture = attachGatewayTrafficCapture(testPage);
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    await waitForMonitorSubscription(capture, sessionId);
    capture.frames.length = 0;
    // The browser send path records the first user turn before launching the
    // agent, so fast Monitor notifications have an active turn to update.
    await session.sendMessageViaButton(script);

    // The dedicated Monitor card renders, not the generic tool_call row.
    const monitorCard = session.chat.locator('[data-testid="monitor-card"]').first();
    await expect(monitorCard).toBeVisible();
    await expect(monitorCard).toContainText("gh pr checks --watch");

    // Event count badge surfaces all three events. Status pill flipped to
    // "ended" because the script issued monitor_end and the parent turn
    // completed.
    await expect(monitorCard).toContainText("3 events", { timeout: 30_000 });
    await expect(session.chat.locator('[data-testid="monitor-status-pill"]').first()).toContainText(
      /ended/,
    );

    // Each event body landed in the recent-events tail.
    const eventList = session.chat.locator('[data-testid="monitor-event"]');
    await expect(eventList).toHaveCount(3);
    await expect(eventList.nth(0)).toContainText("queued: lint");
    await expect(eventList.nth(2)).toContainText("success: lint");

    // Critical: the model's "Human: <task-notification>" echo must NOT appear
    // anywhere in the chat. The adapter strips matched envelopes from the
    // assistant text and drops orphan "Human:" prefixes entirely.
    await expect(session.chat).not.toContainText("<task-notification>");
    await expect(session.chat).not.toContainText("Human: <task");
  });

  test("singular event count uses 'event' not 'events'", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const script = [
      'e2e:monitor_start("task-1", "tail -f log")',
      "e2e:delay(100)",
      'e2e:monitor_event("task-1", "first and only line")',
      'e2e:monitor_end("task-1")',
      'e2e:message("done")',
    ].join("\n");

    const { task, sessionId } = await prepareMonitorTask(apiClient, seedData, "Monitor singular");

    const capture = attachGatewayTrafficCapture(testPage);
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    await waitForMonitorSubscription(capture, sessionId);
    await session.sendMessageViaButton(script);
    const card = session.chat.locator('[data-testid="monitor-card"]').first();
    await expect(card).toContainText("1 event", { timeout: 30_000 });
    await expect(card).not.toContainText("1 events");
  });

  test("page reload preserves the monitor card and recent events", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const script = [
      'e2e:monitor_start("task-1", "wait for ci")',
      "e2e:delay(150)",
      'e2e:monitor_event("task-1", "step-1")',
      "e2e:delay(150)",
      'e2e:monitor_event("task-1", "step-2")',
      'e2e:monitor_end("task-1")',
      'e2e:message("done")',
    ].join("\n");

    const { task, sessionId } = await prepareMonitorTask(apiClient, seedData, "Monitor reload");

    const capture = attachGatewayTrafficCapture(testPage);
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    await waitForMonitorSubscription(capture, sessionId);
    await session.sendMessageViaButton(script);
    await expect(session.chat.locator('[data-testid="monitor-card"]').first()).toContainText(
      "2 events",
      { timeout: 30_000 },
    );

    // Reload — SSR + Zustand hydration must reconstitute the card from DB.
    await testPage.reload();
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });

    const card = session.chat.locator('[data-testid="monitor-card"]').first();
    await expect(card).toBeVisible();
    await expect(card).toContainText("wait for ci");
    await expect(card).toContainText("2 events");
    await expect(session.chat.locator('[data-testid="monitor-event"]')).toHaveCount(2);
  });
});
