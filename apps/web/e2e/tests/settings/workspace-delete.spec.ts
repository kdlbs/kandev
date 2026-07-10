import { test, expect } from "../../fixtures/test-base";
import { OfficeApiClient } from "../../helpers/office-api-client";

test.describe("Workspace settings deletion", () => {
  test("deletes a workspace from the settings edit page", async ({ testPage, apiClient }) => {
    const suffix = Date.now().toString(36);
    const workspaceName = `Settings Delete ${suffix}`;
    const workspace = await apiClient.createWorkspace(workspaceName);

    await testPage.goto(`/settings/workspace/${workspace.id}`);
    await expect(testPage.getByRole("heading", { name: workspaceName })).toBeVisible({
      timeout: 15_000,
    });

    await testPage.getByTestId("workspace-settings-delete-button").click();

    const confirmInput = testPage.getByTestId("workspace-settings-delete-confirm-input");
    const confirmButton = testPage.getByTestId("workspace-settings-delete-confirm-button");
    await expect(confirmInput).toBeVisible();

    // The wrong confirmation string ("delete") must not enable deletion — the
    // backend requires confirm_name to equal the workspace name.
    await confirmInput.fill("delete");
    await expect(confirmButton).toBeDisabled();

    await confirmInput.fill(workspaceName);
    await expect(confirmButton).toBeEnabled();

    // Cancel-then-reopen must reset the confirmation field; otherwise the
    // re-type requirement would be silently bypassed on the second open.
    await testPage.getByRole("button", { name: "Cancel" }).click();
    await testPage.getByTestId("workspace-settings-delete-button").click();
    await expect(confirmInput).toHaveValue("");
    await expect(confirmButton).toBeDisabled();

    await confirmInput.fill(workspaceName);
    await expect(confirmButton).toBeEnabled();

    // Deletion runs through an action wrapper, so assert the user-visible
    // outcome: redirect to the workspace list and the workspace gone from the
    // backend.
    await confirmButton.click();
    await expect(testPage).toHaveURL(/\/settings\/workspace$/, { timeout: 10_000 });

    const { workspaces } = await apiClient.listWorkspaces();
    expect(workspaces.some((item) => item.id === workspace.id)).toBe(false);
  });

  test("deletes an office workspace from the settings edit page", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    const officeApi = new OfficeApiClient(backend.baseUrl);
    const suffix = Date.now().toString(36);
    const workspaceName = `Settings Office Delete ${suffix}`;
    const onboarded = await officeApi.completeOnboarding({
      workspaceName,
      taskPrefix: `SOD${suffix.toUpperCase().slice(0, 3)}`,
      agentName: "Settings Delete CEO",
      agentProfileId: seedData.agentProfileId,
      executorPreference: "local_pc",
    });

    await officeApi.createSkill(onboarded.workspaceId, {
      name: `Settings Cleanup Skill ${suffix}`,
      slug: `settings-cleanup-skill-${suffix}`,
      content: "# Settings cleanup skill\n",
    });

    await testPage.goto(`/settings/workspace/${onboarded.workspaceId}`);
    await expect(testPage.getByRole("heading", { name: workspaceName })).toBeVisible({
      timeout: 15_000,
    });

    await testPage.getByTestId("workspace-settings-delete-button").click();
    const confirmInput = testPage.getByTestId("workspace-settings-delete-confirm-input");
    const confirmButton = testPage.getByTestId("workspace-settings-delete-confirm-button");
    await expect(confirmInput).toBeVisible();

    await confirmInput.fill(workspaceName);
    await expect(confirmButton).toBeEnabled();
    await confirmButton.click();
    await expect(testPage).toHaveURL(/\/settings\/workspace$/, { timeout: 10_000 });

    const { workspaces } = await apiClient.listWorkspaces();
    expect(workspaces.some((item) => item.id === onboarded.workspaceId)).toBe(false);

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: seedData.workflowId,
      keyboard_shortcuts: {},
      enable_preview_on_click: false,
      sidebar_views: [],
    });
  });
});
