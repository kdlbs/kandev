import fs from "node:fs";
import path from "node:path";
import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

type ContextWindowStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => {
      tasks: { activeSessionId: string | null };
      setContextWindow: (
        sessionId: string,
        contextWindow: {
          size: number;
          used: number;
          remaining: number;
          efficiency: number;
          compactionCount: number;
          source: "acp" | "api";
        },
      ) => void;
    };
  };
};

/**
 * Seed a task + session via the API and navigate directly to the session page.
 * Waits for the mock agent to complete its turn (idle input visible).
 */
async function seedTaskWithSession(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  opts: { description?: string; agentProfileId?: string } = {},
): Promise<SessionPage> {
  const description = opts.description ?? "/e2e:simple-message";
  const agentProfileId = opts.agentProfileId ?? seedData.agentProfileId;
  const task = await apiClient.createTaskWithAgent(seedData.workspaceId, title, agentProfileId, {
    description,
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });

  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await testPage.goto(`/t/${task.id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });

  return session;
}

/** Seed a high-usage reading that represents the stale value from before reset. */
async function seedStaleContextWindow(testPage: Page): Promise<void> {
  await testPage.evaluate(() => {
    const store = (window as ContextWindowStoreWindow).__KANDEV_E2E_STORE__;
    if (!store) throw new Error("E2E store bridge is unavailable");

    const sessionId = store.getState().tasks.activeSessionId;
    if (!sessionId) throw new Error("No active session is available");

    store.getState().setContextWindow(sessionId, {
      size: 200_000,
      used: 190_000,
      remaining: 10_000,
      efficiency: 95,
      compactionCount: 0,
      source: "acp",
    });
  });
}

/**
 * Create an ACP profile for the mock agent that fails on resume. The
 * mock-agent's ACP LoadSession handler exits 1 when --fail-on-resume is set,
 * simulating an agent that can't restore a previous conversation — the
 * scenario that previously left the original recovery message's
 * "Resume session requested" button stuck on screen.
 */
async function createACPProfileWithFailOnResume(apiClient: ApiClient, name: string) {
  const { agents } = await apiClient.listAgents();
  const mockAgent = agents.find((a) => a.name === "mock-agent");
  if (!mockAgent) {
    throw new Error(
      `mock-agent not found in listAgents() (got ${agents.map((a) => `${a.id}=${a.name}`).join(", ")})`,
    );
  }
  return apiClient.createAgentProfile(mockAgent.id, name, {
    model: "mock-fast",
    cli_passthrough: false,
    cli_flags: [{ description: "fail on ACP resume", flag: "--fail-on-resume", enabled: true }],
  });
}

// Worst-case wait for the manual-recovery banner after `/crash`. The mock
// agent's crash is a real subprocess exit whose ACP-level error carries the
// same "peer disconnected before response" text the routingerr classifier
// now treats as a retryable transport-lost signature (see
// docs/decisions/2026-08-08-provider-neutral-agent-error-recovery.md) — every
// genuine production occurrence of that text also means the local subprocess
// died, so the classifier deliberately does not special-case a crash out of
// the retry ladder. `/crash` always re-crashes on relaunch, so recovery
// exhausts the full Kanban backoff (5+10+20+40+60s = 135s) before the manual
// recovery banner replaces the retry-countdown card. Give it headroom above
// that worst case rather than the default 30s.
const CRASH_RECOVERY_TIMEOUT = 170_000;

test.describe("Session recovery", () => {
  test.describe.configure({ retries: 1 });

  test("session startup keeps the composer editable while submission waits", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    // Keep workspace preparation in STARTING long enough to exercise the
    // startup gate. The backend fixture's git shim reads this file before
    // fetching the worktree, matching a slow real-world preparation phase.
    const delayFile = path.join(backend.tmpDir, "git-delay-ms");
    fs.writeFileSync(delayFile, "5000");

    try {
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Session startup composer readiness test",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: seedData.worktreeExecutorProfileId,
        },
      );

      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();

      const starting = testPage.locator('[data-placeholder="Preparing workspace..."]');
      const editor = session.activeChat().getByTestId("chat-input-editor");
      const submit = session.submitButton();
      await expect(starting).toBeVisible({ timeout: 15_000 });

      // @covers AC-UI-SESSION-START-COMPOSER-READINESS-001.1
      await expect(editor).toHaveAttribute("contenteditable", "true");
      await editor.fill("draft during startup");

      // @covers AC-UI-SESSION-START-COMPOSER-READINESS-001.2
      await expect(submit).toBeDisabled();

      await expect(starting).toBeHidden({ timeout: 60_000 });
      // The initial task prompt may still be finishing after environment
      // preparation. Wait for the same submit gate to clear, not a timer.
      await expect(submit).toBeEnabled({ timeout: 60_000 });

      // @covers AC-UI-SESSION-START-COMPOSER-READINESS-001.3
      await expect(editor).toHaveText("draft during startup");
    } finally {
      if (fs.existsSync(delayFile)) fs.unlinkSync(delayFile);
    }
  });

  test("reset context hides stale usage until a fresh report arrives", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedTaskWithSession(testPage, apiClient, seedData, "Reset Context Test");

    await seedStaleContextWindow(testPage);
    const contextRing = testPage.getByRole("button", { name: "Context window: 95% used" });
    const contextIndicators = testPage.getByRole("button", { name: /^Context window:/ });
    await expect(contextRing).toBeVisible();

    // Click reset context button — confirmation dialog should appear
    await session.resetContextButton().click();
    await expect(session.resetContextConfirm()).toBeVisible();

    // Confirm the reset
    await session.resetContextConfirm().click();

    // Divider should appear in chat
    await expect(session.contextResetDivider()).toBeVisible({ timeout: 30_000 });

    // Agent should restart and become idle again
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await expect(session.resetContextButton()).toBeVisible();
    await expect(contextIndicators).toHaveCount(0, { timeout: 30_000 });

    // The ring should return only after the agent reports fresh usage.
    await session.sendMessage("/background 1ms");
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await expect(testPage.getByRole("button", { name: "Context window: 2% used" })).toBeVisible({
      timeout: 30_000,
    });
  });

  test("agent crash — start fresh session recovers", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(220_000);

    const session = await seedTaskWithSession(
      testPage,
      apiClient,
      seedData,
      "Crash Recovery Fresh Test",
    );

    // Send /crash to make the agent exit with code 1
    await session.sendMessage("/crash");

    // Recovery buttons should appear
    await expect(session.recoveryFreshButton()).toBeVisible({
      timeout: CRASH_RECOVERY_TIMEOUT,
    });
    await expect(session.recoveryResumeButton()).toBeVisible();

    // Click "Start fresh session"
    await session.recoveryFreshButton().click();

    // Recovery briefly exposes the idle placeholder before the replacement
    // agent starts. Observe the starting phase before treating the composer as
    // ready so that transient idle state cannot satisfy the assertion.
    const freshStarting = testPage.locator('[data-placeholder="Preparing workspace..."]');
    await expect(freshStarting).toBeVisible({ timeout: 30_000 });
    await expect(freshStarting).not.toBeVisible({ timeout: 30_000 });
    await expect(testPage.getByTestId("chat-input-editor")).toHaveAttribute(
      "contenteditable",
      "true",
      {
        timeout: 30_000,
      },
    );

    // Verify agent works after recovery
    await session.sendMessage("/e2e:simple-message");
    await session.expectChatResponseVisible("simple mock response", 1, { timeout: 30_000 });
  });

  test("agent crash — resume session recovers", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(220_000);

    const session = await seedTaskWithSession(
      testPage,
      apiClient,
      seedData,
      "Crash Recovery Resume Test",
    );

    // Send /crash to make the agent exit with code 1
    await session.sendMessage("/crash");

    // Recovery buttons should appear
    await expect(session.recoveryResumeButton()).toBeVisible({
      timeout: CRASH_RECOVERY_TIMEOUT,
    });

    // Click "Resume session"
    await session.recoveryResumeButton().click();
    const editor = session.activeChat().getByTestId("chat-input-editor");
    // The recovery endpoint waits until the resumed agent is prompt-ready
    // before it resolves. The startup-specific composer gate is exercised by
    // the slow-preparation test above; this flow verifies the recovered
    // composer is usable afterwards.
    await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 30_000 });
    await expect(session.submitButton()).toBeEnabled({ timeout: 30_000 });

    // The resume settles the session back to WAITING_FOR_INPUT (agent idle).
    // The recovery card must not reappear now that the resume is resolved —
    // before the fix it came back until the next message flipped the session to
    // RUNNING, without the user ever typing anything.
    await expect(session.recoveryResumeButton()).toHaveCount(0);
    await expect(session.recoveryFreshButton()).toHaveCount(0);

    // Reload: the card is persisted, so a fix that only remembered the click in
    // component state would resurrect it here (this is also what the user hits
    // when a task is reopened and the session auto-resumes on open). The
    // transcript's own agent-boot record is what keeps it hidden.
    await testPage.reload();
    await session.waitForLoad();
    // Gate on the transcript's own boot record rather than the composer becoming
    // editable: the boot row is the signal the card is derived from, so waiting
    // for it removes the race where a fast hydration outruns the WS history.
    // (session.waitForChatIdle is unusable here — it clicks a visible Resume
    // button, which would hide the very regression this asserts.)
    await expect(testPage.getByText(/Resumed agent|Started agent/i).first()).toBeVisible({
      timeout: 30_000,
    });
    await expect(session.recoveryResumeButton()).toHaveCount(0);
    await expect(session.recoveryFreshButton()).toHaveCount(0);

    // Verify the resumed agent works after recovery.
    await session.sendMessage("/e2e:simple-message");
    await session.expectChatResponseVisible("simple mock response", 1, { timeout: 30_000 });
  });

  test("agent crash — resume fails again, no stuck button remains", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(220_000);

    // Unique suffix so a swallowed cleanup from a prior run doesn't collide on name.
    const profile = await createACPProfileWithFailOnResume(
      apiClient,
      `ACP Fail On Resume ${Date.now()}`,
    );

    try {
      const session = await seedTaskWithSession(
        testPage,
        apiClient,
        seedData,
        "Crash Recovery Resume Fails Test",
        { agentProfileId: profile.id },
      );

      // Crash the agent so the recovery message renders with action buttons.
      await session.sendMessage("/crash");
      await expect(session.recoveryResumeButton()).toBeVisible({
        timeout: CRASH_RECOVERY_TIMEOUT,
      });

      // Click "Resume session". The backend may resolve this through several
      // paths depending on a race (handleAgentStartFailed vs the AgentFailed
      // event from MarkCompleted) — either FAILED session state, a new
      // recovery ActionMessage, or a warning status. We don't pin the
      // downstream behaviour; the fix is purely frontend, so the assertion
      // below focuses on what the fix actually guarantees.
      await session.recoveryResumeButton().click();

      // The user-visible bug was the original button getting permanently
      // re-labelled to "Resume session requested" (disabled forever, only a
      // refresh got out of it). With the fix, the button unmounts as soon as
      // the ws_request completes — so the relabel never renders. Before the
      // fix this assertion fails as soon as the click's WS round-trip
      // resolves; with the fix the text never appears in the DOM.
      await expect(testPage.getByText(/Resume session requested/i)).toHaveCount(0, {
        timeout: 15_000,
      });
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true).catch(() => undefined);
    }
  });
});
