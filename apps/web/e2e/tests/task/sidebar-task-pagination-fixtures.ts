import type { ApiClient } from "../../helpers/api-client";

export type SidebarPaginationSeedData = {
  workspaceId: string;
  workflowId: string;
  startStepId: string;
};

export async function seedSidebarPaginationTasks(
  apiClient: ApiClient,
  seedData: SidebarPaginationSeedData,
  token: string,
): Promise<string[]> {
  const taskIds: string[] = [];
  for (let offset = 0; offset < 101; offset += 10) {
    const seeded = await Promise.all(
      Array.from({ length: Math.min(10, 101 - offset) }, (_, index) => {
        const taskIndex = offset + index;
        let title: string;
        if (taskIndex < 99) title = `${token} match row ${String(taskIndex).padStart(3, "0")}`;
        else if (taskIndex === 99) title = `${token} match boundary`;
        else title = `${token} other boundary`;
        return apiClient.seedTask(seedData.workspaceId, title, {
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
        });
      }),
    );
    taskIds.push(...seeded.map((task) => task.task_id));
  }
  return taskIds;
}

export async function archiveSidebarPaginationTasks(
  apiClient: ApiClient,
  taskIds: string[],
): Promise<void> {
  for (let offset = 0; offset < taskIds.length; offset += 10) {
    await Promise.all(
      taskIds.slice(offset, offset + 10).map((taskId) => apiClient.archiveTask(taskId)),
    );
  }
}
