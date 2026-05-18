import { test, expect } from "../../fixtures/office-fixture";

/**
 * E2E coverage for the office dashboard "Agents" panel: per-agent cards
 * rendered always (even with zero activity), driven reactively by the
 * existing WS pipeline (`session.state_changed`, `office.task.updated`,
 * `office.agent.updated`). Spec: docs/specs/office-dashboard-agent-cards.
 *
 * The lifecycle harness is rich enough to drive these scenarios without a
 * real executor: `apiClient.seedTaskSession` writes directly to the DB
 * AND publishes `task_session.state_changed` (see
 * `internal/office/testharness/routes.go:publishSessionStateChanged`),
 * which the gateway broadcasts as the `session.state_changed` WS action.
 * The seeder is idempotent on the (task, agent) pair — sending RUNNING
 * then IDLE updates the same row so the card flips back to "finished"
 * via the same WS event.
 */

const CEO_AGENT_ID_FALLBACK = "ceo";

test.describe("Dashboard agent cards", () => {
  test("renders one card per workspace agent regardless of activity", async ({
    testPage,
    officeSeed,
  }) => {
    await testPage.goto("/office");

    const card = testPage.getByTestId(`agent-card-${officeSeed.agentId}`);
    await expect(card).toBeVisible({ timeout: 10_000 });

    // Card subtitle. With no sessions seeded yet → "Never run".
    const subtitle = card.getByTestId("agent-card-subtitle");
    await expect(subtitle).toHaveText("Never run", { timeout: 10_000 });

    // No live dot, no task pill on a never-run card.
    await expect(card.getByTestId("agent-card-live-dot")).toHaveCount(0);
    await expect(card.getByTestId("agent-card-task-pill")).toHaveCount(0);
  });

  test("flips to live when a session goes RUNNING (background trigger)", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Background Run Task", {
      workflow_id: officeSeed.workflowId,
    });

    await testPage.goto("/office");
    const card = testPage.getByTestId(`agent-card-${officeSeed.agentId}`);
    await expect(card).toBeVisible({ timeout: 10_000 });
    // Baseline: no activity yet.
    await expect(card.getByTestId("agent-card-subtitle")).toHaveText("Never run", {
      timeout: 10_000,
    });

    // Schedule the seed 200ms after the page is loaded so the page has
    // already mounted its WS subscription. The harness publishes
    // session.state_changed; the panel must update without reload.
    setTimeout(() => {
      void apiClient.seedTaskSession(task.id, {
        state: "RUNNING",
        agentProfileId: officeSeed.agentId,
        startedAt: new Date(Date.now() - 5_000).toISOString(),
      });
    }, 200);

    // WS-driven subtitle flip.
    await expect(card.getByTestId("agent-card-subtitle")).toHaveText("Live now", {
      timeout: 10_000,
    });
    await expect(card.getByTestId("agent-card-live-dot")).toBeVisible({ timeout: 10_000 });

    // Task pill carries the identifier or title from the just-seeded session.
    const pill = card.getByTestId("agent-card-task-pill");
    await expect(pill).toBeVisible({ timeout: 10_000 });
    await expect(pill).toContainText("Background Run Task");
  });

  test("flips back to finished when the same (task,agent) session goes IDLE", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Cycle Task", {
      workflow_id: officeSeed.workflowId,
    });
    await apiClient.seedTaskSession(task.id, {
      state: "RUNNING",
      agentProfileId: officeSeed.agentId,
      startedAt: new Date(Date.now() - 30_000).toISOString(),
    });

    await testPage.goto("/office");
    const card = testPage.getByTestId(`agent-card-${officeSeed.agentId}`);
    await expect(card.getByTestId("agent-card-subtitle")).toHaveText("Live now", {
      timeout: 10_000,
    });

    // Idempotent re-seed flips the same row to IDLE — harness updates in
    // place rather than creating a duplicate (see seedTaskSessionHandler).
    await apiClient.seedTaskSession(task.id, {
      state: "IDLE",
      agentProfileId: officeSeed.agentId,
      startedAt: new Date(Date.now() - 30_000).toISOString(),
    });

    await expect(card.getByTestId("agent-card-subtitle")).toHaveText(/^Finished /, {
      timeout: 10_000,
    });
    await expect(card.getByTestId("agent-card-live-dot")).toHaveCount(0);
    // The task pill remains because last_session is still set.
    await expect(card.getByTestId("agent-card-task-pill")).toBeVisible({ timeout: 10_000 });
  });

  test("multi-agent: only the running agent's card pulses", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    // Create a second agent. The CEO is the only one we'll seed activity
    // for; the new agent should render but stay muted.
    const reviewer = (await officeApi.createAgent(officeSeed.workspaceId, {
      name: "Dashboard Reviewer",
      role: "worker",
    })) as Record<string, unknown>;
    const reviewerId = (reviewer.id as string) || CEO_AGENT_ID_FALLBACK;
    expect(reviewerId).toBeTruthy();

    const task = await apiClient.createTask(officeSeed.workspaceId, "Multi-agent Task", {
      workflow_id: officeSeed.workflowId,
    });
    await apiClient.seedTaskSession(task.id, {
      state: "RUNNING",
      agentProfileId: officeSeed.agentId,
      startedAt: new Date(Date.now() - 10_000).toISOString(),
    });

    await testPage.goto("/office");

    const ceoCard = testPage.getByTestId(`agent-card-${officeSeed.agentId}`);
    const reviewerCard = testPage.getByTestId(`agent-card-${reviewerId}`);
    await expect(ceoCard).toBeVisible({ timeout: 10_000 });
    await expect(reviewerCard).toBeVisible({ timeout: 10_000 });

    await expect(ceoCard.getByTestId("agent-card-live-dot")).toBeVisible({ timeout: 10_000 });
    await expect(reviewerCard.getByTestId("agent-card-live-dot")).toHaveCount(0);
    await expect(reviewerCard.getByTestId("agent-card-subtitle")).toHaveText("Never run", {
      timeout: 10_000,
    });
  });
});
