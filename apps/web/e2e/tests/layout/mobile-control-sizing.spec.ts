import { expect, test } from "../../fixtures/test-base";
import { expectTouchControl } from "../../helpers/control-sizing";

test.describe("shared control sizing on mobile", () => {
  test("keeps the Appearance selector touch-sized at the phone boundary", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto("/settings/preferences/appearance");

    const selector = testPage.getByTestId("theme-settings-card").getByRole("combobox");
    await expect(selector).toBeVisible();
    await expectTouchControl(selector);
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
      await testPage.evaluate(() => document.documentElement.clientWidth),
    );
  });

  test("keeps the GitHub provider credential field touch-sized on a phone", async ({
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

    const surface = testPage.getByTestId("github-connection-mobile");
    await surface.locator("#github-method-pat").click();
    const token = surface.locator("#github-workspace-token");
    await expect(token).toBeVisible();
    await expectTouchControl(token);
    await expectTouchControl(surface.getByRole("button", { name: "Show token" }));
  });
});
