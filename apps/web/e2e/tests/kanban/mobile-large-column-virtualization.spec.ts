import { expect, test } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
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

test("keeps the focused mobile column virtualized and touch-navigable", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await seedLargeColumnTasks(apiClient, seedData, "Mobile large column task");

  const mobile = new MobileKanbanPage(testPage);
  await mobile.goto();
  await expect(mobile.mobileKanbanLayout()).toBeVisible();
  await expect(testPage.getByTestId("desktop-kanban-stage-navigator")).toHaveCount(0);

  await mobile.boardNavigator.click();
  const startColumnTab = testPage
    .getByTestId("mobile-board-navigator-drawer")
    .locator('[data-testid^="column-tab-"]')
    .filter({ hasText: String(LARGE_COLUMN_TASK_COUNT) });
  await expect(startColumnTab).toHaveCount(1);
  await startColumnTab.tap();

  const column = mobile.mobileKanbanLayout().getByTestId(`kanban-column-${seedData.startStepId}`);
  await expect(column).toBeVisible();
  await expect.poll(() => taskCards(column).count()).toBeGreaterThan(0);

  const initialCardIds = await mountedTaskCardIds(column);
  await expectBoundedMountedCards(column);

  const pageWidth = await testPage.evaluate(() => ({
    scroll: document.documentElement.scrollWidth,
    client: document.documentElement.clientWidth,
  }));
  expect(pageWidth.scroll).toBeLessThanOrEqual(pageWidth.client);

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

  await column.getByTestId(reachedCardTestId).tap();
  await expect(testPage).toHaveURL(new RegExp(`/t/${reachedTaskId}`));
});
