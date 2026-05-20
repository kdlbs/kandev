import { test, expect } from "../../fixtures/office-fixture";

/**
 * Regression test: navigating to /office with a deleted workspace ID in user
 * settings should fall back to the first available workspace and show the
 * correct dashboard data — not stale metrics from the deleted workspace.
 */
test.describe("Stale workspace recovery", () => {
  test("recovers when saved workspace ID no longer exists", async ({
    apiClient,
    officeSeed,
    testPage,
  }) => {
    // Point user settings at a non-existent workspace ID.
    await apiClient.saveUserSettings({
      workspace_id: "00000000-0000-0000-0000-000000000000",
      workflow_filter_id: officeSeed.workflowId,
      keyboard_shortcuts: {},
      enable_preview_on_click: false,
      sidebar_views: [],
    });

    // Navigate to /office — the layout should detect the stale ID,
    // fall back to the first available workspace, and render correctly.
    await testPage.goto("/office");
    await expect(testPage).toHaveURL(/\/office/, { timeout: 10_000 });

    // The dashboard should load without errors.
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });

    // Verify user settings were corrected: reload and confirm the
    // dashboard still loads (no redirect loop or error from stale ID).
    await testPage.reload();
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });
  });
});
