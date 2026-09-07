import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

/**
 * The palette's scope strip does not shrink. With four scopes it no longer fits
 * beside the search field on a phone, so it has to wrap onto its own row rather
 * than squeeze the query out of view.
 */
const MODIFIER = process.platform === "darwin" ? "Meta" : "Control";
const QUERY = "a reasonably long typed query";

test("@search the palette scope strip wraps below the field on a phone", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);

  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile palette scopes",
    seedData.agentProfileId,
    {
      description: "/e2e:steer-fold-setup",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await testPage.goto(`/t/${task.id}`);
  await new SessionPage(testPage).waitForLoad();

  await testPage.keyboard.press(`${MODIFIER}+k`);
  const dialog = testPage.getByRole("dialog");
  await expect(dialog).toBeVisible({ timeout: 10_000 });

  const input = dialog.getByRole("combobox");
  await input.fill(QUERY);
  await expect(input).toHaveValue(QUERY);

  const tablist = dialog.getByRole("tablist", { name: "Command palette mode" });
  await expect(tablist.getByRole("tab")).toHaveCount(4);

  const inputBox = (await input.boundingBox())!;
  const tablistBox = (await tablist.boundingBox())!;
  const dialogBox = (await dialog.boundingBox())!;

  // The strip is on a row of its own, under the field.
  expect(tablistBox.y).toBeGreaterThanOrEqual(inputBox.y + inputBox.height);
  // The field keeps most of the dialog rather than collapsing to a few characters.
  expect(inputBox.width).toBeGreaterThan(dialogBox.width * 0.6);

  // Neither row pushes the document wider than the viewport.
  const overflows = await testPage.evaluate(
    () => document.documentElement.scrollWidth > window.innerWidth,
  );
  expect(overflows).toBe(false);

  // A task row keeps its title on a narrow screen; the metadata is what yields.
  await input.fill("Mobile palette scopes");
  const taskRow = dialog.getByRole("option").filter({ hasText: "Mobile palette scopes" }).first();
  await expect(taskRow).toBeVisible({ timeout: 15_000 });
  await expect(taskRow.getByTestId("task-state-running")).toBeVisible();
  await expect(taskRow.getByRole("img", { name: "In progress" })).toBeVisible();
  const titleBox = (await taskRow.getByText("Mobile palette scopes").first().boundingBox())!;
  expect(titleBox.width).toBeGreaterThan(60);
});

// @covers AC-TASKS-COMMAND-PANEL-ARCHIVED-TASKS-001.1, AC-TASKS-COMMAND-PANEL-ARCHIVED-TASKS-001.2,
// AC-TASKS-COMMAND-PANEL-ARCHIVED-TASKS-001.3, AC-TASKS-COMMAND-PANEL-ARCHIVED-TASKS-001.4,
// AC-TASKS-COMMAND-PANEL-ARCHIVED-TASKS-001.8
test("@search archived task results keep their cues and title space on a phone", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const archivedInProgress = await apiClient.createTask(
    seedData.workspaceId,
    "Mobile archived in progress",
    {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    },
  );
  await apiClient.archiveTask(archivedInProgress.id);

  await new KanbanPage(testPage).goto();
  await testPage.keyboard.press(`${MODIFIER}+k`);
  const dialog = testPage.getByRole("dialog");
  await expect(dialog).toBeVisible({ timeout: 10_000 });

  const input = dialog.getByRole("combobox");
  await input.fill("Mobile archived");
  const taskResults = dialog.getByTestId("command-panel-task-preview");
  await expect(taskResults).toBeVisible({ timeout: 10_000 });

  const archivedOption = taskResults
    .getByRole("option")
    .filter({ hasText: archivedInProgress.title });
  await expect(archivedOption).toBeVisible();
  await expect(archivedOption.getByTestId("task-state-archived")).toBeVisible();
  await expect(archivedOption.getByRole("img", { name: "Archived" })).toBeVisible();
  await expect(archivedOption.getByText("Archived", { exact: true })).toBeVisible();

  const titleBox = (await archivedOption.getByText(archivedInProgress.title).boundingBox())!;
  expect(titleBox.width).toBeGreaterThan(60);
  const overflows = await testPage.evaluate(
    () => document.documentElement.scrollWidth > window.innerWidth,
  );
  expect(overflows).toBe(false);
});
