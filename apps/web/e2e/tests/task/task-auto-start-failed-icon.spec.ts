import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

// Auto-start-failed indicator: when a workflow step's `auto_start_agent`
// on_enter action cannot launch a run (for example an Office task moved into
// a step whose runtime context isn't ready), the orchestrator stamps the
// `auto_start_failed` metadata key and the kanban card shows a red alert icon
// instead of silently sitting there looking normal. Seeding `auto_start_failed`
// through the public task-metadata surface is the deterministic stand-in for
// a real failed launch; the icon itself must render from that marker alone.

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
    await apiClient.updateTaskState(failed.id, "REVIEW");

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
});
