import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { watchWs } from "../../helpers/causal-waits";

// Auto-start-failed indicator: when a workflow step's `auto_start_agent`
// on_enter action cannot launch a run (for example an Office task moved into
// a step whose runtime context isn't ready), the orchestrator stamps the
// `auto_start_failed` metadata key and the kanban card shows a red alert icon
// instead of silently sitting there looking normal. Seeding `auto_start_failed`
// through the public task-metadata surface is the deterministic stand-in for
// a real failed launch; the icon itself must render from that marker alone.
//
// The fixture is left in a session-less IN_PROGRESS state on purpose: a real
// failed launch comes from startTask setting the task to SCHEDULING/IN_PROGRESS
// before session creation, then failing before a session ever attaches. That
// shape makes shouldShowTaskRunningSpinner report true, which previously
// short-circuited renderTaskStatusIcon straight to the launch spinner before
// the auto-start-failed marker ever got a chance to render.

test.describe("Kanban card — auto-start-failed icon", () => {
  test("shows the red auto-start-failed icon only for marked, non-terminal tasks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const failed = await apiClient.createTask(seedData.workspaceId, "Auto Start Failed Fixture", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      metadata: { auto_start_failed: "true" },
    });
    // Session-less IN_PROGRESS: the exact shape a launch failure before
    // session creation leaves behind, and the one the running-spinner
    // short-circuit previously masked the marker in.
    await apiClient.updateTaskState(failed.id, "IN_PROGRESS");

    const plain = await apiClient.createTask(seedData.workspaceId, "Plain Fixture", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await apiClient.updateTaskState(plain.id, "REVIEW");

    // A terminal marked task must keep its done affordance, never the red icon.
    const terminal = await apiClient.createTask(seedData.workspaceId, "Terminal Fixture", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      metadata: { auto_start_failed: "true" },
    });
    await apiClient.updateTaskState(terminal.id, "COMPLETED");

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const failedCard = kanban.taskCardByTitle("Auto Start Failed Fixture");
    await expect(failedCard).toBeVisible({ timeout: 20_000 });
    await expect(failedCard.getByTestId("task-state-auto-start-failed")).toBeVisible({
      timeout: 20_000,
    });

    const plainCard = kanban.taskCardByTitle("Plain Fixture");
    await expect(plainCard).toBeVisible({ timeout: 20_000 });
    await expect(plainCard.getByTestId("task-state-auto-start-failed")).toHaveCount(0);

    const terminalCard = kanban.taskCardByTitle("Terminal Fixture");
    await expect(terminalCard).toBeVisible({ timeout: 20_000 });
    await expect(terminalCard.getByTestId("task-state-auto-start-failed")).toHaveCount(0);

    // Reload: the marker must survive SSR hydration from the boot payload.
    await testPage.reload();
    await kanban.board.waitFor({ state: "visible" });
    await expect(kanban.taskCardByTitle("Auto Start Failed Fixture")).toBeVisible({
      timeout: 20_000,
    });
    await expect(
      kanban
        .taskCardByTitle("Auto Start Failed Fixture")
        .getByTestId("task-state-auto-start-failed"),
    ).toBeVisible({ timeout: 20_000 });
    await expect(
      kanban.taskCardByTitle("Plain Fixture").getByTestId("task-state-auto-start-failed"),
    ).toHaveCount(0);
  });

  // Every other case in this file seeds `auto_start_failed` before the page
  // ever loads, so it only ever exercises the HTTP snapshot / boot-payload
  // producer. That left the WS producer (task.updated -> kanban.update store
  // merge) unverified: it hand-builds its payload separately and had silently
  // dropped the field, so a failure that happened while the board was already
  // open never showed the badge until a reload. This test sets and clears the
  // marker via a live PATCH while the board stays open, so only the WS path
  // can make it pass.
  test("shows and hides the auto-start-failed icon over a live WS update, without a reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const task = await apiClient.createTask(seedData.workspaceId, "Live WS Auto Start Fixture", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await apiClient.updateTaskState(task.id, "IN_PROGRESS");

    const kanban = new KanbanPage(testPage);
    const ws = watchWs(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("Live WS Auto Start Fixture");
    await expect(card).toBeVisible({ timeout: 20_000 });
    await expect(card.getByTestId("task-state-auto-start-failed")).toHaveCount(0);

    const markerSet = ws.waitForEvent("task.updated", {
      where: (payload) => payload.task_id === task.id && payload.auto_start_failed === true,
    });
    await apiClient.updateTaskMetadata(task.id, { auto_start_failed: "true" });
    await markerSet;

    await expect(card.getByTestId("task-state-auto-start-failed")).toBeVisible({
      timeout: 10_000,
    });

    const markerCleared = ws.waitForEvent("task.updated", {
      where: (payload) => payload.task_id === task.id && payload.auto_start_failed === false,
    });
    await apiClient.updateTaskMetadata(task.id, {});
    await markerCleared;

    await expect(card.getByTestId("task-state-auto-start-failed")).toHaveCount(0);
  });
});
