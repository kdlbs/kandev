import { test, expect } from "../../fixtures/office-fixture";

/**
 * E2E coverage for the per-agent paginated runs list and the run
 * detail page (Wave 1 Agent B):
 *
 *   - Seeds 30 runs for an agent → page 1 shows 25 rows + a "Load
 *     more" button → click yields the next page appended below.
 *   - Opens a specific run's detail page → header shows status +
 *     adapter + duration + cost; the recent-runs sidebar highlights
 *     the active row; the conversation embed renders for the linked
 *     session; the events log shows the seeded structured events.
 *
 * The test harness exposes `_test/runs` and `_test/run-events` so
 * the runs and events can be seeded deterministically without
 * launching an executor.
 */
test.describe("Office agent run detail", () => {
  test("paginated runs list loads more on demand", async ({ testPage, apiClient, officeSeed }) => {
    // The office fixture's agent is worker-scoped and may already have
    // runs from earlier tests, so we count rows relative to whatever
    // page-size baseline the UI shows after seeding rather than fixing
    // it at 25/30 — pagination semantics matter, not absolute counts.
    const base = Date.now();
    for (let i = 0; i < 30; i++) {
      await apiClient.seedRun({
        agentProfileId: officeSeed.agentId,
        status: "finished",
        reason: "task_assigned",
        requestedAt: new Date(base - i * 60_000).toISOString(),
      });
    }

    await testPage.goto(`/office/agents/${officeSeed.agentId}/runs`);

    const rows = testPage.locator('[data-testid^="agent-run-row-"]');
    const loadMore = testPage.getByTestId("agent-runs-load-more");

    // First page renders ≥1 row and surfaces the load-more affordance.
    await expect(rows.first()).toBeVisible({ timeout: 10_000 });
    await expect(loadMore).toBeVisible();
    const firstPageCount = await rows.count();

    // Clicking load-more grows the rendered set.
    await loadMore.click();
    await expect.poll(() => rows.count(), { timeout: 10_000 }).toBeGreaterThan(firstPageCount);
  });

  test("runs list shows 'No more runs' empty state when cursor is exhausted", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    // Use a fresh agent so prior tests' worker-scoped runs don't keep
    // the cursor open indefinitely. Three runs fit comfortably in one
    // page (limit 25), so the very first render has no next cursor.
    const agent = (await officeApi.createAgent(officeSeed.workspaceId, {
      name: `RunsEmptyState-${Date.now()}`,
      role: "worker",
    })) as { id: string };

    const base = Date.now();
    for (let i = 0; i < 3; i++) {
      await apiClient.seedRun({
        agentProfileId: agent.id,
        status: "finished",
        reason: "task_assigned",
        requestedAt: new Date(base - i * 60_000).toISOString(),
      });
    }

    await testPage.goto(`/office/agents/${agent.id}/runs`);

    const rows = testPage.locator('[data-testid^="agent-run-row-"]');
    await expect(rows.first()).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByTestId("agent-runs-end-of-list")).toBeVisible();
    await expect(testPage.getByTestId("agent-runs-load-more")).toHaveCount(0);
  });

  test("load-more button shows a busy state during fetch", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const base = Date.now();
    for (let i = 0; i < 30; i++) {
      await apiClient.seedRun({
        agentProfileId: officeSeed.agentId,
        status: "finished",
        reason: "task_assigned",
        requestedAt: new Date(base - i * 60_000).toISOString(),
      });
    }

    // Slow the runs API down so the busy state is observable. We
    // delay only the cursor-paginated calls (those carrying a
    // `cursor=` query) to avoid stalling the initial page load.
    await testPage.route("**/api/v1/office/agents/*/runs?*", async (route) => {
      const url = route.request().url();
      if (!url.includes("cursor=")) {
        await route.continue();
        return;
      }
      await new Promise((resolve) => setTimeout(resolve, 800));
      await route.continue();
    });

    await testPage.goto(`/office/agents/${officeSeed.agentId}/runs`);

    const loadMore = testPage.getByTestId("agent-runs-load-more");
    await expect(loadMore).toBeVisible({ timeout: 10_000 });
    await loadMore.click();
    // Spinner + disabled attribute appear while the request is in-flight.
    await expect(testPage.getByTestId("agent-runs-load-more-spinner")).toBeVisible();
    await expect(loadMore).toBeDisabled();
  });

  test("recent-runs sidebar marks active row with aria-current and animates RUNNING rows", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const base = Date.now();
    const ids: string[] = [];
    // Seed three runs: a running one, then two finished neighbours.
    const statuses: Array<"claimed" | "finished"> = ["claimed", "finished", "finished"];
    for (let i = 0; i < statuses.length; i++) {
      const res = await apiClient.seedRun({
        agentProfileId: officeSeed.agentId,
        status: statuses[i],
        reason: "task_assigned",
        requestedAt: new Date(base - i * 60_000).toISOString(),
      });
      ids.push(res.run_id);
    }
    const runningId = ids[0];
    const targetRunId = ids[1];

    await testPage.goto(`/office/agents/${officeSeed.agentId}/runs/${targetRunId}`);

    // The active row has aria-current="page"; siblings do not.
    const activeRow = testPage.getByTestId(`recent-runs-row-${targetRunId}`);
    await expect(activeRow).toBeVisible({ timeout: 10_000 });
    await expect(activeRow).toHaveAttribute("aria-current", "page");
    const otherRow = testPage.getByTestId(`recent-runs-row-${runningId}`);
    await expect(otherRow).not.toHaveAttribute("aria-current", "page");

    // The running row carries the animated icon testid.
    await expect(testPage.getByTestId(`recent-run-row-running-icon-${runningId}`)).toBeVisible();
  });

  test("run detail renders header, sidebar highlight, conversation, and events", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "run-detail-task", {
      workflow_id: officeSeed.workflowId,
    });
    // Seed a session so the conversation embed has something to
    // resolve; chat panel ignores hideInput=true cleanly when no
    // session messages exist (renders the empty state).
    const seeded = await apiClient.seedTaskSession(task.id, {
      state: "RUNNING",
      agentProfileId: officeSeed.agentId,
    });

    // Seed three runs so the sidebar has neighbours; the middle one
    // is the run we open.
    const base = Date.now();
    const ids: string[] = [];
    for (let i = 0; i < 3; i++) {
      const res = await apiClient.seedRun({
        agentProfileId: officeSeed.agentId,
        status: i === 1 ? "finished" : "finished",
        reason: "task_assigned",
        taskId: task.id,
        sessionId: seeded.session_id,
        capabilities:
          i === 1 ? JSON.stringify({ post_comment: true, create_agent: true }) : undefined,
        inputSnapshot: i === 1 ? JSON.stringify({ task_id: task.id }) : undefined,
        requestedAt: new Date(base - i * 60_000).toISOString(),
      });
      ids.push(res.run_id);
    }
    const targetRunId = ids[1];

    // Seed a few events on the target run so the events log has
    // rows to render.
    for (const eventType of ["init", "adapter.invoke", "step", "complete"]) {
      await apiClient.seedRunEvent({ runId: targetRunId, eventType, level: "info" });
    }

    await testPage.goto(`/office/agents/${officeSeed.agentId}/runs/${targetRunId}`);

    // Header is rendered with the run id_short (8 chars).
    await expect(testPage.getByTestId("run-header")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByTestId("run-status-badge")).toContainText("finished");

    // Sidebar highlights the active row.
    const activeRow = testPage.getByTestId(`recent-runs-row-${targetRunId}`);
    await expect(activeRow).toBeVisible();
    await expect(activeRow).toHaveAttribute("data-active", "true");

    // Other sidebar rows are NOT marked active.
    const otherRow = testPage.getByTestId(`recent-runs-row-${ids[0]}`);
    await expect(otherRow).toHaveAttribute("data-active", "false");

    // Conversation embed renders (the chat panel container, not
    // the empty-state stub, since a session_id is wired).
    await expect(testPage.getByTestId("run-conversation")).toBeVisible();

    // Runtime panel shows the process capabilities captured before launch.
    await expect(testPage.getByTestId("runtime-panel")).toBeVisible();
    await expect(testPage.getByTestId("runtime-capabilities")).toContainText("create_agent");

    // Events log shows the seeded events.
    await expect(testPage.getByTestId("events-log")).toBeVisible();
    const eventRows = testPage.locator('[data-testid^="events-log-row-"]');
    await expect(eventRows).toHaveCount(4);
  });
});
