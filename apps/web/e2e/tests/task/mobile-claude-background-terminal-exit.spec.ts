import { test } from "../../fixtures/test-base";
import { expectClaudeBackgroundTerminalExit } from "./claude-background-terminal-exit.helpers";

test.describe("Claude background terminal exit (mobile)", () => {
  test.describe.configure({ retries: 0 });

  test.beforeAll(async ({ backend }) => {
    await backend.restart({
      KANDEV_MOCK_PROVIDERS: "claude-acp",
      E2E_MOCK_AGENT_CLAUDE_BRIDGE_VERSION: "0.97.0",
    });
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("a null terminal exit clears only its matching background job", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await expectClaudeBackgroundTerminalExit(testPage, apiClient, seedData);
  });
});
