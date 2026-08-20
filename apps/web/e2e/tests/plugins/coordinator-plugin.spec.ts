import { expect, test } from "../../fixtures/test-base";
import { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";

test.describe("Coordinator workspace-agent fixture", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
  });

  test("opens the integration destination on the managed chat surface", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    test.setTimeout(90_000);
    const capture = new PrAssetCapture(testPage, testInfo.file);

    await apiClient.updateWorkspace(seedData.workspaceId, {
      default_agent_profile_id: seedData.agentProfileId,
    });
    await installFixturePlugin(testPage);

    await testPage.goto("/");
    await testPage.getByRole("button", { name: "Integrations" }).click();
    const coordinator = testPage.getByTestId("plugin-nav-item-e2e-coordinator");
    await expect(coordinator).toBeVisible({ timeout: 15_000 });
    await coordinator.click();

    await expect(testPage).toHaveURL(/\/coordinator$/);
    await expect(testPage).not.toHaveURL(/\/t\//);
    await expect(testPage.getByTestId("fixture-coordinator-page")).toBeVisible();
    await expect(testPage.getByTestId("workspace-agent-chat")).toBeVisible({ timeout: 20_000 });
    await capture.screenshot("desktop-coordinator-chat", {
      caption:
        "Desktop Coordinator route opened from Integrations with the host-owned chat surface",
    });
    await testPage.getByTestId("fixture-coordinator-reports").click();
    await expect(testPage.getByTestId("fixture-coordinator-reports-view")).toBeVisible();
    await capture.screenshot("desktop-coordinator-reports", {
      caption: "Desktop Coordinator Reports tab rendered inside the dedicated plugin route",
    });
    capture.flush();
  });
});
