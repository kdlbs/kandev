import { test, expect } from "../../fixtures/office-fixture";

/**
 * Pins fixes shipped for the agent comment thread:
 *
 *  - Agent name: bridged session comments must render the agent's
 *    real name (e.g. "CEO"), not the literal string "Agent". The bug
 *    was a hardcoded fallback in the page-level mapper.
 *
 *  - Per-turn dedup: two session-bridged comments with different
 *    bodies for the same (task, agent) must both appear. The bug
 *    was a per-task dedup that suppressed every turn after the first.
 *
 *  - Per-turn collapsible: each session-bridged agent comment owns an
 *    inline expandable, distinct from the legacy session-wide entry.
 */

test.describe("Office task comments", () => {
  test("bridged agent comment renders the agent's real name (not 'Agent')", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Comment name resolution", {
      workflow_id: officeSeed.workflowId,
    });
    await apiClient.rawRequest("PATCH", `/api/v1/office/tasks/${task.id}`, {
      assignee_agent_profile_id: officeSeed.agentId,
    });
    // Seed a session so the chat embed has somewhere to attach.
    await apiClient.seedTaskSession(task.id, {
      state: "IDLE",
      agentProfileId: officeSeed.agentId,
      startedAt: new Date(Date.now() - 60_000).toISOString(),
    });
    await apiClient.seedComment({
      taskId: task.id,
      authorType: "agent",
      authorId: officeSeed.agentId,
      body: "First reply from the assigned agent.",
      source: "session",
    });

    await testPage.goto(`/office/tasks/${task.id}`);

    // The seeded agent name on the office fixture is "CEO".
    // The comment body must be visible (so we know the comment rendered)
    // AND the rendered author name must be "CEO" — not the legacy
    // hardcoded "Agent" string.
    await expect(testPage.getByText("First reply from the assigned agent.")).toBeVisible({
      timeout: 10_000,
    });

    // The bridged comment must render the agent's real name in its
    // author header. Walk to the comment container, then assert the
    // adjacent author label contains "CEO" — not the legacy "Agent" string.
    // Pre-fix, the page-level mapper hardcoded "Agent" for every
    // agent comment regardless of which agent authored it.
    //
    // Use a regex match because other tests in the same worker may have
    // renamed the CEO (e.g. agents.spec.ts:14 sets it to "CEO Updated"),
    // and the worker-scoped office agent persists across tests.
    const commentBlock = testPage
      .getByText("First reply from the assigned agent.")
      .locator("xpath=ancestor::div[contains(@class,'flex-1')][1]");
    await expect(commentBlock.locator("span.font-medium").first()).toHaveText(/CEO/i);
    await expect(commentBlock.locator("span.font-medium").first()).not.toHaveText(/^Agent$/);
  });

  test("two turns on the same session render as two separate comments", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Per-turn comment rendering", {
      workflow_id: officeSeed.workflowId,
    });
    await apiClient.rawRequest("PATCH", `/api/v1/office/tasks/${task.id}`, {
      assignee_agent_profile_id: officeSeed.agentId,
    });
    await apiClient.seedTaskSession(task.id, {
      state: "IDLE",
      agentProfileId: officeSeed.agentId,
      startedAt: new Date(Date.now() - 120_000).toISOString(),
    });

    // Turn 1 (older).
    await apiClient.seedComment({
      taskId: task.id,
      authorType: "agent",
      authorId: officeSeed.agentId,
      body: "Turn 1 — initial analysis.",
      source: "session",
      createdAt: new Date(Date.now() - 60_000).toISOString(),
    });
    // User reply between turns.
    await apiClient.seedComment({
      taskId: task.id,
      authorType: "user",
      authorId: "user",
      body: "tell me the date",
      source: "user",
      createdAt: new Date(Date.now() - 30_000).toISOString(),
    });
    // Turn 2 — must NOT be deduped against turn 1.
    await apiClient.seedComment({
      taskId: task.id,
      authorType: "agent",
      authorId: officeSeed.agentId,
      body: "Turn 2 — answering your follow-up.",
      source: "session",
      createdAt: new Date(Date.now() - 5_000).toISOString(),
    });

    await testPage.goto(`/office/tasks/${task.id}`);

    // Both turns must be visible — the per-task dedup bug would have
    // hidden turn 2.
    await expect(testPage.getByText("Turn 1 — initial analysis.")).toBeVisible({
      timeout: 10_000,
    });
    await expect(testPage.getByText("Turn 2 — answering your follow-up.")).toBeVisible();
    await expect(testPage.getByText("tell me the date")).toBeVisible();

    // Sanity: chat entries appear in chronological order.
    const html = await testPage.locator('[data-testid="task-chat-entries"]').innerHTML();
    const t1 = html.indexOf("Turn 1 — initial analysis.");
    const u = html.indexOf("tell me the date");
    const t2 = html.indexOf("Turn 2 — answering your follow-up.");
    expect(t1).toBeGreaterThanOrEqual(0);
    expect(u).toBeGreaterThan(t1);
    expect(t2).toBeGreaterThan(u);
  });
});
