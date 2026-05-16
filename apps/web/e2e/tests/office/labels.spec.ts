import { test, expect } from "../../fixtures/office-fixture";

test.describe("Labels", () => {
  test("add label creates with auto-color", async ({ officeApi, officeSeed, apiClient }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Label Auto-Color Task", {
      workflow_id: officeSeed.workflowId,
    });
    const taskId = task.id;

    const result = await officeApi.addLabel(officeSeed.workspaceId, taskId, "bug");
    const label = (result as { label: { name: string; color: string } }).label;

    expect(label.name).toBe("bug");
    expect(label.color).toMatch(/^#[0-9a-fA-F]{6}$/);
  });

  test("add same label twice is idempotent", async ({ officeApi, officeSeed, apiClient }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Label Idempotent Task", {
      workflow_id: officeSeed.workflowId,
    });
    const taskId = task.id;

    await officeApi.addLabel(officeSeed.workspaceId, taskId, "duplicate");
    // Adding again must not throw
    await officeApi.addLabel(officeSeed.workspaceId, taskId, "duplicate");

    const labelsResp = await officeApi.listTaskLabels(officeSeed.workspaceId, taskId);
    const labels = (labelsResp as { labels: Array<{ name: string }> }).labels;
    const matching = labels.filter((l) => l.name === "duplicate");
    // Idempotent: only one entry expected
    expect(matching.length).toBe(1);
  });

  test("remove label from task", async ({ officeApi, officeSeed, apiClient }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Label Remove Task", {
      workflow_id: officeSeed.workflowId,
    });
    const taskId = task.id;

    await officeApi.addLabel(officeSeed.workspaceId, taskId, "to-remove");

    // Verify it was added
    const beforeResp = await officeApi.listTaskLabels(officeSeed.workspaceId, taskId);
    const before = (beforeResp as { labels: Array<{ name: string }> }).labels;
    expect(before.some((l) => l.name === "to-remove")).toBe(true);

    await officeApi.removeLabel(officeSeed.workspaceId, taskId, "to-remove");

    const afterResp = await officeApi.listTaskLabels(officeSeed.workspaceId, taskId);
    const after = (afterResp as { labels: Array<{ name: string }> }).labels;
    expect(after.some((l) => l.name === "to-remove")).toBe(false);
  });

  test("list labels for task returns correct labels", async ({
    officeApi,
    officeSeed,
    apiClient,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Label List Task", {
      workflow_id: officeSeed.workflowId,
    });
    const taskId = task.id;

    await officeApi.addLabel(officeSeed.workspaceId, taskId, "alpha");
    await officeApi.addLabel(officeSeed.workspaceId, taskId, "beta");

    const labelsResp = await officeApi.listTaskLabels(officeSeed.workspaceId, taskId);
    const labels = (labelsResp as { labels: Array<{ name: string }> }).labels;

    const names = labels.map((l) => l.name);
    expect(names).toContain("alpha");
    expect(names).toContain("beta");
  });

  test("list workspace labels shows all created labels", async ({
    officeApi,
    officeSeed,
    apiClient,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Workspace Labels Task", {
      workflow_id: officeSeed.workflowId,
    });
    const taskId = task.id;

    await officeApi.addLabel(officeSeed.workspaceId, taskId, "workspace-label-1");
    await officeApi.addLabel(officeSeed.workspaceId, taskId, "workspace-label-2");

    const wsLabelsResp = await officeApi.listWorkspaceLabels(officeSeed.workspaceId);
    const wsLabels = (wsLabelsResp as { labels: Array<{ name: string }> }).labels;
    const names = wsLabels.map((l) => l.name);

    expect(names).toContain("workspace-label-1");
    expect(names).toContain("workspace-label-2");
  });

  test("update label color changes the color", async ({ officeApi, officeSeed, apiClient }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Label Color Update Task", {
      workflow_id: officeSeed.workflowId,
    });
    const taskId = task.id;

    const addResp = await officeApi.addLabel(officeSeed.workspaceId, taskId, "color-test");
    const addedLabel = (addResp as { label: { id: string; color: string } }).label;

    const newColor = "#ff0000";
    await officeApi.updateLabel(officeSeed.workspaceId, addedLabel.id, { color: newColor });

    const wsLabelsResp = await officeApi.listWorkspaceLabels(officeSeed.workspaceId);
    const wsLabels = (wsLabelsResp as { labels: Array<{ id: string; color: string }> }).labels;
    const updated = wsLabels.find((l) => l.id === addedLabel.id);

    expect(updated).toBeDefined();
    expect(updated!.color).toBe(newColor);
  });

  test("delete label removes it from catalog and tasks", async ({
    officeApi,
    officeSeed,
    apiClient,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Label Delete Task", {
      workflow_id: officeSeed.workflowId,
    });
    const taskId = task.id;

    const addResp = await officeApi.addLabel(officeSeed.workspaceId, taskId, "to-delete");
    const addedLabel = (addResp as { label: { id: string; name: string } }).label;

    // Verify on task before deletion
    const taskLabelsResp = await officeApi.listTaskLabels(officeSeed.workspaceId, taskId);
    const taskLabels = (taskLabelsResp as { labels: Array<{ name: string }> }).labels;
    expect(taskLabels.some((l) => l.name === "to-delete")).toBe(true);

    await officeApi.deleteLabel(officeSeed.workspaceId, addedLabel.id);

    // Should be gone from workspace catalog
    const wsLabelsResp = await officeApi.listWorkspaceLabels(officeSeed.workspaceId);
    const wsLabels = (wsLabelsResp as { labels: Array<{ id: string }> }).labels;
    expect(wsLabels.some((l) => l.id === addedLabel.id)).toBe(false);

    // Should be gone from the task too (cascade delete)
    const afterTaskLabels = await officeApi.listTaskLabels(officeSeed.workspaceId, taskId);
    const afterLabels = (afterTaskLabels as { labels: Array<{ name: string }> }).labels;
    expect(afterLabels.some((l) => l.name === "to-delete")).toBe(false);
  });
});
