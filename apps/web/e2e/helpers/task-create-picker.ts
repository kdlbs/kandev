import { expect, type Page } from "@playwright/test";

/** Finish repository popup exit before opening the next task-creation picker. */
export async function waitForRepositoryPickerClose(page: Page): Promise<void> {
  const picker = page.locator('[data-slot="popover-content"]').filter({
    has: page.getByPlaceholder("Search repositories...", { exact: true }),
  });
  await expect(picker).toHaveCount(0);
}
