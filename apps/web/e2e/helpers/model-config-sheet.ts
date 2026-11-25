import { expect, type Page } from "@playwright/test";

/**
 * Closes the phone model sheet when it is open. Desktop popovers do not render
 * the sheet close control, so callers can use this after any model selection
 * without branching on the project.
 */
export async function closeModelConfigSheetIfOpen(page: Page): Promise<void> {
  const close = page.getByTestId("model-config-sheet-close");
  if ((await close.count()) === 0 || !(await close.isVisible())) return;
  await close.tap();
  await expect(close).toBeHidden();
}
