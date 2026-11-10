import { test } from "../../fixtures/test-base";
import { repositorySaveRetry } from "./repository-save-retry-helpers";

// @covers AC-WORKSPACES-REPOSITORY-SAVE-001.1, AC-WORKSPACES-REPOSITORY-SAVE-001.4
for (const fresh of [false, true]) {
  test(`repository Save retries preserve acknowledged ${fresh ? "new" : "established"} resources`, async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }, testInfo) => {
    await repositorySaveRetry({ page: testPage, testInfo, apiClient, backend, seedData, fresh });
  });
}
