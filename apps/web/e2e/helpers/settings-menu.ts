import { expect, type Page } from "@playwright/test";

export const SETTINGS_APPEARANCE_PATH = "/settings/preferences/appearance";
/** The settings takeover's left panel — the menu every mode renders into. */
export const SETTINGS_TAKEOVER_TESTID = "app-sidebar-settings-mode";

/**
 * Choose a settings menu mode through the real control, so a spec exercises
 * the same preview → save path a user does rather than seeding localStorage
 * behind it.
 *
 * The default is `accordion`, but specs choose a mode explicitly so branch
 * assertions do not depend on a device's remembered preference.
 */
export async function setSettingsMenuMode(
  testPage: Page,
  mode: "flat" | "accordion" | "persistent",
): Promise<void> {
  await testPage.goto(SETTINGS_APPEARANCE_PATH);
  const option = testPage.getByTestId(`settings-menu-mode-${mode}`);
  const radio = option.getByRole("radio");
  if ((await radio.getAttribute("aria-checked")) === "true") {
    return;
  }

  await option.click();
  const floatingSave = testPage.getByTestId("settings-floating-save");
  await floatingSave.getByRole("button", { name: "Save changes" }).click();
  await expect(floatingSave).not.toBeVisible({ timeout: 15_000 });
}
