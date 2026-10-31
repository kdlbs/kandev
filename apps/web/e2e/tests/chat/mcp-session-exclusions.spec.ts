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
// @covers AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.1, AC-AGENTS-SESSION-MCP-EXCLUSIONS-002.1, AC-AGENTS-SESSION-MCP-EXCLUSIONS-003.1
test("excludes and restores inherited MCP for one session", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await checkMCPSessionExclusions(testPage, apiClient, seedData, false);
});

test.describe("narrow fine-pointer", () => {
  test.use({ viewport: { width: 320, height: 740 } });
  test("keeps exclusion controls reachable", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(120_000);
    await checkMCPSessionExclusions(testPage, apiClient, seedData, false);
  });
});
test.describe("coarse-pointer tablet", () => {
  test.use({ viewport: { width: 820, height: 1180 }, hasTouch: true });
  test("keeps exclusion controls touch sized", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(120_000);
    await checkMCPSessionExclusions(testPage, apiClient, seedData, true);
  });
});
