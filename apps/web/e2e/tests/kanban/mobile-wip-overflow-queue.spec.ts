import { expect, test } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { SessionPage } from "../../pages/session-page";
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

test("moves through the mobile task switcher and shows touch queue status", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const workflow = await apiClient.createWorkflow(
    seedData.workspaceId,
    "Mobile Move Queue Workflow",
  );
  const backlogStep = await apiClient.createWorkflowStep(workflow.id, "Backlog", 0, {
    is_start_step: true,
  });
  const reviewStep = await apiClient.createWorkflowStep(workflow.id, "Review", 1);
  const doneStep = await apiClient.createWorkflowStep(workflow.id, "Done", 2);
  await apiClient.updateWorkflowStep(reviewStep.id, { wip_limit: 1 });
  await apiClient.saveUserSettings({
    workspace_id: seedData.workspaceId,
    workflow_filter_id: workflow.id,
  });

  await apiClient.createTask(seedData.workspaceId, "Mobile Admitted Review", {
    workflow_id: workflow.id,
    workflow_step_id: reviewStep.id,
  });
  const source = await apiClient.seedTask(seedData.workspaceId, "Mobile Moved Queue", {
    workflow_id: workflow.id,
    workflow_step_id: backlogStep.id,
  });
  const anchor = await apiClient.seedTask(seedData.workspaceId, "Mobile Queue Anchor", {
    workflow_id: workflow.id,
    workflow_step_id: backlogStep.id,
  });

  await testPage.goto(`/t/${anchor.task_id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await testPage.getByTestId("mobile-session-menu").tap();
  const sheet = testPage.getByRole("dialog", { name: "Tasks" });
  const sourceRow = sheet.getByTestId("sidebar-task-item").filter({
    hasText: "Mobile Moved Queue",
  });
  await expect(sourceRow).toBeVisible({ timeout: 15_000 });
  await sourceRow.getByRole("button", { name: "Task actions" }).tap();
  await testPage.getByTestId("task-context-move-to").tap();
  await testPage.getByTestId(`task-context-step-${reviewStep.id}`).tap();

  await expect
    .poll(async () => (await apiClient.getTask(source.task_id)).workflow_step_id)
    .toBe(reviewStep.id);
  await testPage.keyboard.press("Escape");
  await expect(testPage.getByRole("dialog", { name: "Tasks" })).toHaveCount(0);
  await testPage.getByTestId("mobile-session-menu").tap();
  const queuedSourceRow = testPage
    .getByRole("dialog", { name: "Tasks" })
    .getByTestId("sidebar-task-item")
    .filter({ hasText: "Mobile Moved Queue" });
  const queueStatus = queuedSourceRow.getByTestId("sidebar-task-wip-queue");
  await expect(queueStatus).toBeVisible();
  await expect(queueStatus.locator("svg")).toBeVisible();
  await expect(queueStatus).toHaveAttribute("aria-label", "Position 1 of 1 in Review queue");
  await queueStatus.focus();
  await expect(testPage.getByRole("tooltip")).toHaveText("Position 1 of 1 in Review queue");
  await testPage.keyboard.press("Escape");

  const mobile = new MobileKanbanPage(testPage);
  await testPage.goto(`/?workflowId=${workflow.id}`);
  await mobile.goto();
  await mobile.boardNavigator.click();
  await testPage.getByTestId("column-tab-1").tap();
  const reviewColumn = testPage.getByTestId(`kanban-column-${reviewStep.id}`);
  await expect(reviewColumn.getByTestId("kanban-queued-section")).toBeVisible();
  await expect(mobile.taskCardByTitle("Mobile Moved Queue")).toContainText("Queued for Review");

  await testPage.goto(`/t/${anchor.task_id}`);
  await session.waitForLoad();
  await testPage.getByTestId("mobile-session-menu").tap();
  const admittedRow = testPage
    .getByRole("dialog", { name: "Tasks" })
    .getByTestId("sidebar-task-item")
    .filter({ hasText: "Mobile Admitted Review" });
  await admittedRow.getByRole("button", { name: "Task actions" }).tap();
  await testPage.getByTestId("task-context-move-to").tap();
  await testPage.getByTestId(`task-context-step-${doneStep.id}`).tap();

  await expect
    .poll(async () => (await apiClient.getTask(source.task_id)).workflow_step_id)
    .toBe(reviewStep.id);
  await testPage.keyboard.press("Escape");
  await expect(testPage.getByRole("dialog", { name: "Tasks" })).toHaveCount(0);
  await testPage.getByTestId("mobile-session-menu").tap();
  await expect(
    testPage
      .getByRole("dialog", { name: "Tasks" })
      .getByTestId("sidebar-task-item")
      .filter({ hasText: "Mobile Moved Queue" })
      .getByTestId("sidebar-task-wip-queue"),
  ).toHaveCount(0);
  await expect(
    await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
  ).toBe(true);
});
