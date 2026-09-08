import type { Locator, Page } from "@playwright/test";
import { test, expect } from "../fixtures/test-base";
import type { ApiClient } from "../helpers/api-client";
import { watchWs } from "../helpers/causal-waits";

/**
 * The Automations section of the app sidebar.
 *
 * Automations are a background concern — they run whether or not anyone is
 * looking — so the section is folded until asked for, and must say how many it
 * is hiding while shut or it reads as empty and nobody opens it. Opened, each
 * row carries the one fact a recurring thing owes its reader: when it last
 * happened.
 *
 * Every test here seeds real automations. A section with nothing in it renders
 * no rows, no count and no times, which would satisfy most of the assertions
 * below while the feature was entirely broken.
 */

type Seed = { workspaceId: string; workflowId: string; startStepId: string };

/** The section header button — the one that toggles the accordion. */
function automationsHeader(page: Page): Locator {
  // Located by role rather than testid because the accessible name carries the
  // collapsed count ("Automations 2"), which is itself part of the contract.
  // The chevron beside it is aria-hidden, so it is not a second match.
  return page.getByTestId("app-sidebar").getByRole("button", { name: /^automations/i });
}

async function seedAutomations(apiClient: ApiClient, seed: Seed, names: string[]) {
  const created = [];
  for (const name of names) {
    created.push(
      await apiClient.seedAutomation({
        workspaceId: seed.workspaceId,
        name,
        workflowId: seed.workflowId,
        workflowStepId: seed.startStepId,
      }),
    );
  }
  return created;
}

test.describe("Automations section in the app sidebar", () => {
  test("starts folded and says how many automations it is hiding", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const [first, second] = await seedAutomations(apiClient, seedData, [
      "Nightly drift",
      "Dependency audit",
    ]);

    await testPage.goto("/");
    const header = automationsHeader(testPage);
    await expect(header).toBeVisible({ timeout: 15_000 });

    // Folded: the accordion reports itself shut and no row is mounted.
    await expect(header).toHaveAttribute("aria-expanded", "false");
    await expect(testPage.getByTestId(`sidebar-automation-${first.id}`)).toHaveCount(0);
    await expect(testPage.getByTestId(`sidebar-automation-${second.id}`)).toHaveCount(0);

    // …but it still says how much is behind it, or it reads as an empty section.
    await expect(testPage.getByTestId("sidebar-section-collapsed-summary")).toHaveText("2", {
      timeout: 15_000,
    });
  });

  test("opens to the workspace's automations and drops the count", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const [first, second] = await seedAutomations(apiClient, seedData, [
      "Nightly drift",
      "Dependency audit",
    ]);

    await testPage.goto("/");
    const header = automationsHeader(testPage);
    await expect(testPage.getByTestId("sidebar-section-collapsed-summary")).toBeVisible({
      timeout: 15_000,
    });

    await header.click();
    await expect(header).toHaveAttribute("aria-expanded", "true");

    const firstRow = testPage.getByTestId(`sidebar-automation-${first.id}`);
    await expect(firstRow).toBeVisible({ timeout: 15_000 });
    await expect(firstRow).toHaveAttribute("href", `/automations/${first.id}`);
    await expect(testPage.getByTestId(`sidebar-automation-${second.id}`)).toBeVisible();

    // Once the rows are on screen they speak for themselves; the count is noise.
    await expect(testPage.getByTestId("sidebar-section-collapsed-summary")).toHaveCount(0);
  });

  test("shows how long ago each automation last ran", async ({ testPage, apiClient, seedData }) => {
    const [ran, neverRan] = await seedAutomations(apiClient, seedData, [
      "Nightly drift",
      "Dependency audit",
    ]);
    // A real run, so the age comes from a row the server reported rather than
    // from a placeholder the row could render for anything.
    const task = await apiClient.createTask(seedData.workspaceId, "Nightly drift run", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.seedAutomationRun(ran.id, "succeeded", task.id);

    await testPage.goto("/");
    await automationsHeader(testPage).click();

    const age = testPage.getByTestId(`sidebar-automation-last-run-${ran.id}`);
    await expect(age).toBeVisible({ timeout: 15_000 });
    // The runs rail's own phrasing, so the two surfaces cannot describe the
    // same age differently. Just-seeded, so it is seconds or minutes old.
    await expect(age).toHaveText(/^(just now|\d+[smh] ago)$/);

    // An automation that has never run says nothing rather than "never".
    await expect(testPage.getByTestId(`sidebar-automation-${neverRan.id}`)).toBeVisible();
    await expect(testPage.getByTestId(`sidebar-automation-last-run-${neverRan.id}`)).toHaveCount(0);
  });

  test("refreshes an open row when a run starts without navigation", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const [automation] = await seedAutomations(apiClient, seedData, ["Nightly drift"]);
    const ws = watchWs(testPage);

    await testPage.goto("/");
    const header = automationsHeader(testPage);
    await expect(header).toBeVisible({ timeout: 15_000 });

    const initialSummary = ws.waitForResponse("automation.summaries");
    await header.click();
    await initialSummary;

    const row = testPage.getByTestId(`sidebar-automation-${automation.id}`);
    await expect(row).toBeVisible();
    await expect(row.getByTestId(`sidebar-automation-running-${automation.id}`)).toHaveCount(0);
    const urlBeforeRun = testPage.url();

    const liveSummary = ws.waitForResponse("automation.summaries");
    await apiClient.seedAutomationRun(automation.id, "triggered");
    await liveSummary;

    const runningIndicator = row.getByTestId(`sidebar-automation-running-${automation.id}`);
    await expect(runningIndicator).toBeVisible();
    await expect(runningIndicator).toHaveClass(/animate-spin/);
    await expect(runningIndicator).toHaveAttribute("aria-hidden", "true");
    await expect(row.getByText("Running.")).toBeAttached();
    await expect(testPage).toHaveURL(urlBeforeRun);
    await prCapture.screenshot("automation-sidebar-running", {
      caption: "Expanded desktop Automations sidebar with a running automation indicator",
    });
  });
});
