import { test, expect } from "../../fixtures/test-base";
import { LinearSettingsPage } from "../../pages/linear-settings-page";

test.describe("Linear settings on mobile", () => {
  test("workspace-scoped route scopes the credentials form", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const target = await apiClient.createWorkspace("Mobile Linear Workspace");

    const settings = new LinearSettingsPage(testPage);
    await settings.gotoWorkspace(seedData.workspaceId);

    await settings.secretInput.fill("lin_api_mobile");
    await settings.saveButton.click();
    await expect(testPage.getByText(/leave blank to keep the current value/i)).toBeVisible();

    const targetResponse = await apiClient.rawRequest(
      "GET",
      `/api/v1/linear/config?workspace_id=${target.id}`,
    );
    const targetConfig =
      targetResponse.status === 204
        ? null
        : ((await targetResponse.json()) as { hasSecret?: boolean });
    expect(targetConfig?.hasSecret ?? false).toBe(false);

    await settings.gotoWorkspace(target.id);

    await expect(settings.secretInput).toHaveValue("");
    await expect(settings.saveButton).toHaveCount(0);
    await expect(testPage.getByText(/leave blank to keep the current value/i)).toHaveCount(0);
  });
});
