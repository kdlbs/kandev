import { test, expect } from "../../fixtures/office-fixture";
import { SessionPage } from "../../pages/session-page";

function taskId(task: Record<string, unknown>): string {
  return task.id as string;
}

test.describe("Mobile Office sidebar Nest under", () => {
  // @covers AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.1
  // @covers AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.4
  // @covers AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.5
  test("nests an Office parent under another Office task", async ({
    testPage,
    apiClient,
    officeApi,
    seedData,
  }) => {
    const project = await officeApi.createProject(
      seedData.workspaceId,
      "Mobile Office nesting project",
    );
    const projectId = taskId(project);
    const subject = await apiClient.createTask(
      seedData.workspaceId,
      "Mobile Office nesting subject",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    const descendant = await apiClient.createTask(
      seedData.workspaceId,
      "Mobile Office nesting descendant",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        parent_id: subject.id,
      },
    );
    const target = await apiClient.createTask(
      seedData.workspaceId,
      "Mobile Office nesting target",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await Promise.all([
      officeApi.updateTaskProjectId(subject.id, projectId),
      officeApi.updateTaskProjectId(descendant.id, projectId),
      officeApi.updateTaskProjectId(target.id, projectId),
    ]);
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: seedData.workflowId,
      keyboard_shortcuts: {},
      enable_preview_on_click: false,
      sidebar_views: [],
    });

    await testPage.goto(`/t/${subject.id}`);
    await new SessionPage(testPage).waitForLoad();
    await testPage.getByTestId("mobile-session-menu").tap();

    const taskSheet = testPage.getByRole("dialog", { name: "Tasks" });
    const subjectRow = taskSheet
      .getByTestId("sidebar-task-item")
      .filter({ hasText: "Mobile Office nesting subject" });
    await expect(subjectRow).toBeVisible();
    await subjectRow.getByRole("button", { name: "Task actions" }).tap();
    await testPage.getByRole("menuitem", { name: "Nest under" }).tap();
    await expect(
      testPage.getByRole("menuitem", { name: "Mobile Office nesting descendant" }),
    ).toHaveCount(0);
    await testPage.getByRole("menuitem", { name: "Mobile Office nesting target" }).tap();

    const taskBlock = (id: string) =>
      taskSheet.locator(`[data-testid="sortable-task-block"][data-task-id="${id}"]`);
    await expect(taskBlock(subject.id)).toHaveAttribute("data-depth", "1", { timeout: 10_000 });
    await expect(taskBlock(descendant.id)).toHaveAttribute("data-depth", "2");
    await expect
      .poll(async () => (await apiClient.getTask(subject.id)).parent_id ?? "")
      .toBe(target.id);
  });
});
