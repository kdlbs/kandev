import { test } from "../../fixtures/test-base";
import { expectTouchControl } from "../../helpers/control-sizing";
import { LayoutSettingsPage } from "../../pages/layout-settings-page";

test("keeps layout actions and repository secret fields touch-sized on mobile", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const secret = await apiClient.createSecret(
    "Mobile control sizing repository secret",
    "mobile-control-sizing-value",
  );
  await apiClient.updateRepository(seedData.repositoryId, {
    secret_bindings: [{ key: "MOBILE_CONTROL_SIZING_TOKEN", secret_id: secret.id }],
  });

  try {
    const layouts = new LayoutSettingsPage(testPage);
    await layouts.openFromSettingsIndex();
    await expectTouchControl(testPage.getByTestId("layout-profile-create"));
    await expectTouchControl(testPage.getByTestId("layout-profile-duplicate"));

    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/repositories`);
    await testPage.getByRole("heading", { name: "E2E Repo", exact: true }).click();
    const editor = testPage.getByTestId("repository-secret-bindings");
    await expectTouchControl(editor.getByTestId("repository-secret-key-0"));
    await expectTouchControl(editor.getByTestId("repository-secret-select-0"));
  } finally {
    await apiClient.updateRepository(seedData.repositoryId, { secret_bindings: [] });
    await apiClient.deleteSecretIfPresent(secret.id);
  }
});
