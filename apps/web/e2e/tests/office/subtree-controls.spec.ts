import { test, expect } from "../../fixtures/office-fixture";

type TreePreviewResponse = {
  task_count?: number;
  tasks?: Array<Record<string, unknown>>;
  active_run_count?: number;
  active_hold?: Record<string, unknown> | null;
};

type TreeHoldResponse = {
  hold?: {
    id?: string;
    mode?: string;
    root_task_id?: string;
    released_at?: string | null;
  };
};

type SubtreeCostResponse = {
  task_id?: string;
  task_count?: number;
  cost_subcents?: number;
  tokens_in?: number;
  tokens_out?: number;
};

/**
 * Subtree Controls E2E tests.
 *
 * These tests are API-driven and exercise the tree control endpoints:
 *   POST /tasks/:id/tree/preview
 *   POST /tasks/:id/tree/pause
 *   POST /tasks/:id/tree/resume
 *   POST /tasks/:id/tree/cancel
 *   POST /tasks/:id/tree/restore
 *   GET  /tasks/:id/tree/cost-summary
 */
test.describe("Subtree Controls", () => {
  test("preview tree returns correct task count for parent with children", async ({
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const parent = await apiClient.createTask(officeSeed.workspaceId, "Tree Preview Parent", {
      workflow_id: officeSeed.workflowId,
    });
    const child1 = await apiClient.createTask(officeSeed.workspaceId, "Tree Preview Child 1", {
      workflow_id: officeSeed.workflowId,
      parent_id: parent.id,
    });
    const child2 = await apiClient.createTask(officeSeed.workspaceId, "Tree Preview Child 2", {
      workflow_id: officeSeed.workflowId,
      parent_id: parent.id,
    });
    expect(child1.id).toBeTruthy();
    expect(child2.id).toBeTruthy();

    const preview = (await officeApi.previewTaskTree(parent.id)) as TreePreviewResponse;
    expect(preview.task_count).toBeGreaterThanOrEqual(3); // parent + 2 children
    expect(Array.isArray(preview.tasks)).toBe(true);
    expect((preview.tasks ?? []).length).toBeGreaterThanOrEqual(3);
  });

  test("preview tree for leaf task returns task count of 1", async ({
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const leaf = await apiClient.createTask(officeSeed.workspaceId, "Tree Preview Leaf", {
      workflow_id: officeSeed.workflowId,
    });

    const preview = (await officeApi.previewTaskTree(leaf.id)) as TreePreviewResponse;
    expect(preview.task_count).toBeGreaterThanOrEqual(1);
  });

  test("pause tree creates a pause hold", async ({ apiClient, officeApi, officeSeed }) => {
    const parent = await apiClient.createTask(officeSeed.workspaceId, "Pause Tree Parent", {
      workflow_id: officeSeed.workflowId,
    });
    await apiClient.createTask(officeSeed.workspaceId, "Pause Tree Child", {
      workflow_id: officeSeed.workflowId,
      parent_id: parent.id,
    });

    const result = (await officeApi.pauseTaskTree(parent.id)) as TreeHoldResponse;
    expect(result.hold).toBeDefined();
    expect(result.hold?.mode).toBe("pause");
    expect(result.hold?.root_task_id).toBe(parent.id);
    expect(result.hold?.released_at).toBeFalsy();
  });

  test("pause tree: preview shows active hold after pausing", async ({
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const parent = await apiClient.createTask(officeSeed.workspaceId, "Pause Preview Parent", {
      workflow_id: officeSeed.workflowId,
    });
    await apiClient.createTask(officeSeed.workspaceId, "Pause Preview Child", {
      workflow_id: officeSeed.workflowId,
      parent_id: parent.id,
    });

    const holdResult = (await officeApi.pauseTaskTree(parent.id)) as TreeHoldResponse;
    const holdId = holdResult.hold?.id;
    expect(holdId).toBeTruthy();

    const preview = (await officeApi.previewTaskTree(parent.id)) as TreePreviewResponse;
    expect(preview.active_hold).toBeDefined();
    expect((preview.active_hold as Record<string, unknown>)?.id).toBe(holdId);
  });

  test("resume tree releases the pause hold", async ({ apiClient, officeApi, officeSeed }) => {
    const parent = await apiClient.createTask(officeSeed.workspaceId, "Resume Tree Parent", {
      workflow_id: officeSeed.workflowId,
    });
    await apiClient.createTask(officeSeed.workspaceId, "Resume Tree Child", {
      workflow_id: officeSeed.workflowId,
      parent_id: parent.id,
    });

    // Pause first.
    await officeApi.pauseTaskTree(parent.id);

    // Now resume.
    const result = (await officeApi.resumeTaskTree(parent.id)) as TreeHoldResponse;
    expect(result.hold).toBeDefined();
    expect(result.hold?.mode).toBe("pause");

    // After resume, preview should show no active hold.
    const preview = (await officeApi.previewTaskTree(parent.id)) as TreePreviewResponse;
    expect(preview.active_hold).toBeFalsy();
  });

  test("cancel tree creates a cancel hold", async ({ apiClient, officeApi, officeSeed }) => {
    const parent = await apiClient.createTask(officeSeed.workspaceId, "Cancel Tree Parent", {
      workflow_id: officeSeed.workflowId,
    });
    await apiClient.createTask(officeSeed.workspaceId, "Cancel Tree Child", {
      workflow_id: officeSeed.workflowId,
      parent_id: parent.id,
    });

    const result = (await officeApi.cancelTaskTree(parent.id)) as TreeHoldResponse;
    expect(result.hold).toBeDefined();
    expect(result.hold?.mode).toBe("cancel");
    expect(result.hold?.root_task_id).toBe(parent.id);
    expect(result.hold?.released_at).toBeFalsy();
  });

  test("restore cancelled tree releases the cancel hold", async ({
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const parent = await apiClient.createTask(officeSeed.workspaceId, "Restore Tree Parent", {
      workflow_id: officeSeed.workflowId,
    });
    await apiClient.createTask(officeSeed.workspaceId, "Restore Tree Child", {
      workflow_id: officeSeed.workflowId,
      parent_id: parent.id,
    });

    // Cancel tree first.
    await officeApi.cancelTaskTree(parent.id);

    // Restore it.
    const result = (await officeApi.restoreTaskTree(parent.id)) as TreeHoldResponse;
    expect(result.hold).toBeDefined();
    expect(result.hold?.mode).toBe("cancel");

    // After restore, preview shows no active hold.
    const preview = (await officeApi.previewTaskTree(parent.id)) as TreePreviewResponse;
    expect(preview.active_hold).toBeFalsy();
  });

  test("cost summary returns task_count and cost_subcents fields", async ({
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const parent = await apiClient.createTask(officeSeed.workspaceId, "Cost Summary Parent", {
      workflow_id: officeSeed.workflowId,
    });
    await apiClient.createTask(officeSeed.workspaceId, "Cost Summary Child", {
      workflow_id: officeSeed.workflowId,
      parent_id: parent.id,
    });

    const summary = (await officeApi.getSubtreeCostSummary(parent.id)) as SubtreeCostResponse;
    expect(summary).toBeDefined();
    expect(typeof summary.task_count).toBe("number");
    expect(typeof summary.cost_subcents).toBe("number");
    expect(summary.task_count).toBeGreaterThanOrEqual(1);
  });

  test("cost summary includes token fields", async ({ apiClient, officeApi, officeSeed }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Cost Summary Tokens", {
      workflow_id: officeSeed.workflowId,
    });

    const summary = (await officeApi.getSubtreeCostSummary(task.id)) as SubtreeCostResponse;
    expect(typeof summary.tokens_in).toBe("number");
    expect(typeof summary.tokens_out).toBe("number");
  });

  test("resume fails when no active pause hold exists", async ({
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Resume No Hold Task", {
      workflow_id: officeSeed.workflowId,
    });

    // Resuming without a prior pause should fail.
    await expect(officeApi.resumeTaskTree(task.id)).rejects.toThrow();
  });

  test("restore fails when no active cancel hold exists", async ({
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Restore No Hold Task", {
      workflow_id: officeSeed.workflowId,
    });

    // Restoring without a prior cancel should fail.
    await expect(officeApi.restoreTaskTree(task.id)).rejects.toThrow();
  });
});
