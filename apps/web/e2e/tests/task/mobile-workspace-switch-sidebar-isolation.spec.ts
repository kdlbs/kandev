import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

useRegularMode();

test("mobile creates in the selected workspace after leaving an open task", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const workspaceA = await apiClient.createWorkspace("Mobile Workspace A");
  const { workflows: workspaceAWorkflows } = await apiClient.listWorkflows(workspaceA.id);
  const workflowA = workspaceAWorkflows[0];
  if (!workflowA) throw new Error("mobile workspace A has no bootstrap workflow");
  const { steps: workspaceASteps } = await apiClient.listWorkflowSteps(workflowA.id);
  const startStepA = workspaceASteps.find((step) => step.is_start_step) ?? workspaceASteps[0];
  if (!startStepA) throw new Error("mobile workspace A workflow has no steps");
  const taskA = await apiClient.createTask(workspaceA.id, "Mobile Workspace A Task", {
    workflow_id: workflowA.id,
    workflow_step_id: startStepA.id,
  });

  const mobile = new MobileKanbanPage(testPage);
  await mobile.goto();
  await mobile.mobileMenuButton.tap();
  await testPage.getByTestId("mobile-workspace-trigger").tap();
  await testPage.getByTestId(`mobile-workspace-item-${workspaceA.id}`).tap();
  await expect(mobile.taskCard(taskA.id)).toBeVisible({ timeout: 10_000 });
  await mobile.taskCard(taskA.id).tap();
  await expect(testPage).toHaveURL(new RegExp(`/t/${taskA.id}$`));
  await expect(
    testPage.locator("header").getByText("Mobile Workspace A Task", { exact: true }),
  ).toBeVisible();

  // The phone task workbench has no workspace picker. Its existing back action
  // returns to the board, whose responsive Menu drawer owns workspace switching.
  await testPage.locator('header a[href="/"]').tap();
  await expect(mobile.mobileKanbanLayout()).toBeVisible();
  await mobile.mobileMenuButton.tap();
  await testPage.getByTestId("mobile-workspace-trigger").tap();
  await testPage.getByTestId(`mobile-workspace-item-${seedData.workspaceId}`).tap();

  await expect(testPage).toHaveURL(
    (url) => url.pathname === "/" && url.searchParams.get("workspaceId") === seedData.workspaceId,
  );
  await expect(mobile.taskCard(taskA.id)).not.toBeVisible();

  const createdTitle = "Mobile Workspace B UI Task";
  await mobile.mobileFab.tap();
  const dialog = testPage.getByTestId("create-task-dialog");
  await expect(dialog).toBeVisible();
  await dialog.getByTestId("source-mode-scratch").tap();
  await dialog.getByTestId("task-title-input").fill(createdTitle);
  await dialog.getByTestId("task-description-input").fill("Created after changing workspace");
  const createOnly = dialog.getByRole("button", { name: "Create only" });
  await expect(createOnly).toBeEnabled();

  const createdResponse = testPage.waitForResponse(
    (response) =>
      response.url().endsWith("/api/v1/tasks") && response.request().method() === "POST",
  );
  await createOnly.tap();
  const createdTask = (await createdResponse).json() as Promise<{ id: string }>;
  const createdTaskId = (await createdTask).id;

  await expect(dialog).not.toBeVisible();
  await expect(mobile.taskCard(createdTaskId)).toBeVisible({ timeout: 10_000 });
  await expect(mobile.taskCard(taskA.id)).not.toBeVisible();

  await mobile.mobileMenuButton.tap();
  await testPage.getByTestId("mobile-workspace-trigger").tap();
  await testPage.getByTestId(`mobile-workspace-item-${workspaceA.id}`).tap();
  await expect(mobile.taskCard(taskA.id)).toBeVisible({ timeout: 10_000 });
  await expect(mobile.taskCard(createdTaskId)).not.toBeVisible();
});
