import { test, expect, type Page } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

type AgentProfileOption = {
  id: string;
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
async function markProfileCapabilityNotInstalled(testPage: Page, profileId: string) {
  await testPage.evaluate((id) => {
    const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
    if (!store) {
      throw new Error("E2E store bridge missing — is __KANDEV_E2E_EXPOSE_STORE__ set?");
    }
    store.setState((state) => {
      const index = state.agentProfiles.items.findIndex((p) => p.id === id);
      if (index === -1) {
        throw new Error(`agent profile ${id} not found in store`);
      }
      state.agentProfiles.items[index] = {
        ...state.agentProfiles.items[index],
        capability_status: "not_installed",
      };
    });
    const updated = store.getState().agentProfiles.items.find((p) => p.id === id);
    if (updated?.capability_status !== "not_installed") {
      throw new Error("Failed to set capability_status on agent profile in store");
    }
  }, profileId);
}

async function addHealthyHandoffProfile(testPage: Page, sourceProfileId: string) {
  const profileId = "e2e-handoff-profile-b";
  const name = "Handoff Filter Profile B";

  await testPage.waitForFunction(
    (id) => {
      const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
      return store?.getState().agentProfiles.items.some((profile) => profile.id === id) ?? false;
    },
    sourceProfileId,
    { timeout: 10_000 },
  );

  return testPage.evaluate(
    ({ sourceProfileId: sourceId, profileId: id, name: profileName }) => {
      const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
      if (!store) {
        throw new Error("E2E store bridge missing — is __KANDEV_E2E_EXPOSE_STORE__ set?");
      }
      const source = store
        .getState()
        .agentProfiles.items.find((profile) => profile.id === sourceId);
      if (!source) throw new Error(`agent profile ${sourceId} not found in store`);

      const profile = {
        ...source,
        id,
        name: profileName,
        model: "mock-slow",
        capability_status: undefined,
      };
      store.setState((state) => {
        const index = state.agentProfiles.items.findIndex((item) => item.id === id);
        if (index === -1) state.agentProfiles.items.push(profile);
        else state.agentProfiles.items[index] = profile;
      });

      return profile.id;
    },
    { sourceProfileId, profileId, name },
  );
}

test.describe("Session handoff filters unhealthy profiles", () => {
  test("hides a profile once its agent capability is not ready, keeps healthy profiles visible", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const profileAId = seedData.agentProfileId;

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Handoff Filter Task",
      profileAId,
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
    const profileBId = await addHealthyHandoffProfile(testPage, profileAId);

    await session.sessionTabBySessionId(session1Id).click({ button: "right" });
    await session.handoffSubmenu().hover();
    await expect(session.handoffProfileItem(profileBId)).toBeVisible({ timeout: 5_000 });
    await testPage.keyboard.press("Escape");

    // A real agent.available.updated event changes every profile for the
    // agent. Both options share the seeded profile's agent, so update both.
    await markProfileCapabilityNotInstalled(testPage, profileAId);
    await markProfileCapabilityNotInstalled(testPage, profileBId);

    await session.sessionTabBySessionId(session1Id).click({ button: "right" });
    await session.handoffSubmenu().hover();
    await expect(session.handoffProfileItem(profileAId)).not.toBeVisible();
    await expect(session.handoffProfileItem(profileBId)).not.toBeVisible();
  });
});
