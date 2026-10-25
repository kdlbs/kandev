import type { Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SidebarFilterPopoverPage } from "../../pages/sidebar-filter-popover";

export type SeededConversation = {
  id: string;
  title: string;
  sessionId: string;
  response: string;
};

export async function seedActiveConversation(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  response: string,
): Promise<SeededConversation> {
  const task = await apiClient.createTask(seedData.workspaceId, title, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
    state: "COMPLETED",
    agentProfileId: seedData.agentProfileId,
    completedAt: new Date().toISOString(),
  });
  await apiClient.seedSessionMessage(sessionId, { type: "message", content: response });
  await apiClient.updateTaskState(task.id, "COMPLETED");
  return { id: task.id, title, sessionId, response };
}

export async function seedArchivedConversations(
  apiClient: ApiClient,
  seedData: SeedData,
  prefix: string,
): Promise<SeededConversation[]> {
  const results: SeededConversation[] = [];
  for (const suffix of ["A", "B", "C"]) {
    const title = `${prefix} ${suffix}`;
    const response = `${prefix} conversation ${suffix}`;
    const task = await apiClient.createTask(seedData.workspaceId, title, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "COMPLETED",
      completedAt: new Date().toISOString(),
    });
    await apiClient.seedSessionMessage(sessionId, { type: "message", content: response });
    await apiClient.updateTaskState(task.id, "COMPLETED");
    await apiClient.archiveTask(task.id);
    results.push({ id: task.id, title, sessionId, response });
  }
  return results;
}

export async function selectArchivedSidebarView(page: Page): Promise<void> {
  const filters = new SidebarFilterPopoverPage(page);
  await filters.addFilterRow();
  await filters.setClauseDimension(0, "Archived");
  await filters.setClauseBooleanValue(0, true);
  await filters.close();
}
