import { test, expect } from "../fixtures/test-base";

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
      executorProfileId: seedData.worktreeExecutorProfileId,
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
      taskMode: "normal_task",
      repositoryMode: "selected",
      repositories: [{ repository_id: seedData.repositoryId, base_branch: "main" }],
      agentProfileId: seedData.agentProfileId,
      executorProfileId: seedData.worktreeExecutorProfileId,
      prompt: 'e2e:delay(5000)\ne2e:message("visible-task-ok")',
    });

    const result = await apiClient.triggerAutomationManual(automation.id);
    expect(result.run_task_id).toBeTruthy();

    const { tasks } = await apiClient.listTasks(seedData.workspaceId);
    const task = tasks.find((candidate) => candidate.id === result.run_task_id);
    expect(task).toBeTruthy();
    expect(task?.workflow_step_id).toBe(seedData.startStepId);

    await testPage.goto(`/t/${result.run_task_id!}`);
    await expect(testPage.getByText("visible-task-ok", { exact: true })).toBeVisible({
      timeout: 30_000,
    });
  });
});
