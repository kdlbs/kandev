import { test } from "../../fixtures/test-base";
import { checkMCPSessionExclusions } from "../../helpers/mcp-session-exclusions";
let releaseBackendEnv: (() => Promise<void>) | undefined;
test.beforeEach(async ({ backend }) => {
  releaseBackendEnv = await backend.useEnv({ KANDEV_E2E_CANCEL_HOLD_DURATION: "0s" });
});
test.afterEach(async () => {
  await releaseBackendEnv?.();
  releaseBackendEnv = undefined;
});
// @covers AC-AGENTS-SESSION-MCP-EXCLUSIONS-003.3, AC-AGENTS-SESSION-MCP-EXCLUSIONS-003.4
test("excludes and restores inherited MCP with touch controls", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await checkMCPSessionExclusions(testPage, apiClient, seedData, true);
});
