import { test, expect } from "../../fixtures/test-base";
import { chooseArchiveCommand, waitForTaskList } from "./task-list-archive-helpers";

let baseline: { workspace_id: string; workflow_filter_id: string; repository_ids: string[] };
test.beforeEach(async ({ testPage, apiClient }) => {
  void testPage;
  const { settings } = await apiClient.getUserSettings();
  baseline = {
    workspace_id: settings.workspace_id ?? "",
    workflow_filter_id: settings.workflow_filter_id ?? "",
    repository_ids: settings.repository_ids ?? [],
  };
});
test.afterEach(async ({ apiClient }) => {
  if (baseline) await apiClient.saveUserSettings(baseline);
});

for (const entry of ["task", "list", "settings"] as const) {
  // @covers AC-TASKS-LIST-ARCHIVE-FILTERS-001.1 AC-TASKS-LIST-ARCHIVE-FILTERS-001.2 AC-TASKS-LIST-ARCHIVE-FILTERS-001.3
  test(`opens the whole workspace archive from ${entry} without saving scope`, async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const other = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Archive second workflow",
      "simple",
    );
    const { steps } = await apiClient.listWorkflowSteps(other.id);
    const archived = await apiClient.createTask(seedData.workspaceId, "Archived primary result", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    const otherArchived = await apiClient.createTask(
      seedData.workspaceId,
      "Archived second result",
      {
        workflow_id: other.id,
        workflow_step_id: steps[0].id,
      },
    );
    await apiClient.createTask(seedData.workspaceId, "Active excluded result", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.archiveTask(archived.id);
    await apiClient.archiveTask(otherArchived.id);
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: seedData.workflowId,
      repository_ids: [seedData.repositoryId],
    });
    const beforeSessions = (await apiClient.listTaskSessions(archived.id)).sessions.length;
    const href = {
      task: `/t/${archived.id}`,
      list: `/tasks?workspace=${seedData.workspaceId}&q=absent`,
      settings: "/settings",
    }[entry];
    await testPage.goto(href);
    await expect(testPage.getByRole("main")).toBeVisible();
    await chooseArchiveCommand(testPage);
    await expect
      .poll(async () =>
        (await testPage.getByTestId("tasks-list-row-title").allTextContents()).sort(),
      )
      .toEqual(["Archived primary result", "Archived second result"]);
    await expect(testPage.getByTestId("tasks-list-search")).toHaveValue("");
    expect((await apiClient.getUserSettings()).settings).toMatchObject({
      workflow_filter_id: seedData.workflowId,
      repository_ids: [seedData.repositoryId],
    });
    const restored = waitForTaskList(testPage, "archived");
    await testPage.reload();
    await restored;
    await expect(testPage.getByTestId("tasks-list-row-title")).toHaveCount(2);
    await testPage
      .getByTestId("tasks-list-row")
      .filter({ hasText: "Archived primary result" })
      .click();
    await expect(testPage.getByTestId("task-unarchive-button")).toBeVisible();
    expect((await apiClient.listTaskSessions(archived.id)).sessions).toHaveLength(beforeSessions);
  });
}

// @covers AC-TASKS-LIST-ARCHIVE-FILTERS-004.1 AC-TASKS-LIST-ARCHIVE-FILTERS-002.1
test("updates an open list through mode picks and browser history", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await apiClient.createTask(seedData.workspaceId, "Active history task", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const archived = await apiClient.createTask(seedData.workspaceId, "Archived history task", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await apiClient.archiveTask(archived.id);
  await testPage.goto(
    `/tasks?workspace=${seedData.workspaceId}&archive=archived&workflow=all&repository=all`,
  );
  await expect(testPage.getByTestId("tasks-list-row-title")).toHaveText(["Archived history task"]);
  const all = waitForTaskList(testPage, "all");
  await testPage.getByTestId("tasks-list-archive-all").click();
  await all;
  await expect(testPage.getByTestId("tasks-list-row-title")).toHaveCount(2);
  const back = waitForTaskList(testPage, "archived");
  await testPage.goBack();
  await back;
  await expect(testPage.getByTestId("tasks-list-row-title")).toHaveText(["Archived history task"]);
  const forward = waitForTaskList(testPage, "all");
  await testPage.goForward();
  await forward;
  await expect(testPage.getByTestId("tasks-list-row-title")).toHaveCount(2);
});
