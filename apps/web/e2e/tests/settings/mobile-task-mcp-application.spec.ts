import { test } from "../../fixtures/test-base";
import { checkTaskMCPApplication } from "../../helpers/task-mcp-application";
// @covers AC-AGENTS-MCP-APPLY-003.4, AC-AGENTS-MCP-APPLY-003.5
test("keeps MCP application and Retry usable at phone and touch widths", async ({
  testPage,
  apiClient,
  seedData,
}, info) => {
  test.setTimeout(120_000);
  await checkTaskMCPApplication(testPage, apiClient, seedData, true, info);
});
