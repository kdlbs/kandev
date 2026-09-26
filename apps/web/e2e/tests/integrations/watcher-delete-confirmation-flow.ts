import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

export async function seedLinearWatcher(apiClient: ApiClient, seedData: SeedData) {
  await apiClient.setLinearConfig({
    secret: "lin_api_watcher_delete",
    workspaceId: seedData.workspaceId,
  });
  await apiClient.mockLinearSetTeams([{ id: "team-1", key: "ENG", name: "Engineering" }]);
  await apiClient.waitForIntegrationAuthHealthy("linear", {
    workspaceId: seedData.workspaceId,
  });
  return apiClient.createLinearIssueWatch({
    workspaceId: seedData.workspaceId,
    workflowId: seedData.workflowId,
    workflowStepId: seedData.startStepId,
    agentProfileId: seedData.agentProfileId,
    executorProfileId: seedData.worktreeExecutorProfileId,
    filter: { teamKey: "ENG" },
  });
}
