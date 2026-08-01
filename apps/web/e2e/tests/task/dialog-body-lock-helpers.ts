import { expect, type Page } from "@playwright/test";
import { KanbanPage } from "../../pages/kanban-page";

async function bodyLockState(page: Page) {
  return page.evaluate(() => ({
    pointerEvents: document.body.style.pointerEvents,
    scrollLocked: document.body.hasAttribute("data-scroll-locked"),
  }));
}

export async function verifyStalledDialogCloseRecovery(page: Page, touch: boolean) {
  const kanban = new KanbanPage(page);
  await kanban.goto();
  await page.addStyleTag({
    content: `
      [data-state="closed"][data-slot="dialog-content"],
      [data-state="closed"][data-slot="dialog-overlay"] {
        animation-duration: 60s !important;
      }
    `,
  });
  await page.evaluate(() => {
    const accordion = document.createElement("div");
    accordion.dataset.slot = "accordion-content";
    accordion.dataset.state = "open";
    accordion.dataset.testid = "body-lock-open-accordion";
    document.body.appendChild(accordion);
  });

  const createTask = touch
    ? page.getByTestId("mobile-fab")
    : kanban.createTaskButton.filter({ visible: true }).first();
  if (touch) await createTask.tap();
  else await createTask.click();

  const dialog = page.getByTestId("create-task-dialog");
  await expect(dialog).toBeVisible();
  await expect
    .poll(() => bodyLockState(page))
    .toEqual({
      pointerEvents: "none",
      scrollLocked: true,
    });

  // Create Task intentionally omits the generic top-right Close control; the
  // footer Cancel action is the shared dismissal path for both viewports.
  const cancel = dialog.getByRole("button", { name: "Cancel", exact: true });
  if (touch) await cancel.tap();
  else await cancel.click();

  await expect(dialog).not.toBeAttached();
  await expect(page.locator('[data-slot="dialog-overlay"]')).not.toBeAttached();
  await expect
    .poll(() => bodyLockState(page))
    .toEqual({
      pointerEvents: "",
      scrollLocked: false,
    });

  if (touch) await createTask.tap();
  else await createTask.click();

  await expect(dialog).toBeVisible();
  await expect
    .poll(() => bodyLockState(page))
    .toEqual({
      pointerEvents: "none",
      scrollLocked: true,
    });
}
