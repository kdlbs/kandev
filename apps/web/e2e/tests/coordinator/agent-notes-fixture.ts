import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

export const NOTES_ENV = {
  KANDEV_FEATURES_COORDINATOR_PHASE31: "true",
  KANDEV_FEATURES_COORDINATOR_PHASE35: "true",
};

export const NOTE_TEXT = "Releases ship on Tuesdays";

/** Creates a coordinator and a board task the notes can cite. */
export async function setupNotes(apiClient: ApiClient, seedData: SeedData, name: string) {
  const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
    name,
    agent_profile_id: seedData.agentProfileId,
    executor_profile_id: seedData.worktreeExecutorProfileId,
    task_agent_profile_id: seedData.agentProfileId,
    task_executor_profile_id: seedData.worktreeExecutorProfileId,
  });
  const task = await apiClient.createTask(seedData.workspaceId, "Cited task", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  return { coordinator, taskId: task.id };
}
