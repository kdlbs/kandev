import { expect, test } from "../../fixtures/test-base";
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

test("virtualizes a large tablet column in the snap-scrolling layout", async ({
  tabletTestPage,
  apiClient,
  seedData,
}) => {
  await seedLargeColumnTasks(apiClient, seedData, "Tablet large column task");

  await tabletTestPage.goto("/");
  const layout = tabletTestPage.getByTestId("tablet-kanban-layout");
  await expect(layout).toBeVisible();

  const column = layout.getByTestId(`kanban-column-${seedData.startStepId}`);
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
  await expect(tabletTestPage).toHaveURL(new RegExp(`/t/${reachedTaskId}`));
});
