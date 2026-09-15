import type { Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";

/**
 * Stand up the one situation the compatibility gate has to distinguish: an
 * executor profile on which the seeded agent has no credentials while a
 * second agent type is compatible. The E2E backend registers only the mock
 * agent, so callers first restart it with `KANDEV_MOCK_PROVIDERS: "codex-acp"`
 * to get that second type; the remote-credentials catalog is then mocked so
 * the seeded agent needs an env secret the Docker profile does not carry and
 * the Codex alias needs nothing.
 */
export const SECOND_AGENT_ID = "codex-acp";
const SECOND_AGENT_DISPLAY_NAME = "Mock Codex";

type Scenario = {
  seedProfileName: string;
  compatibleProfileName: string;
  /** Display name shared by every profile of the second agent type. */
  secondAgentDisplayName: string;
  dockerProfileId: string;
  cleanup: () => Promise<void>;
};

export async function seedIncompatibleAgentScenario(
  apiClient: ApiClient,
  page: Page,
  seedAgentProfileId: string,
  names: { executor: string; dockerProfile: string; compatibleProfile: string },
): Promise<Scenario> {
  const seedProfile = await apiClient.getAgentProfile(seedAgentProfileId);
  const { agents } = await apiClient.listAgents();
  const seedAgent = agents.find((item) => item.profiles?.some((p) => p.id === seedProfile.id));
  if (!seedAgent) throw new Error(`agent for profile ${seedProfile.id} not found`);
  const secondAgent = agents.find((item) => item.name === SECOND_AGENT_ID);
  if (!secondAgent) {
    throw new Error(`${SECOND_AGENT_ID} is not registered; restart with KANDEV_MOCK_PROVIDERS`);
  }
  const compatibleProfile = await apiClient.createAgentProfile(
    secondAgent.id,
    names.compatibleProfile,
    { model: "mock-fast" },
  );
  const dockerExec = await apiClient.createExecutor(names.executor, "local_docker");
  const dockerProfile = await apiClient.createExecutorProfile(dockerExec.id, {
    name: names.dockerProfile,
    config: {
      image_tag: "kandev-mock-agent:test",
      dockerfile: "FROM busybox\nWORKDIR /workspace\n",
    },
  });
  await page.route("**/api/v1/remote-credentials", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        auth_specs: [
          {
            id: seedAgent.name,
            display_name: seedProfile.agentDisplayName,
            methods: [{ method_id: "test-token", type: "env", env_var: "TEST_TOKEN" }],
          },
          { id: SECOND_AGENT_ID, display_name: SECOND_AGENT_DISPLAY_NAME, methods: [] },
        ],
      }),
    });
  });
  return {
    seedProfileName: seedProfile.name,
    compatibleProfileName: compatibleProfile.name,
    secondAgentDisplayName: SECOND_AGENT_DISPLAY_NAME,
    dockerProfileId: dockerProfile.id,
    cleanup: async () => {
      await apiClient.deleteExecutor(dockerExec.id).catch(() => {});
      await apiClient.deleteAgentProfile(compatibleProfile.id, true).catch(() => {});
    },
  };
}

/** Pin `agentProfileId` on a fresh workflow and make it the dialog's remembered workflow. */
export async function seedLockedWorkflow(
  apiClient: ApiClient,
  workspaceId: string,
  boardWorkflowId: string,
  name: string,
  agentProfileId: string,
): Promise<{ id: string; cleanup: () => Promise<void> }> {
  const workflow = await apiClient.createWorkflow(workspaceId, name, "simple");
  await apiClient.updateWorkflow(workflow.id, { agent_profile_id: agentProfileId });
  const steps = await apiClient.listWorkflowSteps(workflow.id);
  const startStep = steps.steps.find((step) => step.is_start_step) ?? steps.steps[0];
  if (!startStep) throw new Error(`${name} workflow has no start step`);
  // The dialog restores the last-used workflow per workspace, so a task in
  // the locked workflow makes it the dialog's effective workflow.
  await apiClient.createTask(workspaceId, `Remembered ${name} task`, {
    workflow_id: workflow.id,
    workflow_step_id: startStep.id,
  });
  await apiClient.saveUserSettings({
    workspace_id: workspaceId,
    workflow_filter_id: boardWorkflowId,
  });
  return {
    id: workflow.id,
    cleanup: async () => {
      await apiClient.deleteWorkflow(workflow.id).catch(() => {});
      await apiClient.saveUserSettings({
        workspace_id: workspaceId,
        workflow_filter_id: boardWorkflowId,
      });
    },
  };
}
