import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { seedKeyboardRows, tabTo } from "./sidebar-action-keyboard-helpers";

test("coarse desktop inline archive controls own their keyboard activation", async ({
  coarseDesktopTestPage: page,
  apiClient,
  seedData,
}) => {
  const { navigation } = await seedKeyboardRows(apiClient, seedData);
  await page.goto(`/t/${navigation.task_id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  const row = session.sidebar
    .getByTestId("sidebar-task-item")
    .filter({ hasText: "Keyboard action target" });
  await expect(row).toBeVisible();
  const originalUrl = page.url();

  async function openArchive() {
    await row.click({ button: "right" });
    await page.getByRole("menuitem", { name: "Archive", exact: true }).click();
    const confirmation = row.getByTestId("task-archive-inline-confirmation");
    await expect(confirmation).toBeVisible();
    return confirmation;
  }

  const first = await openArchive();
  const cancel = first.getByRole("button", { name: "Cancel" });
  await expect(cancel).toBeFocused();
  await cancel.press("Enter");
  await expect(first).toHaveCount(0);
  await expect(row).toBeVisible();
  expect(page.url()).toBe(originalUrl);

  const second = await openArchive();
  const confirm = second.getByRole("button", { name: "Archive", exact: true });
  await tabTo(page, confirm);
  await confirm.press("Space");
  await expect(row).toHaveCount(0);
  expect(page.url()).toBe(originalUrl);
});
