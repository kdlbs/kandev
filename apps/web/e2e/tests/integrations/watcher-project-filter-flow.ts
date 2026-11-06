import { type Locator, type Page, expect } from "@playwright/test";
import { dwell, waitForHttp } from "../../helpers/causal-waits";
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
  const projectsLoaded = waitForHttp(testPage, "GET", /^\/api\/v1\/linear\/projects$/);
  await pick("Team", /ENG/);
  expect((await projectsLoaded).ok()).toBe(true);
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

// A valid project-only watch must expose its saved IDs without fetching an
// unscoped project inventory or forcing a team change that clears the filter.
export async function assertProjectOnlyWatchIsRemovable(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
) {
  await seedLinearWithProjects(apiClient, seedData);
  const projectIds = ["project-archived", "project-search"];
  const watch = await apiClient.createLinearIssueWatch({
    workspaceId: seedData.workspaceId,
    workflowId: seedData.workflowId,
    workflowStepId: seedData.startStepId,
    agentProfileId: seedData.agentProfileId,
    filter: { teamKey: "", projectIds },
    prompt: "Work on {{issue.identifier}}",
    enabled: false,
  });
  const savedFilter = await watchFilter(apiClient, seedData, watch.id);
  expect(savedFilter?.teamKey ?? "").toBe("");
  expect(savedFilter?.projectIds).toEqual(projectIds);
  const projectRequests: string[] = [];
  testPage.on("request", (request) => {
    if (new URL(request.url()).pathname === "/api/v1/linear/projects") {
      projectRequests.push(request.url());
    }
  });
  await new LinearSettingsPage(testPage).goto();
  await testPage.getByText("project:2 selected", { exact: true }).click();
  const dialog = testPage.getByRole("dialog");
  const chip = dialog.getByTitle("project-archived", { exact: true });
  await expect(chip).toHaveAttribute("aria-pressed", "true");
  await chip.scrollIntoViewIfNeeded();
  const box = await chip.boundingBox();
  expect(box).not.toBeNull();
  if (testPage.viewportSize()!.width < 768) {
    expect(box!.height).toBeGreaterThanOrEqual(44);
    expect(box!.width).toBeGreaterThanOrEqual(44);
  }
  expect(
    await chip.evaluate((element) => {
      const bounds = element.getBoundingClientRect();
      return element.contains(
        document.elementFromPoint(bounds.x + bounds.width / 2, bounds.y + bounds.height / 2),
      );
    }),
  ).toBe(true);
  await chip.click();
  await expect(chip).toHaveCount(0);
  await expect(dialog.getByTitle("project-search", { exact: true })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await dialog.getByRole("button", { name: "Update", exact: true }).click();
  await expect(dialog).toBeHidden();
  await testPage.reload();
  await testPage.getByText("project:1 selected", { exact: true }).click();
  await expect(dialog.getByTitle("project-search", { exact: true })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await dwell(
    testPage,
    300,
    "negative-assertion",
    "No inventory-complete event exists when a project-only watch has no team to request",
  );
  expect(projectRequests).toEqual([]);
  const filter = await watchFilter(apiClient, seedData, watch.id);
  expect(filter?.projectIds).toEqual(["project-search"]);
  expect(filter?.teamKey ?? "").toBe("");
}
