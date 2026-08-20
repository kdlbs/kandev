import { test, expect, type Page } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
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

async function createProfiles(
  apiClient: InstanceType<typeof import("../../helpers/api-client").ApiClient>,
) {
  const { agents } = await apiClient.listAgents();
  if (agents.length === 0) throw new Error("no agents available in test fixtures");
  const agentId = agents[0].id;
  const profileA = await apiClient.createAgentProfile(agentId, "Handoff Filter Profile A", {
    model: "mock-fast",
  });
  const profileB = await apiClient.createAgentProfile(agentId, "Handoff Filter Profile B", {
    model: "mock-slow",
  });
  return { profileA, profileB };
}

test.describe("Session handoff filters unhealthy profiles", () => {
  test("hides a profile once its agent capability is not ready, keeps healthy profiles visible", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const { profileA, profileB } = await createProfiles(apiClient);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Handoff Filter Task",
      profileA.id,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
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

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.taskCardByTitle("Handoff Filter Task").click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await session.sessionTabBySessionId(session1Id).click({ button: "right" });
    await session.handoffSubmenu().hover();
    await expect(session.handoffProfileItem(profileB.id)).toBeVisible({ timeout: 5_000 });
    await testPage.keyboard.press("Escape");

    await markProfileCapabilityNotInstalled(testPage, profileB.id);

    await session.sessionTabBySessionId(session1Id).click({ button: "right" });
    await session.handoffSubmenu().hover();
    await expect(session.handoffProfileItem(profileA.id)).toBeVisible({ timeout: 5_000 });
    await expect(session.handoffProfileItem(profileB.id)).not.toBeVisible();
  });
});
