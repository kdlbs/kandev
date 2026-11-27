import { test } from "../../fixtures/test-base";
import { expectSettingsHomeToKeepTabWorkspace } from "./workspace-tab-settings-home-helpers";

test("keeps each phone tab workspace when returning Home from Settings", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  await expectSettingsHomeToKeepTabWorkspace({
    page: testPage,
    apiClient,
    backendPort: backend.port,
    activeWorkspaceId: seedData.workspaceId,
    mobile: true,
  });
});
