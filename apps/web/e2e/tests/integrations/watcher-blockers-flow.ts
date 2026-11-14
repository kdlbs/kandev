import { type Locator, type Page, expect } from "@playwright/test";
import { LinearSettingsPage } from "../../pages/linear-settings-page";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";

function comboboxByLabel(root: Locator, label: string): Locator {
  return root.getByText(label, { exact: true }).locator("xpath=..").getByRole("combobox");
}

// Watches survive between tests, so each flow starts from an empty watch list
// and the "blockers:done" row it opens is the one it created.
async function seedLinear(apiClient: ApiClient, seedData: SeedData) {
  await apiClient.deleteLinearIssueWatches(seedData.workspaceId);
  await apiClient.mockLinearReset();
  await apiClient.mockLinearSetTeams([{ id: "team-1", key: "ENG", name: "Engineering" }]);
  await apiClient.setLinearConfig({ secret: "lin_api_xxx" });
  await apiClient.waitForIntegrationAuthHealthy("linear");
}

// Turns the setting on before picking a team, creates the watcher, and checks
// that the team pick kept it and that reopening the watcher shows it on.
export async function assertRequireBlockersDonePersists(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  prCapture: PrAssetCapture,
) {
  await seedLinear(apiClient, seedData);
  await new LinearSettingsPage(testPage).goto();

  await testPage.getByRole("button", { name: /new watcher/i }).click();
  const dialog = testPage.getByRole("dialog");
  await expect(dialog).toBeVisible();
  const pick = async (label: string, option: string | RegExp) => {
    await comboboxByLabel(dialog, label).click();
    await testPage.getByRole("listbox").getByRole("option", { name: option }).click();
  };
  await pick("Workspace", "E2E Workspace");
  const toggle = dialog.getByTestId("linear-watch-require-blockers-done");
  await expectTouchTargetOnCoarsePointer(testPage, toggle);
  await toggle.click();
  await expect(toggle).toHaveAttribute("aria-checked", "true");
  await pick("Team", /ENG/);
  await expect(toggle).toHaveAttribute("aria-checked", "true");
  await pick("Workflow", "E2E Workflow");
  await comboboxByLabel(dialog, "Workflow Step").click();
  await testPage.getByRole("listbox").getByRole("option").first().click();

  await toggle.scrollIntoViewIfNeeded();
  await prCapture.screenshot("linear-watch-require-blockers-done", {
    caption: "Blocker setting in the Linear watcher dialog",
  });
  await dialog.getByRole("button", { name: "Create" }).click();
  await expect(dialog).toBeHidden();

  await testPage
    .getByText(/blockers:done/)
    .first()
    .click();
  const editDialog = testPage.getByRole("dialog");
  await expect(editDialog.getByText("Edit Linear Watcher")).toBeVisible();
  await expect(editDialog.getByTestId("linear-watch-require-blockers-done")).toHaveAttribute(
    "aria-checked",
    "true",
  );
}

// On phones the switch keeps its compact look but its hit area must reach
// 44px: points 22px above and below its center (44px in all) still hit the switch.
async function expectTouchTargetOnCoarsePointer(page: Page, toggle: Locator) {
  const coarse = await page.evaluate(() => window.matchMedia("(pointer: coarse)").matches);
  if (page.viewportSize()!.width < 768) expect(coarse).toBe(true);
  if (!coarse) return;
  await toggle.scrollIntoViewIfNeeded();
  const hits = await toggle.evaluate((element) => {
    const box = element.getBoundingClientRect();
    const x = box.x + box.width / 2;
    const y = box.y + box.height / 2;
    return [y - 22, y + 22].map((py) => element.contains(document.elementFromPoint(x, py)));
  });
  expect(hits).toEqual([true, true]);
}

async function taskTitles(apiClient: ApiClient, seedData: SeedData): Promise<string[]> {
  const { tasks } = await apiClient.listTasks(seedData.workspaceId);
  return tasks.map((task) => task.title);
}

// A blocker-aware watcher creates a task for the unblocked issue only, then
// creates one for the held issue on the first poll after its blocker completes.
export async function assertBlockedIssueWaitsForBlocker(apiClient: ApiClient, seedData: SeedData) {
  await seedLinear(apiClient, seedData);
  const run = Math.floor(Math.random() * 1e6);
  const blocker = `ENG-${run}1`;
  const held = `ENG-${run}2`;
  const addHeld = (stateType: string) =>
    apiClient.mockLinearAddIssues([
      {
        id: `issue-${held}`,
        identifier: held,
        title: "Held issue",
        teamKey: "ENG",
        blockers: [{ identifier: blocker, stateType }],
      },
    ]);
  await apiClient.mockLinearAddIssues([
    { id: `issue-${blocker}`, identifier: blocker, title: "Blocker issue", teamKey: "ENG" },
  ]);
  await addHeld("started");
  const watch = await apiClient.createLinearIssueWatch({
    workspaceId: seedData.workspaceId,
    workflowId: seedData.workflowId,
    workflowStepId: seedData.startStepId,
    agentProfileId: seedData.agentProfileId,
    filter: { teamKey: "ENG", requireBlockersDone: true },
    prompt: "Work on {{issue.identifier}}",
    enabled: false,
  });

  expect((await apiClient.triggerLinearIssueWatch(seedData.workspaceId, watch.id)).newIssues).toBe(
    1,
  );
  await expect
    .poll(() => taskTitles(apiClient, seedData))
    .toContainEqual(expect.stringContaining(`[${blocker}]`));
  expect(await taskTitles(apiClient, seedData)).not.toContainEqual(
    expect.stringContaining(`[${held}]`),
  );

  await addHeld("completed");
  expect((await apiClient.triggerLinearIssueWatch(seedData.workspaceId, watch.id)).newIssues).toBe(
    1,
  );
  await expect
    .poll(() => taskTitles(apiClient, seedData))
    .toContainEqual(expect.stringContaining(`[${held}]`));
}
