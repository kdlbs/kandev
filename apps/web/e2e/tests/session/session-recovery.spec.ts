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
  await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

  return session;
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
      await expect(session.recoveryResumeButton()).toBeVisible({ timeout: 30_000 });

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
