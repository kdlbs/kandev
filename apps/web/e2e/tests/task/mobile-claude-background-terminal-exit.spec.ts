import fs from "node:fs";
import path from "node:path";
import { test } from "../../fixtures/test-base";
import { expectClaudeBackgroundTerminalExit } from "./claude-background-terminal-exit.helpers";

let terminalSignalFile = "";

test.describe("Claude background terminal exit (mobile)", () => {
  test.describe.configure({ retries: 0 });

  test.beforeAll(async ({ backend }) => {
    terminalSignalFile = path.join(backend.tmpDir, `claude-terminal-${Date.now()}.signal`);
    fs.rmSync(terminalSignalFile, { force: true });
    await backend.restart({
      KANDEV_MOCK_PROVIDERS: "claude-acp",
      E2E_MOCK_AGENT_CLAUDE_BRIDGE_VERSION: "0.97.0",
      E2E_MOCK_AGENT_CLAUDE_TERMINAL_SIGNAL_FILE: terminalSignalFile,
    });
  });

  test.afterAll(async ({ backend }) => {
    fs.rmSync(terminalSignalFile, { force: true });
    await backend.restart();
  });

  test("a null terminal exit clears only its matching background job", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await expectClaudeBackgroundTerminalExit(testPage, apiClient, seedData, terminalSignalFile);
  });
});
