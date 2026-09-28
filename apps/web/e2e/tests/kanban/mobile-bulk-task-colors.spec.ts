import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import {
  seedColorTasks,
  expectStoredColors,
  expectNoHorizontalOverflow,
} from "../../helpers/bulk-task-colors";

// @covers AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006.6
test("phone selection supports color, dismissal, clearing, and bulk actions", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await testPage.setViewportSize({ width: 393, height: 851 });
  const [a, b, control] = await seedColorTasks(apiClient, seedData);
  const board = new KanbanPage(testPage);
  await board.goto();
  const select = testPage.getByTestId("mobile-select-tasks");
  await select.tap();
  await expect(select).toHaveAttribute("aria-pressed", "true");
  await select.tap();
  await expect(select).toHaveAttribute("aria-pressed", "false");
  await select.tap();
  await board.taskCard(a.id).tap();
  await board.taskCard(b.id).tap();
  await expect(board.multiSelectToolbar).toContainText("2 selected");
  const color = testPage.getByTestId("bulk-color-button");
  await color.tap();
  const picker = testPage.getByTestId("bulk-color-picker");
  await expect(picker).toBeVisible();
  const dialog = testPage.getByRole("dialog", { name: "Color for 2 tasks" });
  await dialog.evaluate(async (el) => {
    await Promise.all(
      el
        .getAnimations({ subtree: true })
        .filter((a) => Number.isFinite(a.effect?.getComputedTiming().iterations))
        .map((a) => a.finished.catch(() => undefined)),
    );
  });
  const bounds = await dialog.boundingBox();
  expect(bounds!.x).toBeGreaterThanOrEqual(0);
  expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(393);
  for (const button of await picker.getByRole("button").all()) {
    expect((await button.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  }
  await expectNoHorizontalOverflow(testPage);
  await testPage.screenshot({ path: "/tmp/bulk-task-colors-phone.png" });
  await dialog.getByRole("button", { name: "Close", exact: true }).tap();
  await expect(color).toBeFocused();
  await expectStoredColors(apiClient, [a.id, b.id], null);
  await color.tap();
  await picker.getByTestId("bulk-color-option-green").tap();
  await expectStoredColors(apiClient, [a.id, b.id], "green");
  await expectStoredColors(apiClient, [control.id], null);
  await expect(board.multiSelectToolbar).toContainText("2 selected");
  await color.tap();
  await picker.getByTestId("bulk-color-option-none").tap();
  await expectStoredColors(apiClient, [a.id, b.id], null);
  await color.tap();
  await picker.getByTestId("bulk-color-option-purple").tap();
  await expectStoredColors(apiClient, [a.id, b.id], "purple");
  await testPage.getByTestId("bulk-actions-button").tap();
  await expect(testPage.getByTestId("bulk-archive-button")).toBeVisible();
  await expect(testPage.getByTestId("bulk-delete-button")).toBeVisible();
  await testPage.keyboard.press("Escape");
  for (const width of [767, 768]) {
    await testPage.setViewportSize({ width, height: 851 });
    await expect(testPage.getByTestId("bulk-actions-button")).toHaveCount(width < 768 ? 1 : 0);
    await expectNoHorizontalOverflow(testPage);
  }
  await testPage.setViewportSize({ width: 393, height: 851 });
  await testPage.goto(`/t/${control.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await testPage.reload();
  await session.waitForLoad();
  await session.mobileSessionMenu.tap();
  const drawer = testPage.getByTestId("mobile-task-switcher-list");
  await expect(
    drawer
      .getByTestId("sidebar-task-item")
      .filter({ hasText: a.title })
      .getByTestId("task-item-color-marker"),
  ).toHaveAttribute("data-color-token", "purple");
});
