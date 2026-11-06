import { test, expect, type Page } from "../../fixtures/test-base";
import { createHandoffProfiles } from "../../helpers/session-handoff-profile-fixtures";
import { SessionPage } from "../../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

type AgentProfileOption = {
  id: string;
  agent_id: string;
  capability_status?: string;
  [key: string]: unknown;
};

type E2EStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => { agentProfiles: { items: AgentProfileOption[] } };
    setState: (
      updater: (state: { agentProfiles: { items: AgentProfileOption[] } }) => void,
    ) => void;
  };
};

/**
 * Marks an existing agent profile option as capability-not-ready by mutating
 * the client store directly, mirroring the effect of a real
 * `agent.available.updated` broadcast reporting the agent's CLI missing (that
 * event refreshes `capability_status` on both `agentProfiles.items` and
 * `settingsAgents.items` — see agents.ts `refreshProfileCapabilities` /
 * `refreshSettingsAgentsCapabilities`). The e2e mock agent
 * (KANDEV_MOCK_AGENT=only) never enters the host-utility cache, so
 * `capability_status` is always undefined for it in this suite — this is the
 * only way to exercise the "not ready" branch end to end.
 */
async function markAgentCapabilityNotInstalled(testPage: Page, agentId: string) {
  return testPage.evaluate((id) => {
    const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
    if (!store) {
      throw new Error("E2E store bridge missing — is __KANDEV_E2E_EXPOSE_STORE__ set?");
    }
    store.setState((state) => {
      for (const profile of state.agentProfiles.items) {
        if (profile.agent_id === id) profile.capability_status = "not_installed";
      }
    });
    const ownedProfiles = store.getState().agentProfiles.items.filter((p) => p.agent_id === id);
    if (
      ownedProfiles.length === 0 ||
      ownedProfiles.some((p) => p.capability_status !== "not_installed")
    ) {
      throw new Error("Failed to set capability_status on the agent's profiles in store");
    }
    return store.getState().agentProfiles.items.every((profile) => profile.agent_id === id);
  }, agentId);
}

test.describe("Session handoff filters unhealthy profiles", () => {
  test("hides every mock profile after its agent capability becomes unavailable", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const { profileA, profileB } = await createHandoffProfiles(apiClient);

    try {
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Handoff Filter Task",
        profileA.id,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: seedData.worktreeExecutorProfileId,
        },
      );

      await expect
        .poll(
          async () => {
            const { sessions } = await apiClient.listTaskSessions(task.id);
            return DONE_STATES.includes(sessions[0]?.state ?? "");
          },
          { timeout: 30_000, message: "Waiting for first session to finish" },
        )
        .toBe(true);

      const { sessions } = await apiClient.listTaskSessions(task.id);
      const session1Id = sessions[0].id;

      // The handoff flow does not depend on Kanban rendering. Navigate directly
      // to the task so a slow board refresh cannot consume the test timeout
      // before the session menu is exercised.
      await testPage.goto(`/t/${task.id}`);
      await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

      const session = new SessionPage(testPage);
      await session.waitForLoad();

      // Capability loss belongs to the agent, including profiles retained from
      // earlier tests in the worker. Both fixture profiles share the same agent.
      const allProfilesUnavailable = await markAgentCapabilityNotInstalled(
        testPage,
        profileA.agentId,
      );

      await session.sessionTabBySessionId(session1Id).click({ button: "right" });
      await session.handoffSubmenu().hover();
      await expect(session.handoffProfileItem(profileA.id)).not.toBeVisible();
      await expect(session.handoffProfileItem(profileB.id)).not.toBeVisible();
      if (allProfilesUnavailable) {
        await expect(
          testPage.getByText(
            "No agent profiles are ready. Install or reconnect an agent CLI in Settings → Agents.",
          ),
        ).toBeVisible();
      }
    } finally {
      await apiClient.deleteAgentProfile(profileB.id, true);
      await apiClient.deleteAgentProfile(profileA.id, true);
    }
  });
});
