import { test } from "../../fixtures/test-base";
import { uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import { verifyPluginActionMotion } from "./plugin-action-motion-helpers";

test.afterEach(async ({ apiClient }) => {
  await uninstallFixturePlugin(apiClient);
});

test("respects reduced motion while keeping plugin busy actions visible and usable", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}, info) => {
  test.setTimeout(120_000);
  await verifyPluginActionMotion(testPage, apiClient, seedData, info, prCapture);
});
