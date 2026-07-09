import { test, expect } from "../../fixtures/test-base";
import { OfficeApiClient } from "../../helpers/office-api-client";

test.describe("Office workspace deletion fallback navigation", () => {
  test("switches to a remaining kanban workspace instead of opening office setup", async ({
    apiClient,
    backend,
    seedData,
    testPage,
  }) => {
    const officeApi = new OfficeApiClient(backend.baseUrl);
    const suffix = Date.now().toString(36);
    const workspaceName = `Only Office Delete ${suffix}`;
    const onboarded = await officeApi.completeOnboarding({
      workspaceName,
      taskPrefix: `OOD${suffix.toUpperCase().slice(0, 3)}`,
      agentName: "Only Office CEO",
      agentProfileId: seedData.agentProfileId,
      executorPreference: "local_pc",
    });

    await apiClient.saveUserSettings({
      workspace_id: onboarded.workspaceId,
      workflow_filter_id: "",
      keyboard_shortcuts: {},
      enable_preview_on_click: false,
      sidebar_views: [],
    });

    await testPage.goto(`/office/workspace/settings?workspaceId=${onboarded.workspaceId}`);
    await expect(testPage.getByText(/danger zone/i)).toBeVisible({ timeout: 15_000 });
    await testPage.getByTestId("workspace-delete-button").click();

    const confirmInput = testPage.getByTestId("workspace-delete-confirm-input");
    const confirmButton = testPage.getByTestId("workspace-delete-confirm-button");
    await expect(confirmInput).toBeVisible();
    await confirmInput.fill(workspaceName);
    await expect(confirmButton).toBeEnabled();
    await confirmButton.click();

    await expect(testPage).toHaveURL(
      (url) => url.pathname === "/" && url.searchParams.get("workspaceId") === seedData.workspaceId,
      { timeout: 10_000 },
    );
    await expect(testPage).not.toHaveURL(/\/office\/setup/);

    const { workspaces } = await apiClient.listWorkspaces();
    expect(workspaces.some((item) => item.id === onboarded.workspaceId)).toBe(false);
  });
});
