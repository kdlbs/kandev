// Routing: /t/{taskId} (task-keyed, not /s/{sessionId})
import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Create a non-TUI task and navigate to its session. Waits for agent idle. */
async function seedTaskWithSession(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
  return session;
}

/** Focus the terminal and wait for the shell to be ready (prompt visible). */
async function focusTerminal(page: Page, session: SessionPage): Promise<void> {
  await expect(session.terminal).toBeVisible({ timeout: 15_000 });
  await expect
    .poll(
      async () => {
        const buf = await readTerminalBuffer(page);
        return buf.length > 0;
      },
      { timeout: 15_000, message: "Waiting for terminal shell to connect" },
    )
    .toBe(true);

  const xterm = session.terminal.locator(".xterm");
  await xterm.click();
}

/** Read the terminal panel's xterm buffer text. */
async function readTerminalBuffer(page: Page): Promise<string> {
  return page.evaluate(() => {
    const panel = document.querySelector('[data-testid="terminal-panel"]');
    if (!panel) return "";
    const xtermEl = panel.querySelector(".xterm");
    type XC = HTMLElement & { __xtermReadBuffer?: () => string };
    const container = xtermEl?.parentElement as XC | null | undefined;
    return container?.__xtermReadBuffer?.() ?? "";
  });
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// Routing: uses /t/{taskId} (task-keyed routing) instead of /s/{sessionId}
test.describe("Terminal keyboard navigation", () => {
  // Standalone executor can fail on cold start; retry once for transient failures.
  test.describe.configure({ retries: 1 });

  test("TERM is set to xterm-256color", async ({ testPage, apiClient, seedData }) => {
    const session = await seedTaskWithSession(testPage, apiClient, seedData, "TERM Check");
    await focusTerminal(testPage, session);

    await testPage.keyboard.type("echo $TERM");
    await testPage.keyboard.press("Enter");

    await session.expectTerminalHasText("xterm-256color");
  });

  test("COLORTERM is set to truecolor", async ({ testPage, apiClient, seedData }) => {
    const session = await seedTaskWithSession(testPage, apiClient, seedData, "COLORTERM Check");
    await focusTerminal(testPage, session);

    await testPage.keyboard.type("echo $COLORTERM");
    await testPage.keyboard.press("Enter");

    await session.expectTerminalHasText("truecolor");
  });

  test("Ctrl+Arrow moves cursor between words", async ({ testPage, apiClient, seedData }) => {
    const session = await seedTaskWithSession(testPage, apiClient, seedData, "CtrlArrow Test");
    await focusTerminal(testPage, session);

    // Start bash explicitly — it has Ctrl+Arrow bindings via /etc/inputrc
    // (zsh without .zshrc doesn't bind Ctrl+Arrow by default)
    await testPage.keyboard.type("bash --norc --noprofile");
    await testPage.keyboard.press("Enter");
    // Wait for bash prompt (bash --norc uses "bash-X.Y$ " pattern)
    await expect
      .poll(async () => (await readTerminalBuffer(testPage)).includes("bash"), {
        timeout: 5_000,
        message: "Waiting for bash prompt",
      })
      .toBe(true);

    // Type a multi-word echo command (don't press Enter yet)
    await testPage.keyboard.type("echo aaa bbb ccc");

    // Ctrl+Left twice: end → start of "ccc" → start of "bbb"
    await testPage.keyboard.press("Control+ArrowLeft");
    await testPage.keyboard.press("Control+ArrowLeft");

    // Insert "X" at cursor (before "bbb") → "echo aaa Xbbb ccc"
    await testPage.keyboard.type("X");
    await testPage.keyboard.press("Enter");

    // The shell should output "aaa Xbbb ccc"
    await session.expectTerminalHasText("aaa Xbbb ccc");
  });

  test("Meta+Arrow moves cursor to start/end of line", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedTaskWithSession(testPage, apiClient, seedData, "MetaArrow Test");
    await focusTerminal(testPage, session);

    // Start bash for a clean prompt
    await testPage.keyboard.type("bash --norc --noprofile");
    await testPage.keyboard.press("Enter");
    await expect
      .poll(async () => (await readTerminalBuffer(testPage)).includes("bash"), {
        timeout: 5_000,
        message: "Waiting for bash prompt",
      })
      .toBe(true);

    // Type text without "echo " prefix (just raw text, not a valid command)
    await testPage.keyboard.type("aaa bbb ccc");

    // Meta+Left → Home (cursor moves to start of line, sends Ctrl+A)
    await testPage.keyboard.press("Meta+ArrowLeft");

    // Type "echo " at the beginning → line becomes "echo aaa bbb ccc"
    await testPage.keyboard.type("echo ");
    await testPage.keyboard.press("Enter");

    // If Home worked, "echo " was inserted at the start and output is "aaa bbb ccc".
    // If Home failed, "echo " was appended and the command would fail.
    // Use a unique marker in the output to avoid matching the typed command itself.
    await expect
      .poll(
        async () => {
          const buf = await readTerminalBuffer(testPage);
          // Find lines that contain "aaa bbb ccc" but NOT "echo" (output lines, not command lines)
          const lines = buf.split("\n");
          return lines.some(
            (line) => line.includes("aaa bbb ccc") && !line.includes("echo") && !line.includes("$"),
          );
        },
        { timeout: 10_000, message: "Expected output line with 'aaa bbb ccc'" },
      )
      .toBe(true);
  });
});
