import { test } from "../../fixtures/test-base";
import { checkTaskMCPApplication } from "../../helpers/task-mcp-application";
// @covers AC-AGENTS-MCP-APPLY-003.4, AC-AGENTS-MCP-APPLY-003.5
test("shows saved MCP application, explicit Retry, and the current catalog", async ({
  testPage,
  apiClient,
  seedData,
}, info) => {
  test.setTimeout(120_000);
  await checkTaskMCPApplication(testPage, apiClient, seedData, false, info);
});
