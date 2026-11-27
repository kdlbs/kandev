import { test, expect } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { cardPRUnlinkLabels, seedCardWithMultiplePRs } from "./pr-unlink-fixtures";

test.describe("Mobile kanban card PR unlink menu", () => {
  test("unlinks one exact association from the dots menu with touch-sized rows", async ({
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
      "Mobile card PR unlink menu",
    );
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    const card = mobile.taskCard(seed.task.id);
    await expect(card).toBeVisible();
    const trigger = card.getByRole("button", { name: "More options" });
    const triggerBox = await trigger.boundingBox();
    expect(triggerBox).not.toBeNull();
    expect(triggerBox!.width).toBeGreaterThanOrEqual(44);
    expect(triggerBox!.height).toBeGreaterThanOrEqual(44);
    await trigger.tap();
    await testPage.getByRole("menuitem", { name: "Edit" }).tap();
    await testPage.getByTestId("kanban-edit-submenu").tap();
    await waitForFiniteAnimations(testPage.getByRole("menu").last());

    const unlinkChoice = testPage.getByRole("menuitem", { name: cardPRUnlinkLabels.api });
    await expect(unlinkChoice).toBeVisible();
    await waitForFiniteAnimations(testPage.getByRole("menu").last());
    await prCapture.screenshot("mobile-kanban-card-pr-unlink-menu", {
      caption: "Mobile Kanban card PR unlink menu",
    });
    const choiceBox = await unlinkChoice.boundingBox();
    expect(choiceBox).not.toBeNull();
    expect(choiceBox!.height).toBeGreaterThanOrEqual(44);
    const viewport = testPage.viewportSize();
    expect(viewport).not.toBeNull();
    expect(choiceBox!.x).toBeGreaterThanOrEqual(0);
    expect(choiceBox!.x + choiceBox!.width).toBeLessThanOrEqual(viewport!.width + 1);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile card unlink menu");
    await unlinkChoice.tap();

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
    await mobile.board.waitFor({ state: "visible" });
    await expect(
      mobile.taskCard(seed.task.id).getByTestId(`pr-task-icon-${seed.task.id}`),
    ).toHaveAttribute("data-pr-count", "1");
    await expect
      .poll(async () =>
        (await apiClient.listTaskPRs(seed.task.id)).map((pr) => `${pr.repo}#${pr.pr_number}`),
      )
      .toEqual(["web#42"]);
  });
});
