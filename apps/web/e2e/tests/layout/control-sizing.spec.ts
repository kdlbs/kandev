import { expect, test } from "../../fixtures/test-base";
import { expectControlHeight, expectTouchControl } from "../../helpers/control-sizing";

test.describe("shared control sizing", () => {
  test("keeps the Appearance selector at the standard desktop height", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 1280, height: 900 });
    await testPage.goto("/settings/preferences/appearance");

    const selector = testPage.getByTestId("theme-settings-card").getByRole("combobox");
    await expect(selector).toBeVisible();
    await expectControlHeight(selector, 28);
  });

  test("keeps the Appearance selector touch-sized on a coarse-pointer tablet", async ({
    tabletTestPage,
  }) => {
    await tabletTestPage.goto("/settings/preferences/appearance");

    const selector = tabletTestPage.getByTestId("theme-settings-card").getByRole("combobox");
    await expect(selector).toBeVisible();
    await expectTouchControl(selector);
  });

  test("keeps the GitHub provider credential field at the standard desktop height", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.mockGitHubSetWorkspaceConnection(seedData.workspaceId, {
      source: "legacy_shared",
      status: "active",
    });
    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/integrations/github`);
    await testPage.getByRole("button", { name: "Change connection" }).click();

    const surface = testPage.getByTestId("github-connection-desktop");
    await surface.locator("#github-method-pat").click();
    const token = surface.locator("#github-workspace-token");
    await expect(token).toBeVisible();
    await expectControlHeight(token, 28);
    await expectControlHeight(surface.getByRole("button", { name: "Show token" }), 28);
  });
});
