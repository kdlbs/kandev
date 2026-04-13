import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

/**
 * Navigate to a kanban card by title and open its session page.
 */
async function openTaskSession(page: Parameters<typeof KanbanPage>[0], title: string) {
  const kanban = new KanbanPage(page);
  await kanban.goto();

  const card = kanban.taskCardByTitle(title);
  await expect(card).toBeVisible({ timeout: 15_000 });
  await card.click();
  await expect(page).toHaveURL(/\/t\//, { timeout: 15_000 });

  const session = new SessionPage(page);
  await session.waitForLoad();

  // Reload to get fresh SSR state — the session may have started via API before
  // the WS connection was established, causing the UI to miss "session is RUNNING"
  // events. A reload guarantees hydrated state from the server.
  await page.reload();
  await session.waitForLoad();

  return session;
}

/**
 * Stop the running session via the tab context menu (session.stop → CANCELLED state),
 * then wait for the FailedSessionBanner to appear.
 *
 * Note: clicking the cancel-agent-button sends `agent.cancel` which only cancels
 * the current turn and transitions to WAITING_FOR_INPUT (not CANCELLED/FAILED).
 * To reach CANCELLED we need `session.stop` which is triggered via right-click → Stop.
 */
async function stopRunningSession(session: SessionPage) {
  // Wait for the agent to be actively running (cancel button becomes visible)
  await expect(session.cancelAgentButton()).toBeVisible({ timeout: 30_000 });

  // Right-click the session tab to open context menu, then click "Stop"
  // This sends session.stop → CANCELLED state
  await session.rightClickFirstSessionTab();
  await session.contextMenuItem("Stop").click();

  // Wait for the FailedSessionBanner's resume button to appear (isFailed = FAILED | CANCELLED)
  await expect(session.failedSessionResumeButton()).toBeVisible({ timeout: 30_000 });
}

test.describe("New session with deleted agent profile", () => {
  test("resume button is disabled when agent profile was deleted", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    // 1. Get agent info and create a temporary custom profile
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    if (!agent) throw new Error("No agents available");
    const profile = await apiClient.createAgentProfile(agent.id, "Temp Profile for Delete Test", {
      model: agent.profiles[0]?.model ?? "mock",
      auto_approve: true,
    });

    // 2. Create task using the custom profile with a slow response so we can cancel it
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Deleted Profile Resume Test",
      profile.id,
      {
        description: "/slow 60s",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 3. Navigate to the task and stop the agent to get CANCELLED state
    const session = await openTaskSession(testPage, "Deleted Profile Resume Test");
    await stopRunningSession(session);

    // 4. While on the page, force-delete the custom agent profile.
    //    The frontend receives an "agent.profile.deleted" WS event and removes
    //    the profile from the store, making profileExists=false reactively.
    await apiClient.deleteAgentProfile(profile.id, true);

    // 5. Wait for the resume button to become disabled (store has updated)
    await expect(session.failedSessionResumeButton()).toBeDisabled({ timeout: 10_000 });

    // 6. Hover the span wrapper (tooltip trigger) and verify the tooltip text
    await testPage.getByTestId("failed-session-resume-wrapper").hover();
    await expect(testPage.getByRole("tooltip")).toContainText(
      "Agent profile no longer exists",
      { timeout: 5_000 },
    );
  });

  test("new agent dialog falls back to available profile when original was deleted", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    // 1. Get agent info and create a temporary custom profile
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    if (!agent) throw new Error("No agents available");
    const profile = await apiClient.createAgentProfile(
      agent.id,
      "Temp Profile for Dialog Test",
      {
        model: agent.profiles[0]?.model ?? "mock",
        auto_approve: true,
      },
    );

    // 2. Create task using the custom profile with a slow response so we can cancel it
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Deleted Profile Dialog Test",
      profile.id,
      {
        description: "/slow 60s",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 3. Navigate to the task and stop the agent to get CANCELLED state
    const session = await openTaskSession(testPage, "Deleted Profile Dialog Test");
    await stopRunningSession(session);

    // 4. While on the page, force-delete the custom agent profile.
    //    The frontend receives an "agent.profile.deleted" WS event and removes
    //    the profile from the store.
    await apiClient.deleteAgentProfile(profile.id, true);

    // 5. Wait for the resume button to become disabled (confirms store updated)
    await expect(session.failedSessionResumeButton()).toBeDisabled({ timeout: 10_000 });

    // 6. Click the "New Agent" button in the FailedSessionBanner to open the dialog
    await testPage.getByRole("button", { name: "New Agent" }).click();

    // 7. Dialog should be visible
    await expect(session.newSessionDialog()).toBeVisible({ timeout: 5_000 });

    // 8. Agent selector should be visible because the original profile was deleted
    //    (isDefaultProfileMissing=true forces the selector to show even with 1 profile)
    const agentSelector = session.newSessionDialog().getByTestId("agent-profile-selector");
    await expect(agentSelector).toBeVisible();

    // 9. Fill in a prompt and submit to create a new session with the fallback profile
    await session.newSessionPromptInput().fill("/e2e:simple-message");
    await session.newSessionStartButton().click();

    // 10. Dialog should close after submit
    await expect(session.newSessionDialog()).not.toBeVisible({ timeout: 10_000 });

    // 11. Verify the new session tab is visible
    await expect(session.sessionTabByText("2")).toBeVisible({ timeout: 15_000 });
  });
});
