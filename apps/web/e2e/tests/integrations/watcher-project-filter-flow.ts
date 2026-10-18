import { type Locator, type Page, expect } from "@playwright/test";
import { LinearSettingsPage } from "../../pages/linear-settings-page";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";

function comboboxByLabel(root: Locator, label: string): Locator {
  return root.getByText(label, { exact: true }).locator("xpath=..").getByRole("combobox");
}

// Watches survive between tests, so each flow starts from an empty watch list
// and the "project:1 selected" row it opens is the one it created.
async function seedLinearWithProjects(apiClient: ApiClient, seedData: SeedData) {
  await apiClient.deleteLinearIssueWatches(seedData.workspaceId);
  await apiClient.mockLinearReset();
  await apiClient.mockLinearSetTeams([{ id: "team-1", key: "ENG", name: "Engineering" }]);
  await apiClient.mockLinearSetProjects("ENG", [
    { id: "project-checkout", name: "Checkout" },
    { id: "project-search", name: "Search" },
  ]);
  await apiClient.setLinearConfig({ secret: "lin_api_xxx" });
  await apiClient.waitForIntegrationAuthHealthy("linear");
}

async function watchFilter(apiClient: ApiClient, seedData: SeedData, watchId: string) {
  const watch = await apiClient.getLinearIssueWatch(seedData.workspaceId, watchId);
  return watch?.filter;
}

// Creates a watcher through the dialog with one project selected, then reopens
// it and checks the selection survived the backend round-trip.
export async function assertWatcherProjectSelectionPersists(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  prCapture: PrAssetCapture,
) {
  await seedLinearWithProjects(apiClient, seedData);
  const settings = new LinearSettingsPage(testPage);
  await settings.goto();

  await testPage.getByRole("button", { name: /new watcher/i }).click();
  const dialog = testPage.getByRole("dialog");
  await expect(dialog).toBeVisible();
  const pick = async (label: string, option: string | RegExp) => {
    await comboboxByLabel(dialog, label).click();
    await testPage.getByRole("listbox").getByRole("option", { name: option }).click();
  };
  await pick("Workspace", "E2E Workspace");
  await pick("Team", /ENG/);
  await pick("Workflow", "E2E Workflow");
  await comboboxByLabel(dialog, "Workflow Step").click();
  await testPage.getByRole("listbox").getByRole("option").first().click();

  const projects = dialog.getByTestId("linear-watch-projects");
  await projects.getByRole("button", { name: "Checkout" }).click();
  await expect(projects.getByRole("button", { name: "Checkout" })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await projects.scrollIntoViewIfNeeded();
  await prCapture.screenshot("linear-watch-project-filter", {
    caption: "Projects filter in the Linear watcher dialog",
  });
  await dialog.getByRole("button", { name: "Create" }).click();
  await expect(dialog).toBeHidden();

  await testPage.getByText("project:1 selected").first().click();
  const editDialog = testPage.getByRole("dialog");
  await expect(editDialog.getByText("Edit Linear Watcher")).toBeVisible();
  const editProjects = editDialog.getByTestId("linear-watch-projects");
  await expect(editProjects.getByRole("button", { name: "Checkout" })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await expect(editProjects.getByRole("button", { name: "Search" })).toHaveAttribute(
    "aria-pressed",
    "false",
  );
}

// Edits an unrelated field of a watcher whose project filter was set through
// the API and checks that saving from the dialog keeps projectIds.
export async function assertWatcherEditKeepsProjectIds(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
) {
  await seedLinearWithProjects(apiClient, seedData);
  const watch = await apiClient.createLinearIssueWatch({
    workspaceId: seedData.workspaceId,
    workflowId: seedData.workflowId,
    workflowStepId: seedData.startStepId,
    agentProfileId: seedData.agentProfileId,
    filter: { teamKey: "ENG", projectIds: ["project-search"] },
    prompt: "Work on {{issue.identifier}}",
    enabled: false,
  });

  const settings = new LinearSettingsPage(testPage);
  await settings.goto();
  await testPage.getByText("project:1 selected").first().click();
  const dialog = testPage.getByRole("dialog");
  await expect(dialog.getByText("Edit Linear Watcher")).toBeVisible();
  await expect(
    dialog.getByTestId("linear-watch-projects").getByRole("button", { name: "Search" }),
  ).toHaveAttribute("aria-pressed", "true");

  await dialog.getByRole("button", { name: "Update" }).click();
  await expect(dialog).toBeHidden();
  await expect
    .poll(async () => (await watchFilter(apiClient, seedData, watch.id))?.projectIds)
    .toEqual(["project-search"]);
}
