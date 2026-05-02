import { test, expect } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";

type TopbarMetrics = {
  workflowRight: number;
  actionsLeft: number;
  actionsRight: number;
  headerRight: number;
};

async function readTopbarMetrics(page: Page) {
  return page.evaluate<TopbarMetrics | null>(() => {
    const header = document.querySelector('[data-testid="task-topbar"]');
    const workflow = document.querySelector('[data-testid="workflow-stepper"]');
    const actions = document.querySelector('[data-testid="topbar-action-overflow"]');
    if (!header || !workflow || !actions) return null;

    const headerRect = header.getBoundingClientRect();
    const workflowRect = workflow.getBoundingClientRect();
    const actionsRect = actions.getBoundingClientRect();

    return {
      workflowRight: workflowRect.right,
      actionsLeft: actionsRect.left,
      actionsRight: actionsRect.right,
      headerRight: headerRect.right,
    };
  });
}

test.describe("Task topbar overflow", () => {
  test("keeps contextual PR actions inline before quick chat overflows", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 1100, height: 720 });

    const task = await apiClient.createTask(
      seedData.workspaceId,
      "Responsive topbar PR review task",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 761,
      pr_url: "https://github.com/testorg/testrepo/pull/761",
      pr_title: "Responsive topbar overflow",
      head_branch: "feature/topbar-overflow",
      base_branch: "main",
      author_login: "test-user",
      additions: 23,
      deletions: 3,
    });

    await testPage.goto(`/t/${task.id}`);

    await expect(testPage.getByTestId("task-topbar")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByTestId("pr-topbar-button")).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("topbar-action-overflow-trigger")).toBeVisible({
      timeout: 10_000,
    });

    const metrics = await readTopbarMetrics(testPage);
    expect(metrics).not.toBeNull();
    expect(metrics!.workflowRight).toBeLessThanOrEqual(metrics!.actionsLeft + 1);
    expect(metrics!.actionsRight).toBeLessThanOrEqual(metrics!.headerRight + 1);

    await testPage.getByTestId("topbar-action-overflow-trigger").click();
    await expect(testPage.getByRole("button", { name: "Chat" })).toBeVisible();
  });
});
