import { test } from "../../fixtures/test-base";
import { exerciseLinearRefresh } from "./linear-settings-refresh-helpers";

test("desktop keeps accepted Linear connection after an older read", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}, testInfo) => {
  await exerciseLinearRefresh({
    page: testPage,
    api: apiClient,
    seedRepository: seedData.repositoryPath,
    runtimeDirectory: backend.tmpDir,
    testInfo,
    touch: false,
  });
});
