import { expect, test } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

test.describe("Mobile kanban - interrupted-task icon", () => {
  test("shows the warning triangle for an interrupted task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const interrupted = await apiClient.createTask(
      seedData.workspaceId,
      "Mobile interrupted fixture",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        metadata: { interrupted_at: "2026-08-02T10:00:00Z" },
      },
    );
    await apiClient.updateTaskState(interrupted.id, "REVIEW");

    const plain = await apiClient.createTask(seedData.workspaceId, "Mobile plain fixture", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.updateTaskState(plain.id, "REVIEW");

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    const interruptedCard = mobile.taskCard(interrupted.id);
    await expect(interruptedCard).toBeVisible();
    const interruptedIcon = interruptedCard.getByTestId("task-state-interrupted");
    await expect(interruptedIcon).toBeVisible();
    await expect(interruptedIcon).toHaveClass(/tabler-icon-alert-triangle/);
    await expect(interruptedIcon).toHaveClass(/text-yellow-500/);

    const plainCard = mobile.taskCard(plain.id);
    await expect(plainCard).toBeVisible();
    await expect(plainCard.getByTestId("task-state-interrupted")).toHaveCount(0);
  });
});
