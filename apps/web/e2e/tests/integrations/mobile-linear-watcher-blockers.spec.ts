import { test } from "../../fixtures/test-base";
import { assertRequireBlockersDonePersists } from "./watcher-blockers-flow";

test.describe("Linear watcher blocker setting (mobile)", () => {
  test("keeps the setting across team pick, save, and reopen", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await assertRequireBlockersDonePersists(testPage, apiClient, seedData, prCapture);
  });
});
