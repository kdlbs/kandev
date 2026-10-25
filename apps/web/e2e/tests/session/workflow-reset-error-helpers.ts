import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

export const WORKFLOW_RESET_ERROR_MESSAGE =
  "Context reset failed. The workflow step prompt did not start.";
export const WORKFLOW_RESET_ERROR_DETAILS =
  'workflow step "Review Step": provider context reset: provider reset timed out';

export async function seedWorkflowResetFailure(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
) {
  const task = await apiClient.createTask(seedData.workspaceId, title, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const occurredAt = new Date(Date.now() + 60 * 60 * 1000).toISOString();
  const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
    state: "WAITING_FOR_INPUT",
    agentProfileId: seedData.agentProfileId,
    repositoryId: seedData.repositoryId,
    metadata: {
      last_agent_error: {
        message: WORKFLOW_RESET_ERROR_MESSAGE,
        details: WORKFLOW_RESET_ERROR_DETAILS,
        occurred_at: occurredAt,
        agent_execution_id: "workflow-reset-e2e",
        code: "workflow_context_reset_failed",
      },
    },
  });
  return { task, sessionId };
}
