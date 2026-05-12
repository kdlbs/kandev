import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

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
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    agentProfileId,
    {
      description,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await testPage.goto(`/t/${task.id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

  return session;
}

/**
 * Create an ACP profile for the mock agent that fails on resume.
 *
 * The mock-agent's ACP LoadSession handler rejects with an error when
 * --fail-on-resume is set, which drives the orchestrator's
 * handleResumeFailure path: clears the resume token, emits a warning status
 * message, and leaves the session in WAITING_FOR_INPUT without creating new
 * recovery action buttons. That's the exact failure mode that previously left
 * the original recovery message's "Resume session requested" button stuck on
 * screen.
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

test.describe("Session recovery", () => {
  test.describe.configure({ retries: 1 });

  test("reset context shows divider and agent responds fresh", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedTaskWithSession(testPage, apiClient, seedData, "Reset Context Test");

    // Click reset context button — confirmation dialog should appear
    await session.resetContextButton().click();
    await expect(session.resetContextConfirm()).toBeVisible();

    // Confirm the reset
    await session.resetContextConfirm().click();

    // Divider should appear in chat
    await expect(session.contextResetDivider()).toBeVisible({ timeout: 30_000 });

    // Agent should restart and become idle again
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // Verify agent works after reset by sending a new message
    await session.sendMessage("/e2e:simple-message");
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
  });

  test("agent crash — start fresh session recovers", async ({ testPage, apiClient, seedData }) => {
    const session = await seedTaskWithSession(
      testPage,
      apiClient,
      seedData,
      "Crash Recovery Fresh Test",
    );

    // Send /crash to make the agent exit with code 1
    await session.sendMessage("/crash");

    // Recovery buttons should appear
    await expect(session.recoveryFreshButton()).toBeVisible({ timeout: 30_000 });
    await expect(session.recoveryResumeButton()).toBeVisible();

    // Click "Start fresh session"
    await session.recoveryFreshButton().click();

    // Agent should recover and become idle
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // Verify agent works after recovery
    await session.sendMessage("/e2e:simple-message");
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
  });

  test("agent crash — resume session recovers", async ({ testPage, apiClient, seedData }) => {
    const session = await seedTaskWithSession(
      testPage,
      apiClient,
      seedData,
      "Crash Recovery Resume Test",
    );

    // Send /crash to make the agent exit with code 1
    await session.sendMessage("/crash");

    // Recovery buttons should appear
    await expect(session.recoveryResumeButton()).toBeVisible({ timeout: 30_000 });

    // Click "Resume session"
    await session.recoveryResumeButton().click();

    // Agent should recover and become idle
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // Verify agent works after recovery
    await session.sendMessage("/e2e:simple-message");
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
  });

  test("agent crash — resume fails again, no stuck button remains", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const profile = await createACPProfileWithFailOnResume(apiClient, "ACP Fail On Resume");

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
      await expect(session.recoveryResumeButton()).toBeVisible({ timeout: 30_000 });

      // Snapshot the original recovery button locator. Once the resume request
      // fires and the ws_request finishes (regardless of downstream outcome),
      // the fix unmounts this button. We assert it disappears below.
      const originalResumeButton = session.recoveryResumeButton();

      // Click "Resume session". The mock agent's LoadSession will reject
      // because --fail-on-resume is set, the orchestrator clears the token,
      // emits the resume-failed warning status, and leaves the session in
      // WAITING_FOR_INPUT — so the old ActionMessage stays mounted.
      await originalResumeButton.click();

      // The follow-up warning status message renders.
      await expect(
        testPage.getByText(/Previous agent session could not be restored/i),
      ).toBeVisible({ timeout: 30_000 });

      // The original recovery resume button must be gone: either the whole
      // ActionMessage unmounted, or just its action buttons collapsed. Either
      // way, no stuck "Resume session" remains in the DOM. Before the fix the
      // button stayed mounted with a "Resume session requested" label and
      // permanently disabled — the assertion below would fail on main.
      await expect(originalResumeButton).toHaveCount(0, { timeout: 15_000 });

      // Specifically guard against the stuck "Resume session requested" label.
      await expect(testPage.getByText(/Resume session requested/i)).toHaveCount(0);
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true).catch(() => undefined);
    }
  });
});
