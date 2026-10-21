import { test } from "../../fixtures/test-base";
import { repositorySaveRetry } from "./repository-save-retry-helpers";

// @covers AC-WORKSPACES-REPOSITORY-SAVE-001.4
for (const fresh of [false, true]) {
  test(`phone repository Save retries preserve acknowledged ${fresh ? "new" : "established"} resources`, async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    await repositorySaveRetry({
      page: testPage,
      apiClient,
      backend,
      seedData,
      fresh,
      mobile: true,
    });
  });
}
