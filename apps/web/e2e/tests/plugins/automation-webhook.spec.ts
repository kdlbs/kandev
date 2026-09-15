import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { selectPluginCondition, exerciseSavedWebhook } from "./automation-webhook-scenario";
import { PLUGIN_ID } from "../../helpers/plugin-fixture";

test("plugin webhook condition uses the native editor", async ({
  testPage,
  seedData,
  apiClient,
}) => {
  test.setTimeout(90_000);
  try {
    await selectPluginCondition(testPage, seedData);
    await exerciseSavedWebhook(testPage, seedData, apiClient);
  } finally {
    const cleanup = await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`);
    expect(cleanup.ok, `Fixture deletion returned ${cleanup.status}`).toBe(true);
  }
});
