import { type Page } from "@playwright/test";

import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { typeWhileBusy } from "../../helpers/type-while-busy";
import { SessionPage } from "../../pages/session-page";

// ---------------------------------------------------------------------------
// Pause → resume recovery (#1597 pause→resume recovery)
//
// The operator's headline pain: pause a running agent turn and the session
// wedges — the next message is dropped or the composer stays stuck "running",
// and the only recovery is to restart the whole headless service. These tests
// pin the corrected behavior end-to-end against the mock agent:
//
//   1. Pausing a running turn returns the SAME session to an input-ready state;
//      a newly typed message resumes it with prior context intact — no wedged
//      "still running" composer, no service restart.
//   2. A message queued while the turn was running is DELIVERED once the pause
//      settles the session, rather than being stranded on the queue.
// ---------------------------------------------------------------------------

/** Seed an ACP task, open its session, and wait for the initial turn to idle. */
async function seedTaskAndWaitForIdle(
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
  await session.waitForChatIdle({ timeout: 30_000 });
  return session;
}

test.describe("Pause → resume recovery", () => {
  // Cancel/resume timing can be sensitive under CI load.
  test.describe.configure({ retries: 1 });

  test("pausing a running turn lets a newly typed message resume the same session", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const session = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Pause then resume",
    );

    // The initial turn produced one response — this is the context that must
    // survive the pause and prove the same session (not a fresh one) resumed.
    await expect(
      session.chat.getByText("simple mock response", { exact: false }).nth(0),
    ).toBeVisible({ timeout: 30_000 });
    const sessionUrl = testPage.url();

    // Start a long-running turn so we have time to pause it mid-flight.
    await session.sendMessage("/slow 8s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
    await expect(session.cancelAgentButton()).toBeVisible({ timeout: 15_000 });

    // Pause: the operator stops the running turn.
    await session.cancelAgentButton().click();

    // The session must settle back to input-ready — NOT wedge on "still
    // running". This is the regression that forced a service restart.
    await expect(session.agentStatus()).not.toBeVisible({ timeout: 30_000 });
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await expect(session.cancelAgentButton()).not.toBeVisible({ timeout: 15_000 });

    // Still the same session — no navigation / service bounce.
    expect(testPage.url()).toBe(sessionUrl);

    // Sending a new message resumes the SAME session with its context intact.
    await session.sendMessage("/e2e:simple-message");
    await session.expectChatResponseVisible("simple mock response", 1, { timeout: 30_000 });

    // Prior conversation is still present — context was preserved across the pause.
    await expect(
      session.chat.getByText("simple mock response", { exact: false }).nth(0),
    ).toBeVisible({ timeout: 15_000 });
  });

  test("a message queued during a running turn is delivered when the turn is paused", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const session = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Pause drains queued message",
    );

    // Keep the agent busy, then queue a follow-up while it is running.
    await session.sendMessage("/slow 8s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
    await expect(session.cancelAgentButton()).toBeVisible({ timeout: 15_000 });

    const editor = testPage.locator(".tiptap.ProseMirror").first();
    await typeWhileBusy(testPage, editor, "/e2e:simple-message");
    await testPage.getByTestId("submit-message-button").click();

    // The queued message is parked while the turn runs.
    await expect(testPage.getByTestId("queue-chip")).toBeVisible({ timeout: 10_000 });

    // Pause the running turn. Cancel must DELIVER the queued message rather than
    // strand it: on an escalated / dead-process cancel no agent.ready fires, so
    // CancelAgent drains the queue directly.
    await session.cancelAgentButton().click();

    // The queued message drains (chip clears) and its response arrives — proof
    // the paused session accepted the queued input instead of losing it.
    await expect(testPage.getByTestId("queue-chip")).not.toBeVisible({ timeout: 30_000 });
    await session.expectChatResponseVisible("simple mock response", 1, { timeout: 30_000 });
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
  });
});
