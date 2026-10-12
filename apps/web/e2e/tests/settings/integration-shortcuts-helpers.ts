import type { Page } from "@playwright/test";
import { expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";

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
  const saved = waitForHttp(page, "PATCH", /\/api\/v1\/user\/settings$/, {
    predicate: (response) => response.request().postDataJSON()?.keyboard_shortcuts !== undefined,
  });
  const button = save.getByRole("button", { name: /^(Save changes|Retry save)$/ });
  if (touch) await button.tap();
  else await button.click();
  expect((await saved).status()).toBe(200);
  await expect(save).not.toBeVisible();
}
