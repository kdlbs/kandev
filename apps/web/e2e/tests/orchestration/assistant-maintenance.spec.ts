import { test } from "../../fixtures/test-base";
import { ASSISTANT_ENV } from "../../helpers/personal-assistant";
import { exerciseExampleMaintenance } from "../../helpers/assistant-maintenance";
test("human reviews a synthetic improvement without starting unauthorized repair", async ({
  testPage,
  backend,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180000);
  await backend.restart(ASSISTANT_ENV);
  await exerciseExampleMaintenance(testPage, backend, apiClient, seedData);
  await testPage.screenshot({ path: test.info().outputPath("assistant-maintenance-review.png") });
});
