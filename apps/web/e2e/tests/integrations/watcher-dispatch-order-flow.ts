import { type Locator, type Page, expect } from "@playwright/test";
import { LinearSettingsPage } from "../../pages/linear-settings-page";
import type { ApiClient } from "../../helpers/api-client";

// Scopes a Radix Select trigger by its field label. The dialog renders each
// field as `<div class="space-y-1.5"><Label>…</Label><Select>…</Select></div>`,
// so the exact label text uniquely identifies the combobox via its parent.
function comboboxByLabel(root: Locator, label: string): Locator {
  return root.getByText(label, { exact: true }).locator("xpath=..").getByRole("combobox");
}

// Drives the full Linear watcher create → save → reopen flow and asserts the
// per-watch "Dispatch order" (sortBy) selection survives a round-trip through
// the backend edit path. Shared by the desktop (linear-settings.spec.ts) and
// mobile (mobile-linear-watcher-dispatch-order.spec.ts) suites so both
// viewports exercise the real dialog flow, not just payload mapping.
export async function assertWatcherDispatchOrderPersists(testPage: Page, apiClient: ApiClient) {
  // A configured integration plus one team so the watcher filter can be satisfied.
  await apiClient.mockLinearReset();
  await apiClient.mockLinearSetTeams([{ id: "team-1", key: "ENG", name: "Engineering" }]);
  await apiClient.setLinearConfig({ secret: "lin_api_xxx" });
  await apiClient.waitForIntegrationAuthHealthy("linear");

  const settings = new LinearSettingsPage(testPage);
  await settings.goto();

  await testPage.getByRole("button", { name: /new watcher/i }).click();
  const dialog = testPage.getByRole("dialog");
  await expect(dialog).toBeVisible();

  const pick = async (label: string, option: string | RegExp) => {
    await comboboxByLabel(dialog, label).click();
    await testPage.getByRole("listbox").getByRole("option", { name: option }).click();
  };

  // Minimum fields to enable Save (isWatchFormReady): workspace, a non-empty
  // filter (team), workflow, and workflow step. Prompt is pre-filled with the
  // default. Workspace must be set before workflow/step — switching it clears
  // those workspace-scoped fields.
  await pick("Workspace", "E2E Workspace");
  await pick("Team", /ENG/);
  await pick("Workflow", "E2E Workflow");
  await comboboxByLabel(dialog, "Workflow Step").click();
  await testPage.getByRole("listbox").getByRole("option").first().click();

  // Choose a non-default dispatch order, then save.
  await pick("Dispatch order", "Priority (high → low)");
  const createButton = dialog.getByRole("button", { name: "Create" });
  await expect(createButton).toBeEnabled();
  await createButton.click();
  await expect(dialog).toBeHidden();

  // Reopen the saved watcher (row click → edit) and confirm the order persisted
  // through create + reload rather than only living in the create payload.
  await testPage.getByText("team:ENG").first().click();
  const editDialog = testPage.getByRole("dialog");
  await expect(editDialog.getByText("Edit Linear Watcher")).toBeVisible();
  await expect(comboboxByLabel(editDialog, "Dispatch order")).toContainText(
    "Priority (high → low)",
  );
}
