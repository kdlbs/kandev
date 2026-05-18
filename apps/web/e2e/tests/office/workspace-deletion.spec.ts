import { test, expect } from "../../fixtures/office-fixture";

test.describe("Workspace deletion", () => {
  test("deletes a workspace through the settings danger zone", async ({
    apiClient,
    officeApi,
    officeSeed,
    seedData,
    testPage,
  }) => {
    const suffix = Date.now().toString(36);
    const workspaceName = `Delete Workspace ${suffix}`;
    // Use the office onboarding endpoint so the workspace becomes a real
    // office workspace (gets office_workflow_id set + a CEO agent + the
    // "Workspace coordination" task). Without onboarding, the office
    // sidebar/layout filters the workspace out and settings can't render.
    const onboarded = await officeApi.completeOnboarding({
      workspaceName,
      taskPrefix: `DEL${suffix.toUpperCase().slice(0, 3)}`,
      agentName: "Delete CEO",
      agentProfileId: seedData.agentProfileId,
      executorPreference: "local_pc",
    });
    const workspaceId = onboarded.workspaceId;

    await officeApi.createSkill(workspaceId, {
      name: `Cleanup Skill ${suffix}`,
      slug: `cleanup-skill-${suffix}`,
      content: "# Cleanup Skill\n",
    });
    await apiClient.saveUserSettings({
      workspace_id: workspaceId,
      workflow_filter_id: "",
      keyboard_shortcuts: {},
      enable_preview_on_click: false,
      sidebar_views: [],
    });

    await testPage.goto("/office/workspace/settings");
    await expect(testPage.getByText(/danger zone/i)).toBeVisible({ timeout: 15_000 });

    const summaryResponsePromise = testPage.waitForResponse((response) =>
      response.url().includes(`/api/v1/office/workspaces/${workspaceId}/deletion-summary`),
    );
    await testPage.getByTestId("workspace-delete-button").click();
    const summaryResponse = await summaryResponsePromise;
    expect(summaryResponse.ok(), await summaryResponse.text()).toBe(true);
    await expect(testPage.getByTestId("workspace-delete-dialog")).toBeVisible();
    // Use a regex with \d+ counts since onboarding's exact seed shape may
    // change (CEO agent + Workspace coordination task + cleanup skill).
    await expect(
      testPage.getByText(/permanently delete \d+ agents, \d+ tasks, \d+ skills/),
    ).toBeVisible();

    const confirmInput = testPage.getByTestId("workspace-delete-confirm-input");
    const confirmButton = testPage.getByTestId("workspace-delete-confirm-button");
    await expect(confirmButton).toBeDisabled();
    await confirmInput.fill(workspaceName);
    await expect(confirmButton).toBeEnabled();
    await confirmButton.click();

    await expect(testPage).toHaveURL(/\/office$/, { timeout: 10_000 });
    await expect(testPage).not.toHaveURL(/\/office\/setup/);

    const { workspaces } = await apiClient.listWorkspaces();
    expect(workspaces.some((item) => item.id === workspaceId)).toBe(false);

    // Restore user_settings to the original officeSeed workspace so other
    // tests in this worker don't inherit a settings row pointing at the
    // workspace we just deleted.
    await apiClient.saveUserSettings({
      workspace_id: officeSeed.workspaceId,
      workflow_filter_id: "",
      keyboard_shortcuts: {},
      enable_preview_on_click: false,
      sidebar_views: [],
    });
  });
});
