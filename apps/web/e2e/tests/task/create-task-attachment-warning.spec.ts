import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";

useRegularMode();

test("warns when a pasted image is too large for a new task", async ({ testPage }) => {
  const kanban = new KanbanPage(testPage);
  await kanban.goto();
  await kanban.createTaskButton.first().click();

  const prompt = testPage.getByTestId("task-description-input");
  await expect(prompt).toBeEditable();
  await prompt.evaluate((element) => {
    const image = new File([new Uint8Array(14 * 1024 * 1024)], "copied-image.png", {
      type: "image/png",
    });
    const clipboardData = new DataTransfer();
    clipboardData.items.add(image);
    const pasteEvent = new Event("paste", { bubbles: true, cancelable: true });
    Object.defineProperty(pasteEvent, "clipboardData", { value: clipboardData });
    element.dispatchEvent(pasteEvent);
  });

  const warning = testPage
    .getByTestId("toast-message")
    .filter({ hasText: "Attachment is too large" });
  await expect(warning).toContainText(
    "copied-image.png is 14.0 MB. The maximum file size is 10.0 MB.",
  );

  const overlay = testPage.locator('[data-slot="dialog-overlay"]:visible');
  await expect(overlay).toBeVisible();
  const [toastZIndex, overlayZIndex] = await Promise.all([
    testPage
      .getByTestId("toast-container")
      .evaluate((element) => Number.parseInt(getComputedStyle(element).zIndex, 10)),
    overlay.evaluate((element) => Number.parseInt(getComputedStyle(element).zIndex, 10)),
  ]);
  expect(
    toastZIndex,
    "attachment warning should render above the task dialog overlay",
  ).toBeGreaterThan(overlayZIndex);
});
