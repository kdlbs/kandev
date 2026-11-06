import { expect, type Page } from "@playwright/test";
export const OPTIONAL_AUTOMATION_PACKAGES = [
  "configuration",
  "rich-output",
  "walkthroughs",
  "diagnostics",
] as const;
export async function saveAutomationMCP(page: Page) {
  const save = page.getByTestId("settings-floating-save");
  await save.getByRole("button", { name: /^(Save changes|Retry save)$/ }).click();
  await expect(save).toHaveAttribute("data-status", "saved", { timeout: 15000 });
}
