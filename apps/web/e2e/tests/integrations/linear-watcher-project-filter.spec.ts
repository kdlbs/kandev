import { test } from "../../fixtures/test-base";
import {
  assertProjectOnlyWatchIsRemovable,
  assertWatcherEditKeepsProjectIds,
  assertWatcherProjectSelectionPersists,
} from "./watcher-project-filter-flow";

test.describe("Linear watcher project filter", () => {
  test("persists the selected project across save and reopen", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await assertWatcherProjectSelectionPersists(testPage, apiClient, seedData, prCapture);
  });

  test("keeps projectIds when the watcher is edited and saved", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await assertWatcherEditKeepsProjectIds(testPage, apiClient, seedData);
  });
  test("removes a saved project filter without selecting a team", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await assertProjectOnlyWatchIsRemovable(testPage, apiClient, seedData);
  });
});
