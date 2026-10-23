import { test } from "../../fixtures/test-base";
import {
  checkSavedRegistration,
  checkUncertainRegistration,
  cleanupAssignmentServers,
} from "../../helpers/mcp-assignment";

test.afterEach(async ({ apiClient, seedData }) => {
  await cleanupAssignmentServers(apiClient, seedData.workspaceId);
});
for (const source of ["custom", "marketplace"] as const) {
  test(`saved-only ${source} registration`, async ({ testPage, apiClient, seedData }, info) => {
    await checkSavedRegistration(testPage, apiClient, seedData, { source, phone: false, info });
  });
}
test("uncertain registration reads the catalog without another create", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await checkUncertainRegistration(testPage, apiClient, seedData, false);
});
