import fs from "node:fs";
import path from "node:path";
import { test } from "../../fixtures/test-base";
import { expectClaudeBackgroundShellOutput } from "./claude-background-terminal-exit.helpers";

let terminalSignalFile = "";

test.describe("Claude background shell output (desktop)", () => {
  test.describe.configure({ retries: 0 });

  test.beforeAll(async ({ backend }) => {
    terminalSignalFile = path.join(backend.tmpDir, `claude-shell-output-${Date.now()}.signal`);
    fs.rmSync(terminalSignalFile, { force: true });
    await backend.restart({
      KANDEV_MOCK_PROVIDERS: "claude-acp",
      E2E_MOCK_AGENT_CLAUDE_BRIDGE_VERSION: "0.109.0",
      E2E_MOCK_AGENT_CLAUDE_TERMINAL_SIGNAL_FILE: terminalSignalFile,
    });
  });

  test.afterAll(async ({ backend }) => {
    fs.rmSync(terminalSignalFile, { force: true });
    await backend.restart();
  });

  test("streams output while running and retains it after exit", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await expectClaudeBackgroundShellOutput(testPage, apiClient, seedData, terminalSignalFile);
  });
});
