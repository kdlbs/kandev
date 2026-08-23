import { test, expect } from "../fixtures/test-base";
import type { ApiClient } from "../helpers/api-client";

async function localExecutorProfile(apiClient: ApiClient): Promise<string> {
  const { executors } = await apiClient.listExecutors();
  const local = executors.find((executor) => executor.type === "local");
  const profileId = local?.profiles?.[0]?.id;
  expect(profileId, "the E2E workspace must expose a Local executor profile").toBeTruthy();
  return profileId!;
}

test.describe("Automation target modes", () => {
  test("runs a hidden automation without a workflow or repository", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Repository-free report",
      taskMode: "automation_run",
      repositoryMode: "none",
      agentProfileId: seedData.agentProfileId,
      executorProfileId: await localExecutorProfile(apiClient),
      prompt: 'e2e:message("scratch-run-ok")',
    });

    const result = await apiClient.triggerAutomationManual(automation.id);
    expect(result.run_task_id).toBeTruthy();

    await testPage.goto(`/t/${result.run_task_id!}`);
    await expect(testPage.getByText("scratch-run-ok", { exact: true })).toBeVisible({
      timeout: 30_000,
    });

    const { tasks } = await apiClient.listTasks(seedData.workspaceId);
    expect(tasks.some((task) => task.id === result.run_task_id)).toBe(false);
  });

  test("creates a visible normal task in the selected workflow", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Visible workflow task",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
      taskMode: "normal_task",
      repositoryMode: "selected",
      repositoryIds: [seedData.repositoryId],
      agentProfileId: seedData.agentProfileId,
      executorProfileId: seedData.worktreeExecutorProfileId,
      prompt: 'e2e:message("visible-task-ok")',
    });

    const result = await apiClient.triggerAutomationManual(automation.id);
    expect(result.run_task_id).toBeTruthy();

    await testPage.goto(`/t/${result.run_task_id!}`);
    await expect(testPage.getByText("visible-task-ok", { exact: true })).toBeVisible({
      timeout: 30_000,
    });

    const { tasks } = await apiClient.listTasks(seedData.workspaceId);
    expect(tasks.some((task) => task.id === result.run_task_id)).toBe(true);
  });
});
