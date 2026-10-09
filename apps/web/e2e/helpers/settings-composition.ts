import type { Page } from "@playwright/test";
export async function openTaskBehaviorRuntime(page: Page): Promise<void> {
  await page.getByRole("tab", { name: "Runtime", exact: true }).click();
}
