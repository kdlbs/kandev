import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

export const BOARD_CONTEXT_TASK_TITLE = "Kanban board context task";
export const FEATURE_TASK_TITLE = "Feature workflow proceed task";

export async function seedCrossWorkflowProceedScenario(apiClient: ApiClient, seedData: SeedData) {
  const featureWorkflow = await apiClient.createWorkflow(
    seedData.workspaceId,
    "Feature cross-workflow proceed",
  );
  const analysisStep = await apiClient.createWorkflowStep(featureWorkflow.id, "Analysis", 0, {
    is_start_step: true,
  });
  const implementStep = await apiClient.createWorkflowStep(featureWorkflow.id, "Implement", 1);
  await apiClient.updateWorkflowStep(analysisStep.id, {
    prompt: 'e2e:message("analysis complete")\n{{task_prompt}}',
    events: {
      on_enter: [{ type: "auto_start_agent" }],
      on_turn_complete: [{ type: "move_to_next" }],
    },
    auto_advance_requires_signal: true,
  });

  const boardTask = await apiClient.createTask(seedData.workspaceId, BOARD_CONTEXT_TASK_TITLE, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  const featureTask = await apiClient.createTask(seedData.workspaceId, FEATURE_TASK_TITLE, {
    workflow_id: featureWorkflow.id,
    workflow_step_id: analysisStep.id,
    agent_profile_id: seedData.agentProfileId,
    repository_ids: [seedData.repositoryId],
  });

  await apiClient.saveUserSettings({
    workspace_id: seedData.workspaceId,
    workflow_filter_id: seedData.workflowId,
  });

  return { boardTask, featureTask, featureWorkflow, analysisStep, implementStep };
}
