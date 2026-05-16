import { test, expect } from "../../fixtures/office-fixture";

/**
 * E2E coverage for the run detail page's Tasks Touched table
 * (Wave 1 Agent C). The component is owned by Agent C and consumed
 * by Agent B's run-detail view via a stable prop contract:
 * `<TasksTouched runId={...} taskIds={...} />`. The run detail
 * handler unions the run payload's task_id with
 * `ListTasksTouchedByRun(runID)` so any task the agent mutated
 * during the run shows up — even tasks the run wasn't initially
 * scoped to.
 *
 * This spec seeds:
 *   - One run claimed by the seeded agent.
 *   - Three tasks: one is the run's primary task, two more are
 *     "touched" via activity rows tagged with the run id.
 *
 * Then opens `/office/agents/:id/runs/:runId` and asserts:
 *   - All three tasks render in the table.
 *   - Each row has a link to `/office/tasks/:id`.
 *   - Clicking a row navigates to that task's page.
 */
test.describe("Office run detail Tasks Touched", () => {
  test("renders all run-mutated tasks and rows link to task page", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    // Three tasks: one is the run's payload task, the other two get
    // attributed via activity rows.
    const taskA = await apiClient.createTask(officeSeed.workspaceId, "Tasks-touched A", {
      workflow_id: officeSeed.workflowId,
    });
    const taskB = await apiClient.createTask(officeSeed.workspaceId, "Tasks-touched B", {
      workflow_id: officeSeed.workflowId,
    });
    const taskC = await apiClient.createTask(officeSeed.workspaceId, "Tasks-touched C", {
      workflow_id: officeSeed.workflowId,
    });

    const seeded = await apiClient.seedTaskSession(taskA.id as string, {
      state: "RUNNING",
      agentProfileId: officeSeed.agentId,
    });

    // Seed the run pointing at task A as its primary task. The
    // run-detail handler's union-with-payload-task semantics ensure
    // task A surfaces even without a matching activity row.
    const run = await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      status: "finished",
      reason: "task_assigned",
      taskId: taskA.id as string,
      sessionId: seeded.session_id,
    });

    // Tag two activity rows with this run for tasks B and C — these
    // come back from ListTasksTouchedByRun and the handler unions
    // them with the payload task.
    await apiClient.seedActivity({
      workspaceId: officeSeed.workspaceId,
      actorType: "agent",
      actorId: officeSeed.agentId,
      action: "task.touched",
      targetType: "task",
      targetId: taskB.id as string,
      runId: run.run_id,
      sessionId: seeded.session_id,
    });
    await apiClient.seedActivity({
      workspaceId: officeSeed.workspaceId,
      actorType: "agent",
      actorId: officeSeed.agentId,
      action: "task.touched",
      targetType: "task",
      targetId: taskC.id as string,
      runId: run.run_id,
      sessionId: seeded.session_id,
    });

    await testPage.goto(`/office/agents/${officeSeed.agentId}/runs/${run.run_id}`);

    // Tasks Touched card renders.
    const tasksTouched = testPage.getByTestId("tasks-touched");
    await expect(tasksTouched).toBeVisible({ timeout: 10_000 });

    // All three rows render (component fetches the per-task rows
    // lazily; wait until the loaded state replaces the placeholders).
    const rows = tasksTouched.locator('[data-testid="tasks-touched-row"]');
    await expect(rows).toHaveCount(3, { timeout: 10_000 });

    // Each row's id must be one of the three we seeded.
    const seededIds = new Set([taskA.id, taskB.id, taskC.id] as string[]);
    const renderedIds = await rows.evaluateAll((els) =>
      els.map((el) => el.getAttribute("data-task-id")),
    );
    for (const id of renderedIds) {
      expect(seededIds.has(id ?? "")).toBe(true);
    }

    // Clicking the title for task B navigates to its detail page.
    const taskBRow = tasksTouched.locator(`[data-task-id="${taskB.id}"]`);
    await taskBRow.locator('[data-testid="tasks-touched-row-title"]').click();
    await testPage.waitForURL(new RegExp(`/office/tasks/${taskB.id}$`));
  });
});
