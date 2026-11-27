import { test, expect } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { KanbanPage } from "../../pages/kanban-page";
import { cardPRUnlinkLabels, seedCardWithMultiplePRs } from "./pr-unlink-fixtures";

test.describe("Kanban card PR unlink menu", () => {
  test("desktop context and dots menus show exact choices and unlink one association", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    const seed = await seedCardWithMultiplePRs(
      apiClient,
      backend,
      seedData,
      "Desktop card PR unlink menu",
    );
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const card = kanban.taskCard(seed.task.id);
    await expect(card).toBeVisible();
    await expect(card.getByTestId(`pr-task-icon-${seed.task.id}`)).toBeVisible({ timeout: 15_000 });

    await kanban.openTaskContextMenu(seed.task.id);
    const contextEdit = testPage.getByTestId("kanban-edit-submenu");
    await expect(contextEdit).toBeVisible();
    await contextEdit.focus();
    await testPage.keyboard.press("ArrowRight");
    await expect(testPage.getByRole("menuitem", { name: cardPRUnlinkLabels.api })).toBeVisible();
    await expect(testPage.getByRole("menuitem", { name: cardPRUnlinkLabels.web })).toBeVisible();
    await waitForFiniteAnimations(testPage.getByRole("menu").last());
    await prCapture.screenshot("desktop-kanban-card-pr-unlink-menu", {
      caption: "Desktop Kanban card PR unlink menu",
    });
    await testPage.keyboard.press("Escape");
    await testPage.keyboard.press("Escape");
    await expect(contextEdit).toBeHidden();

    await kanban.openTaskActionsMenu(seed.task.id);
    const dropdownEdit = testPage.getByTestId("kanban-edit-submenu");
    await dropdownEdit.focus();
    await testPage.keyboard.press("ArrowRight");
    await testPage.getByRole("menuitem", { name: cardPRUnlinkLabels.api }).click();

    await expect
      .poll(async () =>
        (await apiClient.listTaskPRs(seed.task.id)).map((pr) => `${pr.repo}#${pr.pr_number}`),
      )
      .toEqual(["web#42"]);
    await expect(card.getByTestId(`pr-task-icon-${seed.task.id}`)).toHaveAttribute(
      "data-pr-count",
      "1",
    );

    await testPage.reload();
    await kanban.board.waitFor({ state: "visible" });
    await expect(
      kanban.taskCard(seed.task.id).getByTestId(`pr-task-icon-${seed.task.id}`),
    ).toHaveAttribute("data-pr-count", "1");
    await expect
      .poll(async () =>
        (await apiClient.listTaskPRs(seed.task.id)).map((pr) => `${pr.repo}#${pr.pr_number}`),
      )
      .toEqual(["web#42"]);

    await kanban.openTaskActionsMenu(seed.task.id);
    const finalEdit = testPage.getByTestId("kanban-edit-submenu");
    await finalEdit.focus();
    await testPage.keyboard.press("ArrowRight");
    await testPage.getByRole("menuitem", { name: cardPRUnlinkLabels.web }).click();

    await expect.poll(() => apiClient.listTaskPRs(seed.task.id)).toEqual([]);
    await expect(
      kanban.taskCard(seed.task.id).getByTestId(`pr-task-icon-${seed.task.id}`),
    ).toBeHidden();

    await testPage.reload();
    await kanban.board.waitFor({ state: "visible" });
    await expect(
      kanban.taskCard(seed.task.id).getByTestId(`pr-task-icon-${seed.task.id}`),
    ).toHaveCount(0);
    await expect.poll(() => apiClient.listTaskPRs(seed.task.id)).toEqual([]);
  });
});
