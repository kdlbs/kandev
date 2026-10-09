import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";

/**
 * Seeds a parent task with two subtasks. No agent is started (mirrors the
 * MR/PR badge specs' rationale): an auto-started session's on_turn_complete
 * would move a card mid-test and detach the hover target from under the
 * assertion.
 */
async function seedParentWithSubtasks(apiClient: ApiClient, seedData: SeedData, title: string) {
  await apiClient.saveUserSettings({
    workspace_id: seedData.workspaceId,
    enable_preview_on_click: false,
  });
  const parent = await apiClient.createTask(seedData.workspaceId, title, {
    description: "Title hover fixture parent",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  const childOne = await apiClient.createTask(
    seedData.workspaceId,
    "First subtask with a long enough title to wrap",
    {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      parent_id: parent.id,
      workspace_mode: "inherit_parent",
    },
  );
  const childTwo = await apiClient.createTask(seedData.workspaceId, "Second subtask", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
    parent_id: parent.id,
    workspace_mode: "inherit_parent",
  });
  return { parent, childOne, childTwo };
}

test.describe("Task title hover card on the Kanban card", () => {
  test("AC12/AC13/AC14: hovering the title shows the full bold title and subtask rows, and clicking a row navigates", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const longTitle = "A parent task title long enough to truncate on card";
    const { parent, childOne, childTwo } = await seedParentWithSubtasks(
      apiClient,
      seedData,
      longTitle,
    );

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const card = kanban.taskCard(parent.id);
    await expect(card).toBeVisible({ timeout: 45_000 });

    const title = card.getByTestId("task-card-title");
    await title.hover();

    const hoverCard = testPage.getByTestId("task-title-hover-card");
    await expect(hoverCard).toBeVisible();
    // AC12: the full, untruncated title renders (the card's own <p> is
    // line-clamped, so this line only passes if the hover card itself
    // carries the un-clamped copy).
    await expect(hoverCard).toContainText(longTitle);

    const rowOne = hoverCard.getByTestId(`task-subtask-row-${childOne.id}`);
    const rowTwo = hoverCard.getByTestId(`task-subtask-row-${childTwo.id}`);
    await expect(rowOne).toBeVisible();
    await expect(rowTwo).toBeVisible();
    await expect(rowOne).toContainText("First subtask");
    await expect(rowTwo).toContainText("Second subtask");

    // AC20: no document-level horizontal overflow while the hover card is open.
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);

    // AC14: clicking a subtask row navigates without triggering the parent
    // card's own click handler (which would open the parent task instead).
    await rowOne.click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${childOne.id}`));
  });

  test("AC15: a task with no subtasks does not open a title hover card", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const task = await apiClient.createTask(seedData.workspaceId, "Childless title hover task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const card = kanban.taskCard(task.id);
    await expect(card).toBeVisible({ timeout: 45_000 });

    await card.getByTestId("task-card-title").hover();
    await dwell(
      testPage,
      300,
      "negative-assertion",
      "a childless Kanban card must not open a title preview",
    );
    await expect(testPage.getByTestId("task-title-hover-card")).toHaveCount(0);
  });

  test("the open hover card does not swallow the click that opens the parent task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    // The card is an interactive HoverCard, not a pointer-events-none tooltip,
    // so it really does sit over the board once open. Opening the parent task
    // by clicking its card is the board's primary action and must survive.
    const { parent } = await seedParentWithSubtasks(apiClient, seedData, "Clickable parent card");

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const card = kanban.taskCard(parent.id);
    await expect(card).toBeVisible({ timeout: 45_000 });

    const title = card.getByTestId("task-card-title");
    await title.hover();
    await expect(testPage.getByTestId("task-title-hover-card")).toBeVisible();

    await title.click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${parent.id}`));
  });

  // @covers AC-TASKS-RICH-TASK-TITLE-PREVIEWS-001.2, AC-TASKS-RICH-TASK-TITLE-PREVIEWS-001.3, AC-TASKS-RICH-TASK-TITLE-PREVIEWS-001.4
  test("multi-select closes and suppresses title previews, then restores them when it ends", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { parent } = await seedParentWithSubtasks(
      apiClient,
      seedData,
      "Preview selection parent",
    );
    const sibling = await apiClient.createTask(seedData.workspaceId, "Preview selection sibling", {
      description: "Second preview eligible task",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const parentCard = kanban.taskCard(parent.id);
    const siblingCard = kanban.taskCard(sibling.id);
    await expect(parentCard).toBeVisible({ timeout: 45_000 });
    await expect(siblingCard).toBeVisible({ timeout: 45_000 });

    const parentTitle = parentCard.getByTestId("task-card-title");
    await parentTitle.hover();
    const hoverCard = testPage.getByTestId("task-title-hover-card");
    await expect(hoverCard).toBeVisible();

    // Modifier-click enters selection mode while the pointer remains on the
    // open title preview, so this proves the mode transition itself closes it.
    await parentTitle.click({ modifiers: ["ControlOrMeta"] });
    await expect(kanban.multiSelectToolbar).toContainText("1 selected");
    await expect(hoverCard).toHaveCount(0);

    const siblingTitle = siblingCard.getByTestId("task-card-title");
    await siblingTitle.hover();
    await dwell(
      testPage,
      300,
      "negative-assertion",
      "multi-select must suppress a preview on an unselected title",
    );
    await expect(hoverCard).toHaveCount(0);
    await siblingTitle.click();
    await expect(kanban.multiSelectToolbar).toContainText("2 selected");
    await expect(kanban.board).toBeVisible();
    await expect(testPage).not.toHaveURL(new RegExp(`/t/${sibling.id}`));

    await kanban.multiSelectToggle.first().click();
    await expect(kanban.multiSelectToolbar).not.toBeVisible();
    await parentTitle.hover();
    await expect(hoverCard).toBeVisible();
  });
});
