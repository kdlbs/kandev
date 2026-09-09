import { test } from "../../fixtures/test-base";
import { expectControlHeight } from "../../helpers/control-sizing";
import { LayoutSettingsPage } from "../../pages/layout-settings-page";

test.describe("Settings control sizing", () => {
  test("uses the standard desktop height for layout actions", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 1280, height: 900 });
    const layouts = new LayoutSettingsPage(testPage);
    await layouts.open();

    await expectControlHeight(testPage.getByTestId("layout-profile-create"), 28);
    await expectControlHeight(testPage.getByTestId("layout-profile-duplicate"), 28);
  });

  test("keeps repository secret fields at the standard desktop height", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const secret = await apiClient.createSecret(
      "Control sizing repository secret",
      "control-sizing-value",
    );
    await apiClient.updateRepository(seedData.repositoryId, {
      secret_bindings: [{ key: "CONTROL_SIZING_TOKEN", secret_id: secret.id }],
    });

    try {
      await testPage.setViewportSize({ width: 1280, height: 900 });
      await testPage.goto(`/settings/workspace/${seedData.workspaceId}/repositories`);
      await testPage.getByRole("heading", { name: "E2E Repo", exact: true }).click();

      const editor = testPage.getByTestId("repository-secret-bindings");
      await expectControlHeight(editor.getByTestId("repository-secret-key-0"), 28);
      await expectControlHeight(editor.getByTestId("repository-secret-select-0"), 28);
    } finally {
      await apiClient.updateRepository(seedData.repositoryId, { secret_bindings: [] });
      await apiClient.deleteSecretIfPresent(secret.id);
    }
  });
});
