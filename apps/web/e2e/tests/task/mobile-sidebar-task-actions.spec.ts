import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("Mobile sidebar task actions", () => {
  test("opens task actions on long press without covering diff stats", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const taskTitle = "Mobile task with diff stats";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      taskTitle,
      seedData.agentProfileId,
      {
        description: "/e2e:diff-update-setup",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(
      session.chat.getByText("diff-update-setup complete", { exact: false }),
    ).toBeVisible({
      timeout: 60_000,
    });

    await testPage.getByTestId("mobile-session-menu").click();
    const sheet = testPage.getByRole("dialog");
    const taskRow = sheet.getByTestId("sidebar-task-item").filter({ hasText: taskTitle });
    const diffStats = taskRow.getByTestId("sidebar-task-diff-stats");
    const actions = taskRow.getByRole("button", { name: "Task actions" });

    await expect(diffStats).toBeVisible({ timeout: 15_000 });
    await expect(actions).toBeHidden();

    await taskRow.dispatchEvent("pointerdown", {
      pointerType: "touch",
      isPrimary: true,
      button: 0,
      clientX: 120,
      clientY: 240,
    });
    await testPage.waitForTimeout(1000);
    await taskRow.dispatchEvent("pointerup", {
      pointerType: "touch",
      isPrimary: true,
      button: 0,
      clientX: 120,
      clientY: 240,
    });

    await expect(testPage.getByRole("menuitem", { name: "Archive" })).toBeVisible();
  });
});
