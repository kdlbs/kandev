import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

test.describe("Workflow children completed trigger", () => {
  test("moves parent only after every active child reaches a terminal state", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Children Completed Workflow",
    );

    const waitStep = await apiClient.createWorkflowStep(workflow.id, "Waiting on Children", 0);
    const doneStep = await apiClient.createWorkflowStep(workflow.id, "Parent Done", 1);

    await apiClient.updateWorkflowStep(waitStep.id, {
      events: {
        on_children_completed: [{ type: "move_to_step", config: { step_id: doneStep.id } }],
      },
    });

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      enable_preview_on_click: false,
    });

    const parent = await apiClient.createTask(seedData.workspaceId, "Parent waits for children", {
      workflow_id: workflow.id,
      workflow_step_id: waitStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });
    await apiClient.seedTaskSession(parent.id, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
    });

    const firstChild = await apiClient.createTask(seedData.workspaceId, "First child task", {
      workflow_id: workflow.id,
      workflow_step_id: waitStep.id,
      parent_id: parent.id,
      repository_ids: [seedData.repositoryId],
    });
    const secondChild = await apiClient.createTask(seedData.workspaceId, "Second child task", {
      workflow_id: workflow.id,
      workflow_step_id: waitStep.id,
      parent_id: parent.id,
      repository_ids: [seedData.repositoryId],
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCardInColumn("Parent waits for children", waitStep.id)).toBeVisible({
      timeout: 15_000,
    });

    await apiClient.updateTaskState(firstChild.id, "COMPLETED");
    await expect(kanban.taskCardInColumn("Parent waits for children", waitStep.id)).toBeVisible({
      timeout: 10_000,
    });
    await expect(kanban.taskCardInColumn("Parent waits for children", doneStep.id)).toHaveCount(0);

    await apiClient.updateTaskState(secondChild.id, "COMPLETED");
    await expect(kanban.taskCardInColumn("Parent waits for children", doneStep.id)).toBeVisible({
      timeout: 15_000,
    });
  });
});
