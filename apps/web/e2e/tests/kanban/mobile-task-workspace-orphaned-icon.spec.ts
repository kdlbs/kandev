import { test, expect } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

const ORPHANED_WORKSPACE_METADATA = {
  workspace: {
    mode: "inherit_parent",
    orphaned: true,
    orphaned_parent_id: "11111111-1111-1111-1111-111111111111",
    orphaned_reason: "parent_archived",
    orphaned_at: "2026-09-05T10:00:00Z",
  },
};

test.describe("Mobile kanban — workspace-orphaned icon", () => {
  test("shows the marker on the focused column and omits it for plain tasks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const marked = await apiClient.createTask(seedData.workspaceId, "Mobile orphaned fixture", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.updateTaskMetadata(marked.id, ORPHANED_WORKSPACE_METADATA);
    await apiClient.updateTaskState(marked.id, "IN_PROGRESS");

    const plain = await apiClient.createTask(seedData.workspaceId, "Mobile plain fixture", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.updateTaskState(plain.id, "REVIEW");

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    const markedCard = mobile.taskCard(marked.id);
    await expect(markedCard).toBeVisible();
    await expect(markedCard.getByTestId("task-state-workspace-orphaned")).toBeVisible();

    const plainCard = mobile.taskCard(plain.id);
    await expect(plainCard).toBeVisible();
    await expect(plainCard.getByTestId("task-state-workspace-orphaned")).toHaveCount(0);
  });
});
