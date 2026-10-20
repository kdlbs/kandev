import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import {
  bootSnapshotTaskCount,
  captureJourneyMetadata,
  readBootCapture,
} from "./journey-boot-loading-helpers";

test("boots only the selected board and loads every visible board on demand", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const metadata = await captureJourneyMetadata(testPage);
  const otherWorkflow = await apiClient.createWorkflow(seedData.workspaceId, "Boot demand board");
  const otherStep = await apiClient.createWorkflowStep(otherWorkflow.id, "Ready", 0, {
    is_start_step: true,
  });
  const selectedTask = await apiClient.createTask(seedData.workspaceId, "Selected boot task", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const otherTask = await apiClient.createTask(seedData.workspaceId, "Other boot task", {
    workflow_id: otherWorkflow.id,
    workflow_step_id: otherStep.id,
  });

  await apiClient.saveUserSettings({
    workspace_id: seedData.workspaceId,
    workflow_filter_id: seedData.workflowId,
  });
  await testPage.goto(`/?workspaceId=${seedData.workspaceId}&workflowId=${seedData.workflowId}`);
  const board = new KanbanPage(testPage);
  await expect(board.taskCard(selectedTask.id)).toBeVisible();

  const selectedBoot = await readBootCapture(testPage);
  expect(selectedBoot.version).toBe(2);
  expect(Object.keys(selectedBoot.initialState.kanbanMulti?.snapshots ?? {})).toEqual([
    seedData.workflowId,
  ]);
  expect(selectedBoot.entities?.tasks).toHaveProperty(selectedTask.id);
  expect(selectedBoot.entities?.tasks).not.toHaveProperty(otherTask.id);
  expect(selectedBoot.bytes).toBeLessThanOrEqual(1.25 * 1024 * 1024);
  expect(
    bootSnapshotTaskCount(selectedBoot.initialState.kanbanMulti?.snapshots?.[seedData.workflowId]),
  ).toBeGreaterThan(0);

  await apiClient.saveUserSettings({ workflow_filter_id: "" });
  await testPage.goto(`/?workspaceId=${seedData.workspaceId}&workflowId=`);
  const allBoot = await readBootCapture(testPage);
  expect(Object.keys(allBoot.initialState.kanbanMulti?.snapshots ?? {})).toHaveLength(1);
  await expect(board.taskCard(selectedTask.id)).toBeVisible();
  await expect(board.taskCard(otherTask.id)).toBeVisible({ timeout: 15_000 });
  expect(Object.values(metadata).every((resource) => resource.peak <= 1)).toBe(true);
  await test.info().attach("journey-metadata-resources.json", {
    body: JSON.stringify(metadata, null, 2),
    contentType: "application/json",
  });
});

test("task detail boots its selected task and one sidebar page within the route budget", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.createTask(seedData.workspaceId, "Bound task detail boot", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const sibling = await apiClient.createTask(seedData.workspaceId, "Bound sidebar page sibling", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });

  const workflowSnapshotRequests: string[] = [];
  testPage.on("request", (request) => {
    if (request.url().includes(`/api/v1/workflows/${seedData.workflowId}/snapshot`)) {
      workflowSnapshotRequests.push(request.url());
    }
  });
  await testPage.goto(`/t/${task.id}`);
  await expect(testPage.getByTestId("dockview-task-layout")).toBeVisible();
  const boot = await readBootCapture(testPage);
  const detail = boot.routeData?.taskDetail;
  expect(boot.version).toBe(2);
  expect(detail?.taskId).toBe(task.id);
  expect(bootSnapshotTaskCount(detail?.initialState?.kanban)).toBe(1);
  const sidebarTaskIDs = (detail?.sidebarTaskPage?.entries ?? [])
    .map((entry) => entry.task_id)
    .filter((id): id is string => Boolean(id));
  expect(sidebarTaskIDs).toContain(task.id);
  expect(sidebarTaskIDs).toContain(sibling.id);
  expect(boot.bytes).toBeLessThanOrEqual(512 * 1024);
  expect(workflowSnapshotRequests).toEqual([]);
});
