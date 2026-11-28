import { test, expect } from "../fixtures/test-base";

/**
 * A run recorded while nothing was open must appear without a reload. The run
 * pages only poll while a run is open, so these runs arrive through the
 * workspace-scoped automation.run.created notification.
 */
test.describe("automations — runs appear without a reload", () => {
  test("the Automations page shows a newly recorded run", async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    const name = "Live feed automation";
    const automation = await apiClient.seedAutomation({ workspaceId: seedData.workspaceId, name });
    await testPage.goto("/automations");
    const recent = testPage.getByTestId("recent-runs");
    await expect(recent).toContainText("Nothing has run yet.", { timeout: 15_000 });

    await apiClient.seedAutomationRun(automation.id, "skipped");

    await expect(recent).toContainText(name, { timeout: 5_000 });
    await expect(recent).not.toContainText("Nothing has run yet.");
  });

  test("an automation's history page shows a newly recorded run", async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Live history automation",
    });
    await testPage.goto(`/automations/${automation.id}`);
    await expect(testPage.getByText("No runs yet").first()).toBeVisible({ timeout: 15_000 });

    const run = await apiClient.seedAutomationRun(automation.id, "skipped");

    await expect(testPage.getByTestId(`run-row-${run.id}`).first()).toBeVisible({ timeout: 5_000 });
  });
});
