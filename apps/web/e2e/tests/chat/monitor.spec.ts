import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

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
    test.setTimeout(60_000);

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

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Monitor watching",
      seedData.agentProfileId,
      {
        description: script,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // The dedicated Monitor card renders, not the generic tool_call row.
    const monitorCard = session.chat.locator('[data-testid="monitor-card"]').first();
    await expect(monitorCard).toBeVisible();
    await expect(monitorCard).toContainText("gh pr checks --watch");

    // Event count badge surfaces all three events. Status pill flipped to
    // "ended" because the script issued monitor_end and the parent turn
    // completed.
    await expect(monitorCard).toContainText("3 events");
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
    test.setTimeout(45_000);

    const script = [
      'e2e:monitor_start("task-1", "tail -f log")',
      "e2e:delay(100)",
      'e2e:monitor_event("task-1", "first and only line")',
      'e2e:monitor_end("task-1")',
      'e2e:message("done")',
    ].join("\n");

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Monitor singular",
      seedData.agentProfileId,
      {
        description: script,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    const card = session.chat.locator('[data-testid="monitor-card"]').first();
    await expect(card).toContainText("1 event");
    await expect(card).not.toContainText("1 events");
  });

  test("page reload preserves the monitor card and recent events", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    const script = [
      'e2e:monitor_start("task-1", "wait for ci")',
      "e2e:delay(150)",
      'e2e:monitor_event("task-1", "step-1")',
      "e2e:delay(150)",
      'e2e:monitor_event("task-1", "step-2")',
      'e2e:monitor_end("task-1")',
      'e2e:message("done")',
    ].join("\n");

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Monitor reload",
      seedData.agentProfileId,
      {
        description: script,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await expect(session.chat.locator('[data-testid="monitor-card"]').first()).toContainText(
      "2 events",
    );

    // Reload — SSR + Zustand hydration must reconstitute the card from DB.
    await testPage.reload();
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    const card = session.chat.locator('[data-testid="monitor-card"]').first();
    await expect(card).toBeVisible();
    await expect(card).toContainText("wait for ci");
    await expect(card).toContainText("2 events");
    await expect(session.chat.locator('[data-testid="monitor-event"]')).toHaveCount(2);
  });
});
