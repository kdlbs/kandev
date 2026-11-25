import { test } from "../../fixtures/test-base";
import {
  assertBlockedIssueWaitsForBlocker,
  assertRequireBlockersDonePersists,
} from "./watcher-blockers-flow";

test.describe("Linear watcher blocker setting", () => {
  test("keeps the setting across team pick, save, and reopen", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await assertRequireBlockersDonePersists(testPage, apiClient, seedData, prCapture);
  });

  test("starts a held issue only after its blocker completes", async ({ apiClient, seedData }) => {
    await assertBlockedIssueWaitsForBlocker(apiClient, seedData);
  });
});
