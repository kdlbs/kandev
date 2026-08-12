import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

/**
 * The kanban card is covered by tests/kanban/task-title-hover-subtasks.spec.ts.
 * The title hover also ships on the two other surfaces named in the plan (the
 * /tasks rich list row and the AppSidebar task row), and those read their
 * subtasks from the same store slices the kanban board fills. Whether those
 * slices are populated on a page that never mounts the board is exactly the
 * kind of thing a unit test with a hand-seeded store cannot answer, so it is
 * asserted here against a real backend.
 */
// Backend caps task titles at 60 characters, so these sit just under it.
const PARENT_TITLE = "Surface hover parent with a title long enough to clamp";
const CHILD_ONE_TITLE = "Surface child one with its own long title";
const CHILD_TWO_TITLE = "Surface child two";

async function seedParentWithSubtasks(apiClient: ApiClient, seedData: SeedData) {
  await apiClient.saveUserSettings({
    workspace_id: seedData.workspaceId,
    enable_preview_on_click: false,
  });
  // No agent is started: an auto-started session's on_turn_complete would move
  // the task mid-test and detach the hover target from under the assertion.
  const parent = await apiClient.createTask(seedData.workspaceId, PARENT_TITLE, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  const childOne = await apiClient.createTask(seedData.workspaceId, CHILD_ONE_TITLE, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
    parent_id: parent.id,
    workspace_mode: "inherit_parent",
  });
  const childTwo = await apiClient.createTask(seedData.workspaceId, CHILD_TWO_TITLE, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
    parent_id: parent.id,
    workspace_mode: "inherit_parent",
  });
  return { parent, childOne, childTwo };
}

test.describe("Task title hover card on the non-Kanban surfaces", () => {
  test("AC12/AC13/AC14: the /tasks rich list row shows the full title and clickable subtask rows", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { parent, childOne } = await seedParentWithSubtasks(apiClient, seedData);

    await testPage.goto("/tasks");
    await testPage.waitForLoadState("networkidle");

    const parentRow = testPage
      .getByTestId("tasks-list")
      .getByTestId("tasks-list-row")
      .filter({ hasText: PARENT_TITLE })
      .first();
    await expect(parentRow).toBeVisible({ timeout: 45_000 });

    await parentRow.getByTestId("tasks-list-row-title").hover();

    const hoverCard = testPage.getByTestId("task-title-hover-card");
    await expect(hoverCard).toBeVisible();
    await expect(hoverCard).toContainText(PARENT_TITLE);

    // AC13: the subtask rows are the point of the card. On this page the task
    // list is local component state, so an empty list here would mean the
    // hover silently degrades to a title-only tooltip.
    const rowOne = hoverCard.getByTestId(`task-subtask-row-${childOne.id}`);
    await expect(rowOne).toBeVisible();
    await expect(rowOne).toContainText(CHILD_ONE_TITLE);
    await expect(hoverCard).toContainText(CHILD_TWO_TITLE);

    // AC20: opening the card must not push the document sideways.
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);

    // AC14: the row navigates to the subtask, not to the parent it sits on.
    await rowOne.click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${childOne.id}`));
    expect(testPage.url()).not.toContain(parent.id);
  });

  test("AC16: an archived subtask drops out of the hover card, the live one stays", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { childOne, childTwo } = await seedParentWithSubtasks(apiClient, seedData);
    await apiClient.archiveTask(childTwo.id);

    await testPage.goto("/tasks");
    await testPage.waitForLoadState("networkidle");

    const parentRow = testPage
      .getByTestId("tasks-list")
      .getByTestId("tasks-list-row")
      .filter({ hasText: PARENT_TITLE })
      .first();
    await expect(parentRow).toBeVisible({ timeout: 45_000 });
    await parentRow.getByTestId("tasks-list-row-title").hover();

    const hoverCard = testPage.getByTestId("task-title-hover-card");
    await expect(hoverCard.getByTestId(`task-subtask-row-${childOne.id}`)).toBeVisible();
    // The archived child must not keep advertising itself from the parent's card.
    await expect(hoverCard.getByTestId(`task-subtask-row-${childTwo.id}`)).toHaveCount(0);
    await expect(hoverCard).not.toContainText(CHILD_TWO_TITLE);
  });

  test("AC12/AC13: the AppSidebar task row shows the full title and its subtask rows", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { childOne } = await seedParentWithSubtasks(apiClient, seedData);

    await testPage.goto("/tasks");
    await testPage.waitForLoadState("networkidle");

    const sidebarRow = testPage
      .getByTestId("app-sidebar")
      .getByTestId("sidebar-task-item")
      .filter({ hasText: PARENT_TITLE })
      .first();
    await expect(sidebarRow).toBeVisible({ timeout: 45_000 });

    await sidebarRow.getByText(PARENT_TITLE, { exact: false }).first().hover();

    const hoverCard = testPage.getByTestId("task-title-hover-card");
    await expect(hoverCard).toBeVisible();
    await expect(hoverCard).toContainText(PARENT_TITLE);
    await expect(hoverCard.getByTestId(`task-subtask-row-${childOne.id}`)).toBeVisible();
    await expect(hoverCard).toContainText(CHILD_TWO_TITLE);
  });
});
