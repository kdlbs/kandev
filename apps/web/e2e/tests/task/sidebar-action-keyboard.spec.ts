import { test, expect } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { SessionPage } from "../../pages/session-page";
import { seedKeyboardRows, tabTo } from "./sidebar-action-keyboard-helpers";

test("desktop nested task actions retain native keyboard activation", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const { navigation, target } = await seedKeyboardRows(apiClient, seedData);
  await testPage.goto(`/t/${navigation.task_id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  const row = session.sidebar
    .getByTestId("sidebar-task-item")
    .filter({ hasText: "Keyboard action target" });
  await expect(row).toBeVisible();
  const actions = row.getByRole("button", { name: "Task actions" });
  await tabTo(testPage, row);
  await row.press("Enter");
  await expect(testPage).toHaveURL(new RegExp(`/t/${target.task_id}`));
  await testPage.goto(`/t/${navigation.task_id}`);
  await new SessionPage(testPage).waitForLoad();
  const originalUrl = testPage.url();
  const selected = await row.getAttribute("data-multiselected");

  for (const key of ["Enter", "Space"]) {
    await tabTo(testPage, actions);
    await actions.press(key);
    const menu = testPage.getByRole("menu").last();
    await expect(menu).toBeVisible();
    expect(testPage.url()).toBe(originalUrl);
    expect(await row.getAttribute("data-multiselected")).toBe(selected);
    await waitForFiniteAnimations(menu);
    if (key === "Enter")
      await prCapture.screenshot("desktop-sidebar-keyboard-actions", {
        caption: "Desktop task actions opened by keyboard",
      });
    await testPage.keyboard.press("Escape");
    await expect(menu).toBeHidden();
  }
});
