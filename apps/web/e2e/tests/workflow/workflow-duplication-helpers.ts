import { ApiClient } from "../../helpers/api-client";

export type WorkflowDuplicationSeed = {
  workflowId: string;
  reviewStepId: string;
  doneStepId: string;
  taskId: string;
};

export async function seedWorkflowDuplication(
  apiClient: ApiClient,
  workspaceId: string,
  name: string,
  agentProfileId?: string,
): Promise<WorkflowDuplicationSeed> {
  const workflow = await apiClient.createWorkflow(workspaceId, name);
  const review = await apiClient.createWorkflowStep(workflow.id, "Review", 0, {
    is_start_step: true,
  });
  const done = await apiClient.createWorkflowStep(workflow.id, "Done", 1);

  await apiClient.updateWorkflow(workflow.id, {
    description: "Copied workflow description",
    prompt: "Copied workflow prompt",
    ...(agentProfileId ? { agent_profile_id: agentProfileId } : {}),
  });
  await apiClient.updateWorkflowStep(review.id, {
    prompt: "Review step prompt",
    stage_type: "review",
    events: {
      on_turn_complete: [{ type: "move_to_step", config: { step_id: done.id } }],
    },
    wip_limit: 2,
  });
  await apiClient.updateWorkflowStep(done.id, {
    prompt: "Done step prompt",
    pull_from_step_id: review.id,
  });

  const task = await apiClient.seedTask(workspaceId, `${name} source task`, {
    workflow_id: workflow.id,
    workflow_step_id: review.id,
  });

  return {
    workflowId: workflow.id,
    reviewStepId: review.id,
    doneStepId: done.id,
    taskId: task.task_id,
  };
}
