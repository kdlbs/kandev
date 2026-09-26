import { expect, test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { useRegularMode } from "../../helpers/regular-mode";

useRegularMode();

test("keeps the previous spacing between virtualized cards", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await apiClient.createTask(seedData.workspaceId, "Spacing task one", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await apiClient.createTask(seedData.workspaceId, "Spacing task two", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });

  const kanban = new KanbanPage(testPage);
  await kanban.goto();

  const cards = kanban
    .columnByStepId(seedData.startStepId)
    .locator('[data-slot="card"][data-testid^="task-card-"]');
  await expect(cards).toHaveCount(2);

  const gap = await cards.evaluateAll((elements) => {
    const first = elements[0]?.getBoundingClientRect();
    const second = elements[1]?.getBoundingClientRect();
    if (!first || !second) throw new Error("expected two card boxes");
    return second.top - first.bottom;
  });

  expect(gap).toBeLessThanOrEqual(9);
});
