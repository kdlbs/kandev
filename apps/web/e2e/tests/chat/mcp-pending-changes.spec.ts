import { test } from "../../fixtures/test-base";
import { checkMCPPendingChanges } from "../../helpers/mcp-pending-changes";

let releaseBackendEnv: (() => Promise<void>) | undefined;

test.beforeEach(async ({ backend }) => {
  // Cancellation ends the busy fixture immediately for same-agent idle acceptance.
  releaseBackendEnv = await backend.useEnv({ KANDEV_E2E_CANCEL_HOLD_DURATION: "0s" });
});

test.afterEach(async () => {
  await releaseBackendEnv?.();
  releaseBackendEnv = undefined;
});

// @covers AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.15, AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.16, AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.17, AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.18
test("names saved additions, deleted removals and updates across application states", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(150_000);
  await checkMCPPendingChanges(testPage, apiClient, seedData, false);
});
