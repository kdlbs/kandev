import { test } from "../../fixtures/test-base";
import { exerciseAutoUpdateIntent } from "./plugin-auto-update-helpers";

// @covers AC-PLUGINS-MARKETPLACE-004.1, AC-PLUGINS-MARKETPLACE-004.2, AC-PLUGINS-MARKETPLACE-004.3
for (const success of [true, false]) {
  test(`phone rapid plugin defaults preserve inherited state (success ${success})`, async ({
    testPage,
    apiClient,
  }) => {
    await exerciseAutoUpdateIntent(testPage, apiClient, true, success);
  });
}
