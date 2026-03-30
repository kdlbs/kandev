import { test } from "../../fixtures/test-base";

/**
 * Real agent session resume tests.
 *
 * These tests use actual agent binaries (Claude Code, etc.) and incur API costs.
 * They are skipped by default and only run when KANDEV_E2E_REAL_AGENTS=1 is set.
 *
 * Prerequisites:
 * - Agent binary installed and on PATH
 * - Required API keys set (e.g. ANTHROPIC_API_KEY for Claude Code)
 * - Backend NOT running with KANDEV_MOCK_AGENT=only
 *
 * Run:
 *   KANDEV_E2E_REAL_AGENTS=1 pnpm --filter @kandev/web e2e -- tests/session-resume-real.spec.ts
 */

const realAgentsEnabled = process.env.KANDEV_E2E_REAL_AGENTS === "1";

test.describe("Session resume (real agents)", () => {
  test.skip(!realAgentsEnabled, "Set KANDEV_E2E_REAL_AGENTS=1 to enable");

  // No retries — real agent calls are expensive
  test.describe.configure({ retries: 0 });

  test("resumable agent: resume preserves conversation context", async () => {
    test.setTimeout(180_000);

    // TODO: Implement once mock agent resume tests are stable.
    //
    // Fixtures available: testPage, apiClient, seedData, backend
    //
    // Flow:
    // 1. Find a resumable agent profile (Claude Code, etc.)
    // 2. Create task with a distinctive prompt (e.g. "Remember the word 'pineapple'")
    // 3. Wait for agent to respond and reach idle state
    // 4. backend.restart()
    // 5. Reload page, wait for auto-resume
    // 6. Send follow-up: "What word did I ask you to remember?"
    // 7. Verify agent responds with "pineapple" (proves context preserved)
    test.skip(true, "Scaffold — implement when mock agent tests are stable");
  });

  test("non-resumable agent: new session created after restart", async () => {
    test.setTimeout(180_000);

    // TODO: Implement once mock agent resume tests are stable.
    //
    // Fixtures available: testPage, apiClient, seedData, backend
    //
    // Flow:
    // 1. Find a non-resumable agent (CanRecover=false)
    // 2. Create task with a prompt
    // 3. Wait for agent to respond
    // 4. backend.restart()
    // 5. Reload page
    // 6. Verify session is NOT auto-resumed (needs_resume=false)
    // 7. Manually start a new session
    // 8. Verify agent works without prior context
    // 9. Verify previous messages from the old session are still visible
    test.skip(true, "Scaffold — implement when mock agent tests are stable");
  });
});
