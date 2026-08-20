import { test, expect } from "../../fixtures/office-fixture";
import { waitForHttp } from "../../helpers/causal-waits";

test.describe("Workspace settings UI", () => {
  test("settings page shows danger zone", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office/workspace/settings");
    await expect(testPage.getByText(/danger zone/i)).toBeVisible({ timeout: 10_000 });
  });

  test("renaming the workspace persists and updates the sidebar", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const renamed = "E2E Workspace Renamed";
    let originalName: string | undefined;
    try {
      await testPage.goto("/office/workspace/settings");
      const nameInput = testPage.getByPlaceholder("Workspace name");
      await expect(nameInput).toBeVisible({ timeout: 10_000 });
      originalName = await nameInput.inputValue();
      await nameInput.fill(renamed);
      const saveButton = testPage.getByTestId("appearance-save-button");

      const workspaceSaved = waitForHttp(
        testPage,
        "PATCH",
        new RegExp(`/api/v1/workspaces/${officeSeed.workspaceId}$`),
      );
      await saveButton.click();
      await workspaceSaved;

      // Save button clears once the store reflects the persisted name.
      await expect(saveButton).toBeHidden();
      await expect(testPage.getByTestId("sidebar-workspace-trigger")).toContainText(renamed);

      await testPage.reload();
      await expect(nameInput).toHaveValue(renamed);
    } finally {
      await apiClient.updateWorkspace(officeSeed.workspaceId, {
        name: originalName ?? "E2E Workspace",
      });
    }
  });
});
