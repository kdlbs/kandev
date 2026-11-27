import { test, expect, moveTaskToTerminalStep } from "../../fixtures/office-fixture";

test("task status updates validate, persist and record comments", async ({
  officeApi,
  officeSeed,
  apiClient,
}) => {
  const task = await apiClient.createTask(officeSeed.workspaceId, "Status Lifecycle Task", {
    workflow_id: officeSeed.workflowId,
  });
  expect(await officeApi.updateTaskStatus(task.id, "in_progress", "Resuming work")).toMatchObject({
    ok: true,
  });
  expect(((await officeApi.getTask(task.id)) as { task: { status: string } }).task.status).toBe(
    "in_progress",
  );
  await expect(
    officeApi.updateTaskStatus(task.id, "invalid-status", "some comment"),
  ).rejects.toThrow();
  await moveTaskToTerminalStep(apiClient, officeSeed.workflowId, task.id);
  const comment = "Status updated in E2E test";
  expect(await officeApi.updateTaskStatus(task.id, "done", comment)).toMatchObject({ ok: true });
  expect(((await officeApi.getTask(task.id)) as { task: { status: string } }).task.status).toBe(
    "done",
  );
  const { comments } = (await officeApi.listTaskComments(task.id)) as {
    comments: Array<{ body: string }>;
  };
  expect(comments.some((entry) => entry.body === comment)).toBe(true);
});
