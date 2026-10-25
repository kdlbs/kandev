// AC-COORDINATOR-PROPOSAL-KINDS-001.1, -003.1, -003.3
// (docs/specs/coordinator/requirements/proposal-kinds.md).
//
// The mock agent proposes a move through the real MCP surface, the test
// approves it over the REST route a manager uses, and the task's step is read
// back through the task API. The move card UI is a later work order, so this
// spec drives the proposal with the API only.
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { waitForSessionState } from "../../helpers/session";
import { linkToCoordinatorNeedsYou } from "../../../lib/coordinator/links";
import { eligibleStep, type EligibleStepNode } from "../../../lib/coordinator/eligible-step";
import { stepHasOnEnterAction } from "../../../lib/types/http";

const POLICY_ALLOWING_MOVE = {
  version: 1,
  actions: {
    create_task: "requires_approval",
    start_agent: "denied",
    message: "denied",
    move: "requires_approval",
    resume: "denied",
    stop: "denied",
  },
};

test.describe("Coordinator proposal kinds backend", () => {
  test("a proposed move is applied to the task only after a manager approves it", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const nodes: EligibleStepNode[] = seedData.steps.map((step) => ({
      id: step.id,
      isStart: step.is_start_step ?? false,
      allowManualMove: step.allow_manual_move ?? false,
      autoStartOnEnter: stepHasOnEnterAction(step, "auto_start_agent"),
      pullFromStepId: step.pull_from_step_id ?? null,
    }));
    const eligible = nodes.filter((node) => eligibleStep(nodes, node.id));
    expect(eligible.length, "the seeded workflow needs two quiet steps").toBeGreaterThanOrEqual(2);
    const [fromStep, toStep] = eligible;

    const release = await backend.useEnv({
      KANDEV_FEATURES_COORDINATOR: "true",
      KANDEV_FEATURES_COORDINATOR_PHASE2: "true",
    });
    try {
      const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
        name: "Kinds Coordinator",
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: seedData.worktreeExecutorProfileId,
        task_agent_profile_id: seedData.agentProfileId,
        task_executor_profile_id: seedData.worktreeExecutorProfileId,
      });
      const workspacePath = `/api/v1/workspaces/${seedData.workspaceId}/coordinators/${coordinator.id}`;
      const saved = await apiClient.rawRequest("PUT", `${workspacePath}/settings`, {
        policy: POLICY_ALLOWING_MOVE,
      });
      expect(saved.status).toBe(200);

      const task = await apiClient.createTask(seedData.workspaceId, "Task to move", {
        workflow_id: seedData.workflowId,
        workflow_step_id: fromStep.id,
      });

      await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
      const conversationOpened = waitForHttp(
        testPage,
        "POST",
        /\/coordinators\/[^/]+\/conversation$/,
      );
      await testPage.getByTestId("coordinator-copilot-launcher").click();
      const opened = (await (await conversationOpened).json()) as {
        task_id: string;
        session_id: string;
      };
      const popover = testPage.getByTestId("coordinator-copilot-popover");
      const editor = popover.getByTestId("chat-input-editor");
      await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 15_000 });

      const args = { task_id: task.id, step_id: toStep.id, rationale: "It is ready for review." };
      await editor.fill(`e2e:mcp:kandev:propose_move_kandev(${JSON.stringify(args)})`);
      await editor.press(`${process.platform === "darwin" ? "Meta" : "Control"}+Enter`);
      await waitForSessionState(apiClient, {
        taskId: opened.task_id,
        sessionId: opened.session_id,
        expectedState: "WAITING_FOR_INPUT",
        message: "the mock agent's turn ends after the propose call",
        timeout: 30_000,
      });

      let proposalId = "";
      await expect
        .poll(
          async () => {
            const listed = await apiClient.rawRequest("GET", `${workspacePath}/proposals`);
            const body = (await listed.json()) as {
              proposals: Array<{ id: string; kind: string; status: string }>;
            };
            proposalId = body.proposals.find((p) => p.kind === "move")?.id ?? "";
            return proposalId;
          },
          { timeout: 30_000, message: "the move proposal should be stored" },
        )
        .not.toBe("");

      // Proposing moves nothing.
      expect((await apiClient.getTask(task.id)).workflow_step_id).toBe(fromStep.id);

      const approved = await apiClient.rawRequest(
        "POST",
        `${workspacePath}/proposals/${proposalId}/approve`,
        {},
      );
      expect(approved.status).toBe(200);
      const settled = (await approved.json()) as {
        status: string;
        outcome: { from_step_id: string; to_step_id: string };
      };
      expect(settled.status).toBe("approved");
      expect(settled.outcome).toMatchObject({ from_step_id: fromStep.id, to_step_id: toStep.id });

      await expect
        .poll(async () => (await apiClient.getTask(task.id)).workflow_step_id, { timeout: 15_000 })
        .toBe(toStep.id);
    } finally {
      await release();
    }
  });
});
