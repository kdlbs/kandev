import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";

test("unarchive restores a quarantined workspace before branch recovery", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  const task = await apiClient.createTask(seedData.workspaceId, "Archived storage recovery", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await apiClient.archiveTask(task.id);
  const root = path.join(backend.tmpDir, ".kandev", "tasks", seedData.workspaceId, task.id);
  const artifact = path.join(root, "node_modules", "fixture", "index.js");
  fs.mkdirSync(path.dirname(artifact), { recursive: true });
  fs.writeFileSync(artifact, "restore-before-branch-probe");
  fs.writeFileSync(
    path.join(root, ".kandev-workspace.json"),
    JSON.stringify({
      task_id: task.id,
      workspace_id: seedData.workspaceId,
      task_dir_name: task.id,
      layout_version: 2,
      created_at: "2026-06-01T00:00:00Z",
    }),
  );
  const old = new Date(Date.now() - 8 * 24 * 60 * 60 * 1000);
  fs.utimesSync(root, old, old);

  await testPage.goto("/settings/system/storage");
  await testPage.getByTestId("storage-run-now").click();
  await expect(testPage.getByTestId("storage-run-now")).toHaveAttribute(
    "data-job-state",
    "succeeded",
  );
  expect(fs.existsSync(root)).toBe(false);

  await testPage.goto(`/t/${task.id}`);
  const responsePromise = testPage.waitForResponse((response) =>
    response.url().endsWith(`/api/v1/tasks/${task.id}/unarchive`),
  );
  await testPage.getByTestId("task-unarchive-button").click();
  const response = await responsePromise;
  const body = (await response.json()) as {
    unarchived_ids: string[];
    workspace_recovery: Array<{ status: string }>;
  };
  expect(body.unarchived_ids).toEqual([task.id]);
  expect(body.workspace_recovery).toEqual([expect.objectContaining({ status: "restored" })]);
  expect(body).toHaveProperty("recovery");
  await expect.poll(() => fs.existsSync(artifact)).toBe(true);
  await expect(testPage.getByText("Task unarchived")).toBeVisible();
});
