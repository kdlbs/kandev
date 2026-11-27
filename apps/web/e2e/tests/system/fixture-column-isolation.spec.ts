import { test as base, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

const test = base.extend<{ hiddenColumnPrecondition: void }>({
  hiddenColumnPrecondition: [
    async ({ apiClient, seedData }, use) => {
      await apiClient.saveUserSettings({
        kanban_hidden_step_ids: { [seedData.workflowId]: [seedData.startStepId] },
      });
      const { settings } = await apiClient.getUserSettings();
      expect(settings.kanban_hidden_step_ids?.[seedData.workflowId]).toContain(
        seedData.startStepId,
      );
      await use();
    },
    { auto: true },
  ],
});

test("restores visible Kanban columns after another test changed their visibility", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.createTask(seedData.workspaceId, "Visible after fixture reset", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  const kanban = new KanbanPage(testPage);
  await kanban.goto(seedData.workflowId);
  await expect(kanban.columnByStepId(seedData.startStepId)).toBeVisible();
  await expect(kanban.taskCard(task.id)).toBeVisible();
});
