import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

export async function seedTaskTreeActivityScenario(
  apiClient: ApiClient,
  seedData: SeedData,
  prefix: string,
) {
  const taskOptions = {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  };
  const parent = await apiClient.createTask(seedData.workspaceId, `${prefix} Parent`, taskOptions);
  const child = await apiClient.createTask(seedData.workspaceId, `${prefix} Child`, {
    ...taskOptions,
    parent_id: parent.id,
  });
  const peer = await apiClient.createTask(seedData.workspaceId, `${prefix} Peer`, taskOptions);
  await apiClient.updateTaskTitle(child.id, `${prefix} Child active`);

  const navigationTask = await apiClient.seedTask(seedData.workspaceId, `${prefix} Navigation`, {
    ...taskOptions,
  });
  const viewId = `tree-activity-${prefix.toLowerCase().replaceAll(" ", "-")}`;
  await apiClient.saveUserSettings({
    sidebar_view_state: {
      workspace_id: seedData.workspaceId,
      views: [
        {
          id: viewId,
          name: `${prefix} activity view`,
          filters: [],
          sort: { key: "lastActivityAt", direction: "desc" },
          group: "none",
          collapsed_groups: [],
        },
      ],
      active_view_id: viewId,
    },
  });

  return {
    parent,
    child,
    peer,
    navigationTaskId: navigationTask.task_id,
  };
}
