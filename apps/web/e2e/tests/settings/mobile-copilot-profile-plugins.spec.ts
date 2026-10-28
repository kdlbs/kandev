import { test, expect } from "../../fixtures/test-base";
import {
  copilotPluginDiscovery,
  createCopilotPluginFixture,
} from "../../helpers/copilot-plugin-fixture";

const longName =
  "installed marketplace plugin with a long translated name that should wrap on a phone";

test("phone users can select, save, reset, and clear Copilot profile plugins", async ({
  testPage,
  apiClient,
}) => {
  test.setTimeout(60_000);
  const fixture = await createCopilotPluginFixture(apiClient);
  test.skip(!fixture, "Copilot agent is not registered in this E2E runtime");
  if (!fixture) return;
  const discovery = copilotPluginDiscovery(fixture.agentId, fixture.profileId);
  discovery.plugins[0].name = longName;
  await testPage.route(`**/api/v1/agents/${fixture.agentId}/plugin-discovery`, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(discovery),
    });
  });

  try {
    await testPage.goto(`/settings/agents/${fixture.agentName}/profiles/${fixture.profileId}`);
    const selector = testPage.getByTestId("copilot-plugins-selection");
    await expect(selector).toBeVisible({ timeout: 15_000 });
    const checkbox = testPage.getByRole("checkbox", { name: `Select ${longName}` });
    const row = testPage.locator('[data-testid="copilot-plugin-row"]').first();
    const rowBox = await row.boundingBox();
    expect(rowBox).not.toBeNull();
    expect(rowBox!.height).toBeGreaterThanOrEqual(44);
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);

    await checkbox.tap();
    await expect(checkbox).toBeChecked();
    await testPage.setViewportSize({ width: 767, height: 800 });
    await expect(checkbox).toBeChecked();
    await testPage.setViewportSize({ width: 390, height: 844 });
    await expect(checkbox).toBeChecked();

    const save = testPage.getByRole("button", { name: /^Save( changes)?$/i }).first();
    await save.click();
    await expect
      .poll(async () => (await apiClient.getAgentProfile(fixture.profileId)).copilotSelectedPlugins)
      .toEqual(["installed:copilot-plugins/spark"]);

    await checkbox.tap();
    const reset = testPage.getByRole("button", { name: /^Reset$/i }).first();
    await expect(reset).toBeEnabled();
    await reset.click();
    await expect(checkbox).toBeChecked();

    await checkbox.tap();
    await save.click();
    await expect
      .poll(async () => (await apiClient.getAgentProfile(fixture.profileId)).copilotSelectedPlugins)
      .toEqual([]);
  } finally {
    await fixture.dispose();
  }
});
