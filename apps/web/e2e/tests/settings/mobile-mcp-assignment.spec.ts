import {
  checkAssignmentWorkspaceProfile,
  checkAssignmentProfileHandoff,
  checkAssignmentSessionHandoff,
} from "../../helpers/mcp-assignment-profile-session";
import { test } from "../../fixtures/test-base";
import {
  checkAssignmentRepositoryHandoff,
  checkAssignmentRecovery,
} from "../../helpers/mcp-assignment-handoff";
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
    await checkSavedRegistration(testPage, apiClient, seedData, { source, phone: true, info });
  });
}
test("uncertain registration reads the catalog without another create", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await checkUncertainRegistration(testPage, apiClient, seedData, true);
});
test("Use in opens repository settings without selecting or saving", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await checkAssignmentRepositoryHandoff(testPage, apiClient, seedData, true);
});

test("Use in opens a workspace-scoped global profile without saving", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await checkAssignmentProfileHandoff(testPage, apiClient, seedData);
});

test("Use in opens only the existing session and waits for Customize", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await checkAssignmentSessionHandoff(testPage, apiClient, seedData);
});

test("Use in recovers read failure and rejects a disabled saved server", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await checkAssignmentRecovery(testPage, apiClient, seedData);
});

test("Use in discovers and saves an authorized workspace-owned profile", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await checkAssignmentWorkspaceProfile(testPage, apiClient, seedData);
});
