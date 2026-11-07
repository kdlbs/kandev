import { test } from "../../fixtures/test-base";
import { verifyStartupHousekeeping } from "../../helpers/startup-housekeeping";

test("completes housekeeping before restoring two conversations with retained history and an unavailable remote", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(180000);
  await verifyStartupHousekeeping(testPage, apiClient, seedData, backend);
});
