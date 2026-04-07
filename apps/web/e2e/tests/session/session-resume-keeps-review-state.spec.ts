import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

// Regression: when the backend restarts and auto-resumes a task whose
// session was WAITING_FOR_INPUT, the task must NEVER transition into the
// in_progress / Running sidebar bucket. Before the fix, the resume flow
// cycled the session through STARTING/RUNNING states which briefly
// displaced the task into the Running bucket at the top of the sidebar.
// This test polls the sidebar continuously across the full resume window
// to catch any transient Running flicker.

test.describe("Session resume — task state stability", () => {
  test.describe.configure({ retries: 1 });

  test("resuming after backend restart does not flicker task into Running bucket", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    // 1. Create task and run it to WAITING_FOR_INPUT.
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Resume Stable",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const card = kanban.taskCardByTitle("Resume Stable");
    await expect(card).toBeVisible({ timeout: 15_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });
    await expect(session.idleInput()).toBeVisible({ timeout: 15_000 });
    await expect(session.taskInSection("Resume Stable", "Turn Finished")).toBeVisible({
      timeout: 15_000,
    });

    // 2. Restart backend and reload — triggers auto-resume via SSR + WS.
    await backend.restart();
    await testPage.reload();
    await session.waitForLoad();

    // 3. Continuously assert the task NEVER appears in the Running bucket
    //    across a fixed 10s window after reload. The composer stays visible
    //    throughout a silent resume (the fix intentionally keeps the
    //    workflow state stable), so we cannot use idleInput visibility as
    //    a stop signal — we must poll for a fixed duration to catch any
    //    transient flicker the backend may still emit while reconnecting.
    const runningLocator = session.taskInSection("Resume Stable", "Running");
    const probeWindowMs = 10_000;
    const deadline = Date.now() + probeWindowMs;
    let polls = 0;
    while (Date.now() < deadline) {
      const runningCount = await runningLocator.count();
      expect(runningCount, `task flickered into Running bucket during resume (poll ${polls})`).toBe(
        0,
      );
      polls++;
      await testPage.waitForTimeout(100);
    }
    // Ensure we actually polled meaningfully across the window.
    expect(polls).toBeGreaterThanOrEqual(50);

    // 4. Final settled state: still Turn Finished, not Running.
    await expect(session.idleInput()).toBeVisible({ timeout: 60_000 });
    await expect(session.taskInSection("Resume Stable", "Turn Finished")).toBeVisible({
      timeout: 15_000,
    });
    await expect(session.taskInSection("Resume Stable", "Running")).toHaveCount(0);
  });
});
