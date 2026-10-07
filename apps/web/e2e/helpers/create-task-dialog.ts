import { expect, type Page } from "@playwright/test";

export async function openCreateTaskDialog(page: Page) {
  const button = page.getByTestId("create-task-button").first();
  const expand = page.getByTestId("sidebar-navigation-expand");
  await expect
    .poll(async () => {
      const receivesPointer = await button.evaluate((element) => {
        const bounds = element.getBoundingClientRect();
        const target = document.elementFromPoint(
          bounds.x + bounds.width / 2,
          bounds.y + bounds.height / 2,
        );
        return target !== null && element.contains(target);
      });
      if (receivesPointer) return true;
      if ((await expand.isVisible()) && (await expand.getAttribute("aria-expanded")) === "false") {
        await expand.click();
      }
      return false;
    })
    .toBe(true);
  await button.click();
}
