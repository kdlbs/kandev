import { expect, test } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

useRegularMode();

test("keeps Create Task open after hardware-keyboard Escape on mobile", async ({
  testPage,
  prCapture,
}) => {
  const mobile = new MobileKanbanPage(testPage);
  await mobile.goto();
  await mobile.mobileFab.tap();

  const dialog = testPage.getByTestId("create-task-dialog");
  const textarea = dialog.getByTestId("task-description-input");
  await expect(dialog).toHaveAttribute("data-state", "open");
  await textarea.fill("Keep this mobile draft");

  await textarea.press("Escape");

  await expect(dialog).toHaveAttribute("data-state", "open");
  await expect(textarea).toHaveValue("Keep this mobile draft");
  await expect(textarea).toBeFocused();
  await prCapture.screenshot("create-task-dialog-after-escape-mobile", {
    caption: "Create Task stays open with the draft after Escape on mobile.",
  });
});

test("closes prompt autocomplete and keeps focus after hardware-keyboard Escape on mobile", async ({
  testPage,
}) => {
  const mobile = new MobileKanbanPage(testPage);
  await mobile.goto();
  await mobile.mobileFab.tap();

  const dialog = testPage.getByTestId("create-task-dialog");
  const textarea = dialog.getByTestId("task-description-input");
  const menuTitle = testPage.getByText(/Mention tasks, files, prompts/i);
  await textarea.pressSequentially("@");
  await expect(menuTitle).toBeVisible();

  await testPage.keyboard.press("Escape");

  await expect(menuTitle).toHaveCount(0);
  await expect(dialog).toHaveAttribute("data-state", "open");
  await expect(textarea).toBeFocused();
  await textarea.pressSequentially("continued");
  await expect(textarea).toHaveValue("@continued");
});
