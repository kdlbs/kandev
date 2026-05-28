import { type Page, expect } from "@playwright/test";
import { LinearSettingsPage } from "../../pages/linear-settings-page";

// Drives the Linear watcher create dialog: picks a real agent profile, then
// re-selects "(use step default)" and asserts the field clears back. Shared by
// the desktop (linear-settings.spec.ts) and mobile (mobile-*.spec.ts) suites so
// both viewports exercise the same sentinel-reset behavior.
export async function assertWatcherAgentProfileResetsToStepDefault(testPage: Page) {
  const settings = new LinearSettingsPage(testPage);
  await settings.goto();

  await testPage.getByRole("button", { name: /new watcher/i }).click();
  const dialog = testPage.getByRole("dialog");
  await expect(dialog).toBeVisible();

  // Scope to the Agent Profile field; the label text is unique within the form.
  const agentTrigger = dialog
    .locator("div.space-y-1\\.5")
    .filter({ hasText: "Agent Profile" })
    .getByRole("combobox");

  // The select defaults to the sentinel option.
  await expect(agentTrigger).toContainText("(use step default)");

  // Pick the first real profile — option 0 is the "(use step default)" sentinel.
  await agentTrigger.click();
  const profileOption = testPage.getByRole("option").nth(1);
  await expect(profileOption).toBeVisible();
  const profileLabel = ((await profileOption.textContent()) ?? "").trim();
  expect(profileLabel.length).toBeGreaterThan(0);
  await profileOption.click();
  await expect(agentTrigger).toContainText(profileLabel);

  // Re-select "(use step default)" and confirm the field reverts.
  await agentTrigger.click();
  await testPage.getByRole("option", { name: "(use step default)" }).click();
  await expect(agentTrigger).toContainText("(use step default)");
  await expect(agentTrigger).not.toContainText(profileLabel);
}
