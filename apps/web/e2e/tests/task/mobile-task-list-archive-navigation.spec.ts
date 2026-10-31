import { test, expect } from "../../fixtures/test-base";
import { chooseArchiveCommand, waitForTaskList } from "./task-list-archive-helpers";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

// @covers AC-TASKS-LIST-ARCHIVE-FILTERS-001.1 AC-TASKS-LIST-ARCHIVE-FILTERS-001.5 AC-TASKS-LIST-ARCHIVE-FILTERS-003.5
test("browses the archive by touch and offers its palette command on a phone", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await apiClient.createTask(seedData.workspaceId, "Phone active result", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const archived = await apiClient.createTask(seedData.workspaceId, "Phone archived result", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await apiClient.archiveTask(archived.id);
  await testPage.goto("/");
  await testPage.getByTestId("mobile-topbar-page-context").tap();
  await testPage
    .getByRole("dialog", { name: "View options" })
    .getByRole("radio", { name: "List", exact: true })
    .tap();
  await expect(testPage.getByTestId("tasks-list-archive-active")).toBeVisible();
  const loaded = waitForTaskList(testPage, "archived");
  await testPage.getByTestId("tasks-list-archive-archived").tap();
  await loaded;
  await expect(testPage.getByTestId("tasks-list-row-title")).toHaveText(["Phone archived result"]);
  expect(
    (await testPage.getByTestId("tasks-list-archive-archived").boundingBox())!.height,
  ).toBeGreaterThanOrEqual(44);
  await chooseArchiveCommand(testPage, true);
  await testPage.reload();
  await expect(testPage.getByTestId("tasks-list-row-title")).toHaveText(["Phone archived result"]);
  await assertNoDocumentHorizontalOverflow(testPage);
  await testPage.getByTestId("tasks-list-row").tap();
  await expect(testPage.getByTestId("task-unarchive-button")).toBeVisible();
});
