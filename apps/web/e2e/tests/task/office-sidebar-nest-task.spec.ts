import { test, expect } from "../../fixtures/office-fixture";
import { SessionPage } from "../../pages/session-page";

function taskId(task: Record<string, unknown>): string {
  return task.id as string;
}

test.describe("Office sidebar Nest under", () => {
  // @covers AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.1
  // @covers AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.4
  test("nests an Office parent under another Office task", async ({
    testPage,
    apiClient,
    officeApi,
    seedData,
  }) => {
    const project = await officeApi.createProject(seedData.workspaceId, "Office nesting project");
    const projectId = taskId(project);
    const subject = await apiClient.createTask(seedData.workspaceId, "Office nesting subject", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    const subjectId = subject.id;
    const descendant = await apiClient.createTask(
      seedData.workspaceId,
      "Office nesting descendant",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        parent_id: subjectId,
      },
    );
    const descendantId = descendant.id;
    const target = await apiClient.createTask(seedData.workspaceId, "Office nesting target", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    const targetId = target.id;

    await Promise.all([
      officeApi.updateTaskProjectId(subjectId, projectId),
      officeApi.updateTaskProjectId(descendantId, projectId),
      officeApi.updateTaskProjectId(targetId, projectId),
    ]);
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: seedData.workflowId,
      keyboard_shortcuts: {},
      enable_preview_on_click: false,
      sidebar_views: [],
    });

    await testPage.goto(`/t/${subjectId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.sidebar).toBeVisible({ timeout: 10_000 });

    const taskBlock = (id: string) =>
      session.sidebar.locator(`[data-testid="sortable-task-block"][data-task-id="${id}"]`);
    await expect(taskBlock(subjectId)).toHaveAttribute("data-depth", "0");
    await expect(taskBlock(descendantId)).toHaveAttribute("data-depth", "1");
    await expect(taskBlock(targetId)).toHaveAttribute("data-depth", "0");

    const subjectRow = session.sidebar
      .locator("[data-testid='sidebar-task-item']")
      .filter({ hasText: "Office nesting subject" });
    await subjectRow.hover();
    await subjectRow.getByRole("button", { name: "Task actions" }).click();
    await testPage.getByRole("menuitem", { name: "Nest under" }).hover();

    await expect(testPage.getByRole("menuitem", { name: "Office nesting descendant" })).toHaveCount(
      0,
    );
    await testPage.getByRole("menuitem", { name: "Office nesting target" }).click();

    await expect(taskBlock(subjectId)).toHaveAttribute("data-depth", "1", { timeout: 10_000 });
    await expect(taskBlock(descendantId)).toHaveAttribute("data-depth", "2");
    await expect(
      taskBlock(targetId).locator(
        `[data-testid="sortable-task-block"][data-task-id="${subjectId}"]`,
      ),
    ).toHaveCount(1);

    await expect
      .poll(async () => (await apiClient.getTask(subjectId)).parent_id ?? "")
      .toBe(targetId);

    await testPage.reload();
    await session.waitForLoad();
    await expect(taskBlock(subjectId)).toHaveAttribute("data-depth", "1", { timeout: 10_000 });
    await expect(taskBlock(descendantId)).toHaveAttribute("data-depth", "2");
  });
});
