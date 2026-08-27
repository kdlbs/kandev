import { expect, type Page } from "@playwright/test";

export const SETTINGS_APPEARANCE_PATH = "/settings/preferences/appearance";
/** The settings takeover's left panel — the menu every mode renders into. */
export const SETTINGS_TAKEOVER_TESTID = "app-sidebar-settings-mode";

/**
 * Choose a settings menu mode through the real control, so a spec exercises
 * the same preview → save path a user does rather than seeding localStorage
 * behind it.
 *
 * `flat` is the default and renders no branches at all, so any spec asserting
 * on a branch's rows (a workspace's tabs, its integrations) has to opt into a
 * tree mode first.
 */
export async function setSettingsMenuMode(
  testPage: Page,
  mode: "flat" | "accordion" | "persistent",
): Promise<void> {
  await testPage.goto(SETTINGS_APPEARANCE_PATH);
  await testPage.getByTestId(`settings-menu-mode-${mode}`).click();
  const floatingSave = testPage.getByTestId("settings-floating-save");
  await floatingSave.getByRole("button", { name: "Save changes" }).click();
  await expect(floatingSave).not.toBeVisible({ timeout: 15_000 });
}
