import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";

export type WipQueueScenario = {
  workflowId: string;
  reviewStepId: string;
  admittedTaskId: string;
  queuedTaskId: string;
  queuedTaskTitle: string;
};

export async function createWipQueueScenario(
  apiClient: ApiClient,
  seedData: SeedData,
  name: string,
): Promise<WipQueueScenario> {
  const workflow = await apiClient.createWorkflow(seedData.workspaceId, `${name} workflow`);
  await apiClient.createWorkflowStep(workflow.id, "Backlog", 0, { is_start_step: true });
  const reviewStep = await apiClient.createWorkflowStep(workflow.id, "Review", 1);
  await apiClient.updateWorkflowStep(reviewStep.id, { wip_limit: 1 });
  await apiClient.saveUserSettings({
    workspace_id: seedData.workspaceId,
    workflow_filter_id: workflow.id,
  });

  const admitted = await apiClient.createTask(seedData.workspaceId, `${name} admitted`, {
    workflow_id: workflow.id,
    workflow_step_id: reviewStep.id,
  });
  const queuedTitle = `${name} queued`;
  const queued = await apiClient.createTask(seedData.workspaceId, queuedTitle, {
    workflow_id: workflow.id,
    workflow_step_id: reviewStep.id,
  });

  return {
    workflowId: workflow.id,
    reviewStepId: reviewStep.id,
    admittedTaskId: admitted.id,
    queuedTaskId: queued.id,
    queuedTaskTitle: queuedTitle,
  };
}
