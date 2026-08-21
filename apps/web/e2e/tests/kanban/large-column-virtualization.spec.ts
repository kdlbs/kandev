import { expect, test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { useRegularMode } from "../../helpers/regular-mode";
import {
  expectBoundedMountedCards,
  LARGE_COLUMN_TASK_COUNT,
  mountedTaskCardIds,
  scrollColumnToBottom,
  seedLargeColumnTasks,
  taskCards,
} from "./large-column-virtualization-helpers";

useRegularMode();

test.setTimeout(120_000);

test("virtualizes a large desktop column while preserving reached-card navigation", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await seedLargeColumnTasks(apiClient, seedData, "Desktop large column task");

  const kanban = new KanbanPage(testPage);
  await kanban.goto();

  const column = kanban.columnByStepId(seedData.startStepId);
  await expect(column).toBeVisible();
  await expect(column.getByText(String(LARGE_COLUMN_TASK_COUNT), { exact: true })).toBeVisible();
  await expect.poll(() => taskCards(column).count()).toBeGreaterThan(0);

  const initialCardIds = await mountedTaskCardIds(column);
  await expectBoundedMountedCards(column);

  const scrollOwner = column.getByTestId("kanban-column-scroll");
  await expect(scrollOwner).toBeVisible();
  await scrollColumnToBottom(scrollOwner);

  await expect
    .poll(async () => {
      const mountedIds = await mountedTaskCardIds(column);
      return mountedIds.some((id) => !initialCardIds.includes(id));
    })
    .toBe(true);
  await expectBoundedMountedCards(column);

  const reachedCard = taskCards(column).last();
  await expect(reachedCard).toBeVisible();
  const reachedCardTestId = await reachedCard.getAttribute("data-testid");
  if (!reachedCardTestId) throw new Error("reached card has no stable test id");
  const reachedTaskId = reachedCardTestId.replace(/^task-card-/, "");

  await column.getByTestId(reachedCardTestId).click();
  await expect(testPage).toHaveURL(new RegExp(`/t/${reachedTaskId}`));
});
