import { expect, test } from "../../fixtures/test-base";
import {
  setAgentRuntimeAvailability,
  stubAgentRuntimeRestart,
} from "../../helpers/agent-runtime-availability";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

test.describe("Mobile agent runtime availability", () => {
  test("keeps the task route safe around the bottom navigation", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const taskTitle = "Mobile runtime availability task";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      taskTitle,
      seedData.agentProfileId,
      {
        description: 'e2e:message("mobile runtime availability")',
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    if (!task.session_id) throw new Error("mobile runtime availability task has no session");

    await stubAgentRuntimeRestart(testPage);
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await expect(mobile.taskCard(task.id)).toBeVisible();
    await mobile.taskCard(task.id).tap();
    await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}$`));
    await expect(testPage.getByTestId("mobile-task-layout")).toBeVisible();

    await setAgentRuntimeAvailability(testPage, {
      status: "unavailable",
      reason: "agentctl_exited",
      occurred_at: "2026-08-08T14:22:52Z",
    });

    const alert = testPage.getByTestId("agent-runtime-alert");
    await expect(alert).toBeVisible();
    await expect(alert).toContainText("Local agent runtime stopped");
    await expect(testPage.getByTestId("mobile-task-layout")).toBeVisible();

    const action = alert.getByRole("button", { name: "Restart Kandev" });
    const actionBox = await action.boundingBox();
    expect(actionBox, "runtime recovery action has no rendered hitbox").not.toBeNull();
    expect(actionBox?.height ?? 0).toBeGreaterThanOrEqual(44);

    const bottomNav = testPage.getByTestId("mobile-task-layout").locator("nav");
    await expect(bottomNav).toBeVisible();
    const alertBox = await alert.boundingBox();
    const bottomNavBox = await bottomNav.boundingBox();
    expect(alertBox).not.toBeNull();
    expect(bottomNavBox).not.toBeNull();
    if (alertBox && bottomNavBox) {
      expect(alertBox.y + alertBox.height).toBeLessThanOrEqual(bottomNavBox.y + 1);
    }

    await assertNoDocumentHorizontalOverflow(testPage, "mobile agent runtime alert");

    await setAgentRuntimeAvailability(testPage, { status: "available" });
    await expect(alert).toHaveCount(0);
    await expect(bottomNav).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "mobile agent runtime recovery");
  });
});
