import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { selectPluginCondition, exerciseSavedWebhook } from "./automation-webhook-scenario";
import { PLUGIN_ID } from "../../helpers/plugin-fixture";

for (const width of [393, 700]) {
  test(`plugin webhook condition uses the native editor at ${width}px`, async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    test.setTimeout(90_000);
    await testPage.setViewportSize({ width, height: 851 });
    expect(await testPage.evaluate(() => navigator.maxTouchPoints)).toBeGreaterThan(0);
    try {
      await selectPluginCondition(testPage, seedData);
      await exerciseSavedWebhook(testPage, seedData, apiClient);
    } finally {
      const cleanup = await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`);
      expect(cleanup.ok, `Fixture deletion returned ${cleanup.status}`).toBe(true);
    }
  });
}
