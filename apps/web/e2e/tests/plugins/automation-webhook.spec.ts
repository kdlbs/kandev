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
    await selectPluginCondition(testPage, seedData, apiClient);
    await exerciseSavedWebhook(testPage, seedData, apiClient);
  } finally {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`);
  }
});
