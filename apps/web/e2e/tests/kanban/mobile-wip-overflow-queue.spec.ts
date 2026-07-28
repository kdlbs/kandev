import { expect, test } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { useRegularMode } from "../../helpers/regular-mode";

useRegularMode();

test("shows queued overflow and admitted count in the focused mobile column", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Mobile Queue Workflow");
  const reviewStep = await apiClient.createWorkflowStep(workflow.id, "Review", 0, {
    is_start_step: true,
  });
  await apiClient.createWorkflowStep(workflow.id, "Done", 1);
  await apiClient.updateWorkflowStep(reviewStep.id, { wip_limit: 2 });
  await apiClient.saveUserSettings({
    workspace_id: seedData.workspaceId,
    workflow_filter_id: workflow.id,
  });

  for (let index = 1; index <= 7; index += 1) {
    await apiClient.createTask(seedData.workspaceId, `Mobile Queue Review ${index}`, {
      workflow_id: workflow.id,
      workflow_step_id: reviewStep.id,
    });
  }

  const mobile = new MobileKanbanPage(testPage);
  await mobile.goto();
  await expect(mobile.boardNavigator).toContainText("Review");
  await expect(mobile.taskCardByTitle("Mobile Queue Review 7")).toBeVisible();
  await mobile.boardNavigator.click();
  await expect(testPage.getByTestId("column-tab-0")).toContainText("2/2");
  await expect(testPage.getByTestId("column-tab-0")).toHaveAttribute("data-active", "true");
  await prCapture.screenshot("mobile-visible-queue", {
    caption: "Mobile Kanban focused column showing visible queued overflow and admitted count.",
  });

  const pageWidth = await testPage.evaluate(() => ({
    scroll: document.documentElement.scrollWidth,
    client: document.documentElement.clientWidth,
  }));
  expect(pageWidth.scroll).toBeLessThanOrEqual(pageWidth.client);
});
