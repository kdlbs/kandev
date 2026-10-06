import type { Page } from "@playwright/test";
import { expect } from "../../fixtures/test-base";

export const INTEGRATION_SHORTCUTS_SETTINGS_PATH = "/settings/preferences/keyboard-shortcuts";
export const INTEGRATION_CHORD = "Control+Alt+g";

export async function recordIntegrationShortcut(page: Page, slug: string, touch = false) {
  const recorder = page.getByTestId(`shortcut-recorder-integration:${slug}`);
  await expect(recorder).toHaveCount(1);
  if (touch) await recorder.tap();
  else await recorder.click();
  await expect(recorder).toHaveAttribute("data-shortcut-recording", "true");
  await page.keyboard.press(INTEGRATION_CHORD);
  await expect(recorder).toHaveAttribute("data-shortcut-recording", "false");
  return recorder;
}

export async function saveIntegrationShortcuts(page: Page, touch = false) {
  const save = page.getByTestId("settings-floating-save");
  if (touch) await save.getByRole("button", { name: "Save changes" }).tap();
  else await save.getByRole("button", { name: "Save changes" }).click();
  await expect(save).not.toBeVisible({ timeout: 15_000 });
}
