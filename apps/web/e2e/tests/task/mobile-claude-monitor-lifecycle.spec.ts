import fs from "node:fs";
import path from "node:path";
import { test } from "../../fixtures/test-base";
import { expectClaudeMonitorLifecycle } from "./claude-monitor-lifecycle.helpers";

let configDir = "";
let signalFile = "";

test.describe("Claude Monitor lifecycle on a phone", () => {
  test.describe.configure({ retries: 0 });

  test.beforeAll(async ({ backend }) => {
    const suffix = Date.now();
    configDir = path.join(backend.tmpDir, `claude-monitor-config-${suffix}`);
    signalFile = path.join(backend.tmpDir, `claude-monitor-${suffix}.signal`);
    fs.rmSync(configDir, { recursive: true, force: true });
    fs.rmSync(signalFile, { force: true });
    await backend.restart({
      KANDEV_MOCK_PROVIDERS: "claude-acp",
      E2E_MOCK_AGENT_CLAUDE_BRIDGE_VERSION: "0.107.0",
    });
  });

  test.afterAll(async ({ backend }) => {
    fs.rmSync(configDir, { recursive: true, force: true });
    fs.rmSync(signalFile, { force: true });
    await backend.restart();
  });

  test("keeps live Monitors visible across turns and settles matching outcomes", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await expectClaudeMonitorLifecycle(testPage, apiClient, seedData, configDir, signalFile);
  });
});
