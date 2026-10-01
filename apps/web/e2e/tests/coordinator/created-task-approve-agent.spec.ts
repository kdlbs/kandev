// AC-COORDINATOR-TASK-AGENT 001.6 / 001.8 / 001.10: an approved create-task
// proposal in a workspace with no step, workflow or workspace default agent is
// stamped with the coordinator's "Agent for created tasks", and the card says
// which agent will run. The proposal comes from the coordinator's own copilot
// chat (see what-it-did.spec.ts).
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { linkToCoordinatorNeedsYou } from "../../../lib/coordinator/links";
import { eligibleStep, type EligibleStepNode } from "../../../lib/coordinator/eligible-step";
import { stepHasOnEnterAction } from "../../../lib/types/http";
import { enableCoordinatorFeature } from "./coordinator-fixture";

const AGENT_NAME = "Stamped Task Agent";

test.describe("Coordinator approve stamps the agent for created tasks", () => {
  test("a task proposed with no agent anywhere runs with the coordinator's task agent", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);
    const release = await enableCoordinatorFeature(backend, apiClient, seedData.workspaceId);
    try {
      await apiClient.updateWorkspace(seedData.workspaceId, { default_agent_profile_id: "" });
      const { agents } = await apiClient.listAgents();
      const profile = await apiClient.createAgentProfile(agents[0].id, AGENT_NAME, {
        model: "mock-fast",
        cli_passthrough: false,
      });
      const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
        name: "Stamp Coordinator",
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: seedData.worktreeExecutorProfileId,
        task_agent_profile_id: profile.id,
        task_executor_profile_id: seedData.worktreeExecutorProfileId,
      });

      const nodes: EligibleStepNode[] = seedData.steps.map((step) => ({
        id: step.id,
        isStart: step.is_start_step ?? false,
        allowManualMove: step.allow_manual_move ?? false,
        autoStartOnEnter: stepHasOnEnterAction(step, "auto_start_agent"),
        pullFromStepId: step.pull_from_step_id ?? null,
      }));
      const step = nodes.find((node) => eligibleStep(nodes, node.id));
      expect(step, "the seeded workflow needs an eligible step").toBeTruthy();

      await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
      const conversationOpened = waitForHttp(
        testPage,
        "POST",
        /\/coordinators\/[^/]+\/conversation$/,
      );
      await testPage.getByTestId("coordinator-copilot-launcher").click();
      await conversationOpened;
      const popover = testPage.getByTestId("coordinator-copilot-popover");
      const editor = popover.getByTestId("chat-input-editor");
      await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 15_000 });
      const args = {
        title: "Stamped task",
        description: "Needs an agent to start.",
        rationale: "Exercise the stamp.",
        workflow_id: seedData.workflowId,
        step_id: step!.id,
        repository_id: seedData.repositoryId,
      };
      await editor.fill(`e2e:mcp:kandev:propose_task_kandev(${JSON.stringify(args)})`);
      await editor.press(`${process.platform === "darwin" ? "Meta" : "Control"}+Enter`);

      const card = popover
        .getByTestId("propose-task-renderer")
        .locator('[data-testid^="proposal-card-"]');
      await expect(card).toBeVisible({ timeout: 30_000 });
      await expect(card.getByTestId("proposal-runs-with")).toHaveText(`Runs with: ${AGENT_NAME}`);

      const approved = waitForHttp(testPage, "POST", /\/proposals\/[^/]+\/approve$/);
      await card.getByRole("button", { name: "Approve" }).click();
      const response = await approved;
      expect(response.status()).toBe(200);
      const settled = (await response.json()) as { status: string };
      expect(settled.status).toBe("approved");

      const listed = await apiClient.listTasks(seedData.workspaceId);
      const created = listed.tasks.find((candidate) => candidate.title === "Stamped task");
      expect(created, "the approved proposal should have created the task").toBeTruthy();
      const task = await apiClient.getTask(created!.id);
      expect(task.metadata).toMatchObject({
        agent_profile_id: profile.id,
        executor_profile_id: seedData.worktreeExecutorProfileId,
      });
    } finally {
      await release();
    }
  });
});
