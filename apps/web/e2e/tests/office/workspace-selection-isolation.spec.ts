/**
 * Regression: completing the office setup wizard must NOT overwrite the shared
 * userSettings.workspace_id that kanban uses to remember the active workspace.
 */
import { test, expect } from "../../fixtures/office-fixture";

test.describe("Workspace selection isolation", () => {
  test("wizard completion does not overwrite kanban workspace in user settings", async ({
    testPage,
    apiClient,
    seedData,
    officeSeed: _,
  }) => {
    test.setTimeout(90_000);

    // Point user settings at the kanban workspace (not the office one).
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: seedData.workflowId,
      keyboard_shortcuts: {},
      enable_preview_on_click: false,
      sidebar_views: [],
    });

    // Navigate to the "add workspace" wizard.
    await testPage.goto("/office/setup?mode=new");
    await expect(
      testPage.getByRole("heading", { name: "Set up your Office workspace" }),
    ).toBeVisible({ timeout: 10_000 });

    // Step 1: fill in workspace name.
    const suffix = Date.now().toString(36);
    await testPage.getByLabel("Workspace name").fill(`Isolation Test ${suffix}`);
    await testPage.getByRole("button", { name: /next/i }).click();

    // Step 2: agent config - advance without changing profile selection.
    await expect(
      testPage.getByRole("heading", { name: "Create your coordinator agent" }),
    ).toBeVisible({ timeout: 10_000 });
    await testPage.getByRole("button", { name: /next/i }).click();

    // Step 3: task - skip.
    await expect(
      testPage.getByRole("heading", { name: "Give your CEO something to do" }),
    ).toBeVisible({ timeout: 10_000 });
    await testPage.getByRole("button", { name: /skip/i }).click();

    // Step 4: review and submit.
    await expect(testPage.getByRole("heading", { name: "Review and launch" })).toBeVisible({
      timeout: 10_000,
    });
    await expect(testPage.getByText(`Isolation Test ${suffix}`)).toBeVisible();

    const responsePromise = testPage.waitForResponse(
      (resp) => resp.url().includes("/onboarding/complete") && resp.status() === 201,
      { timeout: 15_000 },
    );
    await testPage.locator("button:has-text('Create & Launch')").click();
    await responsePromise;

    // Wizard redirects to /office (or /office?workspaceId=...) after completion.
    await expect(testPage).toHaveURL(/\/office(\?|$)/, { timeout: 15_000 });

    // The critical assertion: user settings workspace_id must still point to
    // the kanban workspace - not the newly created office workspace.
    const settings = await apiClient.getUserSettings();
    expect(settings.settings.workspace_id).toBe(seedData.workspaceId);
  });

  test("office page with workspaceId URL param shows correct workspace without writing settings", async ({
    testPage,
    apiClient,
    seedData,
    officeSeed,
  }) => {
    // Point user settings at kanban workspace.
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: seedData.workflowId,
      keyboard_shortcuts: {},
      enable_preview_on_click: false,
      sidebar_views: [],
    });

    // Navigate to /office with an explicit workspaceId param.
    await testPage.goto(`/office?workspaceId=${officeSeed.workspaceId}`);
    await expect(testPage).toHaveURL(/\/office(\?|$)/, { timeout: 10_000 });
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });

    // User settings workspace_id must remain the kanban workspace.
    const settings = await apiClient.getUserSettings();
    expect(settings.settings.workspace_id).toBe(seedData.workspaceId);
  });
});
