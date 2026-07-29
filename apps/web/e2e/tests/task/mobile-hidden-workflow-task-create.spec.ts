import { test, expect } from "../../fixtures/test-base";

test.describe("Mobile task creation from hidden workflow context", () => {
  test("creates the new task in the visible workflow", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const hidden = await apiClient.e2eCreateHiddenWorkflow(
      seedData.workspaceId,
      "Hidden mobile task-detail workflow",
    );
    const hiddenStart = await apiClient.createWorkflowStep(hidden.id, "Improve", 0, {
      is_start_step: true,
    });
    const sourceTask = await apiClient.createTask(seedData.workspaceId, "Hidden mobile source", {
      description: "Source task in the hidden workflow",
      workflow_id: hidden.id,
      workflow_step_id: hiddenStart.id,
    });

    await testPage.goto(`/t/${sourceTask.id}`);
    await testPage.getByTestId("mobile-session-menu").tap();
    const taskDrawer = testPage.getByRole("dialog", { name: "Tasks" });
    await expect(taskDrawer).toBeVisible();
    await taskDrawer.getByRole("button", { name: "New", exact: true }).tap();

    const createDialog = testPage.getByTestId("create-task-dialog");
    await expect(createDialog).toBeVisible();
    await createDialog.getByTestId("task-title-input").fill("Visible mobile workflow task");
    await createDialog
      .getByTestId("task-description-input")
      .fill("Created from hidden mobile task detail");
    const startAgent = createDialog.getByTestId("submit-start-agent");
    await expect(startAgent).toBeEnabled({ timeout: 30_000 });
    const createOnly = createDialog.getByRole("button", { name: "Create only", exact: true });
    await expect(createOnly).toBeEnabled();
    await createOnly.tap();

    await expect
      .poll(() => taskIdFromURL(testPage.url()), { timeout: 15_000 })
      .not.toBe(sourceTask.id);
    const createdTaskId = taskIdFromURL(testPage.url());
    const createdTask = await apiClient.getTask(createdTaskId);
    expect(createdTask.workflow_step_id).toBe(seedData.startStepId);
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
  });
});

function taskIdFromURL(url: string): string {
  const match = url.match(/\/t\/([^/?]+)/);
  if (!match) throw new Error(`Cannot extract task ID from URL: ${url}`);
  return match[1];
}
