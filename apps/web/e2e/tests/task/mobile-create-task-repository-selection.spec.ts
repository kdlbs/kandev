import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { useRegularMode } from "../../helpers/regular-mode";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

useRegularMode();

test.describe("Create task workspace repository picker on mobile", () => {
  test("marks another row's repository while keeping it selectable", async ({
    testPage,
    prCapture,
  }) => {
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await mobile.mobileFab.click();

    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    const repositoryChips = dialog.getByTestId("repo-chip-trigger");
    await expect(repositoryChips.first()).toContainText("E2E Repo");

    await dialog.getByTestId("add-repository").click();
    await expect(repositoryChips).toHaveCount(2);
    await repositoryChips.nth(1).tap();

    const selectedElsewhere = testPage.getByRole("option", { name: /^E2E Repo/ });
    await expect(selectedElsewhere).toBeVisible();
    await expect(selectedElsewhere.getByTestId("already-added-repository-marker")).toBeVisible();
    await selectedElsewhere.tap();
    await expect(repositoryChips.nth(1)).toContainText("E2E Repo");
    await testPage.waitForTimeout(300);
    await expect(testPage.getByRole("tooltip")).toBeHidden();
    await assertNoDocumentHorizontalOverflow(testPage, "repository picker selection");
    await prCapture.screenshot("mobile-repository-chip-selection", {
      caption:
        "After mobile repository selection, the chip stays contained and no tooltip is shown.",
    });
  });
});
