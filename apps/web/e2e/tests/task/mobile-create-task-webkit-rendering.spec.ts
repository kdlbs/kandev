import { expect, test } from "../../fixtures/test-base";
import { expectWebkitDialogMotion } from "../../helpers/dialog-webkit-metrics";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { useRegularMode } from "../../helpers/regular-mode";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

useRegularMode();

test("keeps the WebKit Create Task dialog full-height and contained on mobile", async ({
  testPage,
  prCapture,
}) => {
  const mobile = new MobileKanbanPage(testPage);
  await mobile.goto();
  await testPage.locator("html").evaluate((root) => {
    root.setAttribute("data-rendering-engine", "webkit");
  });
  await mobile.mobileFab.tap();

  const dialog = testPage.getByTestId("create-task-dialog");
  await expect(dialog).toBeVisible();
  await expect(dialog.getByRole("button", { name: "Close", exact: true })).toHaveCount(0);
  await expect(dialog.getByRole("button", { name: "Cancel", exact: true })).toBeVisible();
  await expect(dialog).toHaveCSS("padding-top", "0px");
  await expectWebkitDialogMotion(dialog, {
    contentZIndex: "50",
    overlayZIndex: "49",
  });
  await expect(dialog.getByTestId("task-title-input")).toBeVisible();
  await expect(dialog.getByTestId("task-description-input")).toBeVisible();

  const viewport = testPage.viewportSize();
  const box = await dialog.boundingBox();
  expect(viewport).not.toBeNull();
  expect(box).not.toBeNull();
  expect(box!.x).toBeCloseTo(0, 0);
  expect(box!.y).toBeCloseTo(0, 0);
  expect(box!.width).toBeCloseTo(viewport!.width, 0);
  expect(box!.height).toBeCloseTo(viewport!.height, 0);

  await assertNoDocumentHorizontalOverflow(testPage, "WebKit Create Task dialog");
  await prCapture.screenshot("webkit-create-task-dialog-mobile", {
    caption: "The WebKit Create Task dialog remains full-height and contained on mobile.",
  });
});
