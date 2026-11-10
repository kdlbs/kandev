import { test } from "../../fixtures/test-base";
import { exerciseJiraRefresh } from "./jira-settings-refresh-helpers";

test("phone keeps saved Jira baseline after an older absent read", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}, testInfo) => {
  await exerciseJiraRefresh({
    page: testPage,
    api: apiClient,
    seedRepository: seedData.repositoryPath,
    runtimeDirectory: backend.tmpDir,
    testInfo,
    touch: true,
  });
});
