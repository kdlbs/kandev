import { expect, test } from "../../fixtures/test-base";
import { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

test.describe("Mobile coordinator workspace-agent fixture", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
  });

  test("opens Coordinator from mobile Integrations without overflow", async ({
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

    const kanban = new MobileKanbanPage(testPage);
    await kanban.goto();
    await kanban.mobileMenuButton.click();
    const sheet = testPage.getByRole("dialog");
    const coordinator = sheet.getByTestId("plugin-nav-item-e2e-coordinator");
    await expect(coordinator).toBeVisible({ timeout: 15_000 });
    expect((await coordinator.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await coordinator.click();

    await expect(testPage).toHaveURL(/\/coordinator$/);
    await expect(testPage.getByTestId("fixture-coordinator-page")).toBeVisible();
    await expect(testPage.getByTestId("workspace-agent-chat")).toBeVisible({ timeout: 20_000 });
    const composer = await testPage.getByTestId("chat-input-area").boundingBox();
    const viewport = testPage.viewportSize();
    expect(composer).not.toBeNull();
    expect(viewport).not.toBeNull();
    expect(composer!.y + composer!.height).toBeGreaterThanOrEqual(viewport!.height - 24);
    expect(composer!.y + composer!.height).toBeLessThanOrEqual(viewport!.height);
    await capture.screenshot("mobile-coordinator-chat", {
      caption:
        "Phone Coordinator route opened from the mobile Integrations menu with no compressed desktop pane",
    });
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
      await testPage.evaluate(() => document.documentElement.clientWidth),
    );
    capture.flush();
  });
});
