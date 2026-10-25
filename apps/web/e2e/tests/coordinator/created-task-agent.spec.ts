// AC-COORDINATOR-TASK-AGENT: Agent for created tasks
// (docs/specs/coordinator/requirements/created-task-agent.md). The mobile-chrome
// project runs `mobile-created-task-agent.spec.ts` for the phone viewport.
import { expect, test } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { enableCoordinatorFeature, seedCoordinator } from "./coordinator-fixture";

const TASK_PAIR = "coordinator-task-pair";
const TASK_AGENT_PICKER = "coordinator-task-agent-profile-picker";
const AGENT_NAME = "Created Task Agent";

test.describe("Coordinator agent for created tasks", () => {
  test("the editor shows the pair, saves a change and keeps it after reload", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);
    const release = await enableCoordinatorFeature(backend, apiClient, seedData.workspaceId);
    try {
      const { agents } = await apiClient.listAgents();
      const profile = await apiClient.createAgentProfile(agents[0].id, AGENT_NAME, {
        model: "mock-fast",
        cli_passthrough: false,
      });
      const coordinator = await seedCoordinator(apiClient, seedData.workspaceId, {
        name: "Task Pair Coordinator",
        agentProfileId: seedData.agentProfileId,
        executorProfileId: seedData.worktreeExecutorProfileId,
      });
      expect(coordinator.task_agent_profile_id).toBe(seedData.agentProfileId);

      await testPage.goto(
        `/settings/workspaces/${seedData.workspaceId}/coordinators/${coordinator.id}`,
      );
      await expect(testPage.getByTestId(TASK_PAIR)).toBeVisible({ timeout: 15_000 });
      await expect(testPage.getByTestId(TASK_PAIR)).toContainText("Agent for created tasks");

      await testPage.getByTestId(TASK_AGENT_PICKER).click();
      await testPage.getByRole("option", { name: new RegExp(`${AGENT_NAME}$`) }).click();
      const patched = waitForHttp(
        testPage,
        "PATCH",
        new RegExp(`/api/v1/workspaces/${seedData.workspaceId}/coordinators/${coordinator.id}$`),
      );
      await testPage
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: "Save changes" })
        .click();
      await patched;

      await testPage.reload();
      await expect(testPage.getByTestId(TASK_AGENT_PICKER)).toContainText(AGENT_NAME);
      const stored = await apiClient.listCoordinators(seedData.workspaceId);
      const row = stored.coordinators.find((c) => c.id === coordinator.id);
      expect(row?.task_agent_profile_id).toBe(profile.id);
      expect(row?.agent_profile_id).toBe(seedData.agentProfileId);
    } finally {
      await release();
    }
  });

  test("a CLI passthrough profile is disabled in the task agent picker", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);
    const release = await enableCoordinatorFeature(backend, apiClient, seedData.workspaceId);
    try {
      const { agents } = await apiClient.listAgents();
      const passthrough = await apiClient.createAgentProfile(agents[0].id, "Task Passthrough", {
        model: "mock-fast",
        cli_passthrough: true,
      });
      const coordinator = await seedCoordinator(apiClient, seedData.workspaceId, {
        name: "Task Pair Warnings",
        agentProfileId: seedData.agentProfileId,
        executorProfileId: seedData.worktreeExecutorProfileId,
      });
      await testPage.goto(
        `/settings/workspaces/${seedData.workspaceId}/coordinators/${coordinator.id}`,
      );
      await testPage.getByTestId(TASK_AGENT_PICKER).click();
      const option = testPage.getByRole("option", { name: /Task Passthrough/ });
      await expect(option).toHaveAttribute("aria-disabled", "true");
      await testPage.keyboard.press("Escape");
      expect(passthrough.id).toBeTruthy();
    } finally {
      await release();
    }
  });
});
