import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

useRegularMode();

test.describe("Create task workspace repository picker on mobile", () => {
  test("marks another row's repository while keeping it selectable", async ({ testPage }) => {
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await mobile.mobileFab.click();

    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    const repositoryChips = dialog.getByTestId("repo-chip-trigger");
    await expect(repositoryChips.first()).toContainText("E2E Repo");

    await dialog.getByTestId("add-repository").click();
    await expect(repositoryChips).toHaveCount(2);
    await repositoryChips.nth(1).click();

    const selectedElsewhere = testPage.getByRole("option", {
      name: /E2E Repo.*Already added/i,
    });
    await expect(selectedElsewhere).toBeVisible();
    await selectedElsewhere.click();
    await expect(repositoryChips.nth(1)).toContainText("E2E Repo");
  });
});
