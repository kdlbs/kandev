import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";

// The link-issue dialog (Link → Linear Issue) renames the task to
// "<KEY>: <title>". The backend rejects titles longer than 60 characters, so
// the composed title must be truncated client-side instead of surfacing
// "task title is too long". Covers the desktop kanban card menu; the mobile
// task-switcher sheet reaches the same dialog component.
useRegularMode();

test.describe("Linear issue link on long task titles", () => {
  test.beforeEach(async ({ apiClient }) => {
    await apiClient.setLinearConfig({
      secret: "lin_api_xxx",
    });
    await apiClient.waitForIntegrationAuthHealthy("linear");
  });

  test("linking an issue truncates the renamed title instead of failing validation", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.mockLinearAddIssues([
      {
        id: "issue-1",
        identifier: "ENG-12",
        title: "Add billing endpoint",
        description: "",
        url: "https://linear.app/mock-org/issue/ENG-12",
      },
    ]);

    const longTitle = "x".repeat(60);
    const task = await apiClient.createTask(seedData.workspaceId, longTitle, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.openTaskActionsMenu(task.id);

    await testPage.getByTestId("task-context-link").hover();
    await testPage.getByTestId("task-context-link-linear-issue").click();

    const input = testPage.getByTestId("task-external-link-input");
    await expect(input).toBeVisible();
    await input.fill("ENG-12");
    await testPage.getByTestId("task-external-link-submit").click();

    // The rename succeeds (dialog closes) and the card shows the truncated
    // 60-character title with the issue key preserved.
    await expect(input).not.toBeVisible({ timeout: 10_000 });
    await expect(kanban.taskCardByTitle(`ENG-12: ${"x".repeat(51)}…`)).toBeVisible({
      timeout: 10_000,
    });
  });
});
