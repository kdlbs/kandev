import { test } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { checkTaskMCPRecovery } from "../../helpers/mcp-task-edit-recovery";

useRegularMode();

test("task edit reloads accepted MCP choices before an explicit save retry", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await checkTaskMCPRecovery(testPage, apiClient, seedData, false);
});
