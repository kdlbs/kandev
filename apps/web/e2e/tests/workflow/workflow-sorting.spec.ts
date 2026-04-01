import { test, expect } from "../../fixtures/test-base";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

test.describe("Workflow sorting", () => {
  test("API reorder persists workflow order", async ({ apiClient, seedData }) => {
    const { workspaceId } = seedData;

    // Create additional workflows
    const wfB = await apiClient.createWorkflow(workspaceId, "Workflow B", "simple");
    const wfC = await apiClient.createWorkflow(workspaceId, "Workflow C", "simple");

    // Verify initial order: sorted by sort_order ASC (0, 1, 2)
    const before = await apiClient.listWorkflows(workspaceId);
    const namesBefore = before.workflows.map((w) => w.name);
    expect(namesBefore).toEqual(["E2E Workflow", "Workflow B", "Workflow C"]);

    // Reorder: C, E2E, B
    await apiClient.reorderWorkflows(workspaceId, [wfC.id, seedData.workflowId, wfB.id]);

    // Verify new order persists
    const after = await apiClient.listWorkflows(workspaceId);
    const namesAfter = after.workflows.map((w) => w.name);
    expect(namesAfter).toEqual(["Workflow C", "E2E Workflow", "Workflow B"]);
  });

  test("settings page displays workflows in sort order", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { workspaceId } = seedData;

    // Create a second workflow
    const wfExtra = await apiClient.createWorkflow(workspaceId, "Extra Workflow", "simple");

    // Reorder: Extra first, then E2E
    await apiClient.reorderWorkflows(workspaceId, [wfExtra.id, seedData.workflowId]);

    // Navigate to settings page
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(workspaceId);

    // Verify order matches API reorder
    const order = await page.getWorkflowOrder();
    expect(order).toEqual(["Extra Workflow", "E2E Workflow"]);
  });

  test("settings page shows drag handles for workflows", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { workspaceId } = seedData;

    // Create a second workflow so drag handles appear
    await apiClient.createWorkflow(workspaceId, "Second Workflow", "simple");

    const page = new WorkflowSettingsPage(testPage);
    await page.goto(workspaceId);

    // Verify drag handles are visible
    const handles = testPage.locator('[data-testid^="workflow-drag-handle-"]');
    await expect(handles.first()).toBeVisible();
    expect(await handles.count()).toBeGreaterThanOrEqual(2);
  });

  test("kanban board respects workflow sort order", async ({ testPage, apiClient, seedData }) => {
    const { workspaceId, workflowId } = seedData;

    // Create a second workflow with a task so both show on kanban
    const wfSecond = await apiClient.createWorkflow(workspaceId, "Second Board", "simple");
    const secondSteps = await apiClient.listWorkflowSteps(wfSecond.id);
    const secondStartStep = secondSteps.steps.find((s) => s.is_start_step) ?? secondSteps.steps[0];

    // Create tasks in both workflows so they appear
    await apiClient.createTask(workspaceId, "Task in E2E", {
      workflow_id: workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.createTask(workspaceId, "Task in Second", {
      workflow_id: wfSecond.id,
      workflow_step_id: secondStartStep.id,
    });

    // Reorder: Second Board first
    await apiClient.reorderWorkflows(workspaceId, [wfSecond.id, workflowId]);

    // Clear workflow filter so kanban shows all workflows with swimlane headers
    await apiClient.saveUserSettings({
      workspace_id: workspaceId,
      workflow_filter_id: "",
    });

    // Navigate to kanban
    await testPage.goto("/");
    await expect(testPage.getByTestId("swimlane-container")).toBeVisible({ timeout: 10000 });

    // Verify swimlane headers appear in the reordered sequence
    const headers = testPage.getByTestId("swimlane-header");
    await expect(headers.first()).toBeVisible();
    const firstHeader = await headers.first().textContent();
    expect(firstHeader).toContain("Second Board");
  });
});
