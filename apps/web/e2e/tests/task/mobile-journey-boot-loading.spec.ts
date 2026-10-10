import { test, expect } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { captureJourneyMetadata, readBootCapture } from "./journey-boot-loading-helpers";

test("boots one phone board and loads the selected workflow through its picker", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await testPage.setViewportSize({ width: 390, height: 844 });
  const metadata = await captureJourneyMetadata(testPage);
  const otherWorkflow = await apiClient.createWorkflow(
    seedData.workspaceId,
    "Phone boot demand board",
  );
  const otherStep = await apiClient.createWorkflowStep(otherWorkflow.id, "Ready", 0, {
    is_start_step: true,
  });
  const selectedTask = await apiClient.createTask(
    seedData.workspaceId,
    "Phone selected boot task",
    {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    },
  );
  const otherTask = await apiClient.createTask(seedData.workspaceId, "Phone other boot task", {
    workflow_id: otherWorkflow.id,
    workflow_step_id: otherStep.id,
  });
  await apiClient.saveUserSettings({
    workspace_id: seedData.workspaceId,
    workflow_filter_id: seedData.workflowId,
  });

  const mobile = new MobileKanbanPage(testPage);
  await testPage.goto(`/?workspaceId=${seedData.workspaceId}&workflowId=${seedData.workflowId}`);
  await expect(mobile.mobileKanbanLayout()).toBeVisible();
  await expect(mobile.taskCard(selectedTask.id)).toBeVisible();

  const boot = await readBootCapture(testPage);
  expect(boot.version).toBe(2);
  expect(Object.keys(boot.initialState.kanbanMulti?.snapshots ?? {})).toEqual([
    seedData.workflowId,
  ]);
  expect(boot.entities?.tasks).toHaveProperty(selectedTask.id);
  expect(boot.entities?.tasks).not.toHaveProperty(otherTask.id);
  expect(boot.bytes).toBeLessThanOrEqual(1.25 * 1024 * 1024);

  await mobile.boardNavigator.tap();
  const otherWorkflowOption = mobile.workflowItem(otherWorkflow.id);
  await expect(otherWorkflowOption).toBeVisible();
  await otherWorkflowOption.tap();
  await expect(mobile.taskCard(otherTask.id)).toBeVisible({ timeout: 15_000 });
  expect(Object.values(metadata).every((resource) => resource.peak <= 1)).toBe(true);
  await test.info().attach("journey-metadata-resources.json", {
    body: JSON.stringify(metadata, null, 2),
    contentType: "application/json",
  });
});
