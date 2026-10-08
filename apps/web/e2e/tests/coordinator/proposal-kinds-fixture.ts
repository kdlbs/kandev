// Shared by proposal-kinds.spec.ts and mobile-proposal-kinds.spec.ts: seeds a
// coordinator whose policy allows a move, has the mock agent propose one through
// the copilot chat, and returns once the proposal is stored with the popover closed.
import type { Locator } from "@playwright/test";
import { expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { waitForSessionState } from "../../helpers/session";
import { linkToCoordinatorNeedsYou } from "../../../lib/coordinator/links";
import { eligibleStep, type EligibleStepNode } from "../../../lib/coordinator/eligible-step";
import { stepHasOnEnterAction } from "../../../lib/types/http";
import type { test } from "../../fixtures/test-base";

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

export type ProposalKindUnderTest = "move" | "message";

export const MESSAGE_PROPOSAL_TEXT = "Please run the tests before you finish.";

export async function setupMoveProposal(
  submit: (popover: Locator, editor: Locator) => Promise<void>,
  ctx: Pick<
    Parameters<Parameters<typeof test>[2]>[0],
    "testPage" | "apiClient" | "backend" | "seedData"
  >,
  kind: ProposalKindUnderTest = "move",
) {
  const { testPage, apiClient, backend, seedData } = ctx;
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
  const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
    name: "Kinds UI Coordinator",
    agent_profile_id: seedData.agentProfileId,
    executor_profile_id: seedData.worktreeExecutorProfileId,
  });
  const workspacePath = `/api/v1/workspaces/${seedData.workspaceId}/coordinators/${coordinator.id}`;
  const saved = await apiClient.rawRequest("PUT", `${workspacePath}/settings`, {
    policy:
      kind === "message"
        ? {
            ...POLICY_ALLOWING_MOVE,
            actions: { ...POLICY_ALLOWING_MOVE.actions, message: "requires_approval" },
          }
        : POLICY_ALLOWING_MOVE,
  });
  expect(saved.status).toBe(200);
  const task =
    kind === "message"
      ? await apiClient.createTaskWithAgent(
          seedData.workspaceId,
          "Task to message",
          seedData.agentProfileId,
          {
            description: "/e2e:simple-message",
            workflow_id: seedData.workflowId,
            workflow_step_id: seedData.startStepId,
            repository_ids: [seedData.repositoryId],
          },
        )
      : await apiClient.createTask(seedData.workspaceId, "Task to move", {
          workflow_id: seedData.workflowId,
          workflow_step_id: fromStep.id,
        });
  if (kind === "message") {
    if (!task.session_id) throw new Error("expected an active session for the message task");
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId: task.session_id,
      expectedState: "WAITING_FOR_INPUT",
      message: "the task's agent is idle before the message is proposed",
      timeout: 30_000,
    });
  }

  await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
  const conversationOpened = waitForHttp(testPage, "POST", /\/coordinators\/[^/]+\/conversation$/);
  await testPage.getByTestId("coordinator-copilot-launcher").click();
  const opened = (await (await conversationOpened).json()) as {
    task_id: string;
    session_id: string;
  };
  const popover = testPage.getByTestId("coordinator-copilot-popover");
  const editor = popover.getByTestId("chat-input-editor");
  await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 15_000 });
  const rationale = "It is ready for review.";
  const line =
    kind === "message"
      ? `e2e:mcp:kandev:propose_message_kandev(${JSON.stringify({ task_id: task.id, text: MESSAGE_PROPOSAL_TEXT, rationale })})`
      : `e2e:mcp:kandev:propose_move_kandev(${JSON.stringify({ task_id: task.id, step_id: toStep.id, rationale })})`;
  await editor.fill(line);
  await submit(popover, editor);
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
          proposals: Array<{ id: string; kind: string }>;
        };
        proposalId = body.proposals.find((p) => p.kind === kind)?.id ?? "";
        return proposalId;
      },
      { timeout: 30_000, message: "the proposal should be stored" },
    )
    .not.toBe("");

  await popover.getByRole("button", { name: "Close" }).focus();
  await testPage.keyboard.press("Escape");
  await expect(popover).toBeHidden();
  return { release, task, toStep, fromStep, proposalId, workspacePath };
}

const PR_OWNER = "coordinator-e2e-org";
const PR_REPO = "coordinator-e2e-repo";

// Seeds a coordinator and a task whose primary session is idle and whose PR
// aggregates to "ready" (approved, checks passing, clean merge), so the Queue
// lists it under Ready to merge with the phase-2 row actions.
export async function setupReadyToMergeTask(
  ctx: Pick<Parameters<Parameters<typeof test>[2]>[0], "apiClient" | "backend" | "seedData">,
) {
  const { apiClient, backend, seedData } = ctx;
  const release = await backend.useEnv({
    KANDEV_FEATURES_COORDINATOR: "true",
    KANDEV_FEATURES_COORDINATOR_PHASE2: "true",
  });
  const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
    name: "Send Back Coordinator",
    agent_profile_id: seedData.agentProfileId,
    executor_profile_id: seedData.worktreeExecutorProfileId,
  });
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Ready To Merge Send Back",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("expected an active session for the ready task");
  await waitForSessionState(apiClient, {
    taskId: task.id,
    sessionId: task.session_id,
    expectedState: "WAITING_FOR_INPUT",
    message: "the mock agent's turn ends after the simple message",
    timeout: 30_000,
  });
  await apiClient.mockGitHubAssociateTaskPR({
    workspace_id: seedData.workspaceId,
    task_id: task.id,
    repository_id: seedData.repositoryId,
    owner: PR_OWNER,
    repo: PR_REPO,
    pr_number: 601,
    pr_url: `https://github.com/${PR_OWNER}/${PR_REPO}/pull/601`,
    pr_title: "Ready to merge fixture",
    head_branch: "feature/ready-send-back",
    base_branch: "main",
    author_login: "coordinator-e2e",
    state: "open",
    review_state: "approved",
    checks_state: "success",
    mergeable_state: "clean",
    unresolved_review_threads: 0,
  });
  await expect
    .poll(async () => (await apiClient.listTaskPRs(task.id)).length, { timeout: 15_000 })
    .toBe(1);
  return {
    release,
    task,
    sessionId: task.session_id,
    coordinatorId: coordinator.id,
    prUrl: `https://github.com/${PR_OWNER}/${PR_REPO}/pull/601`,
  };
}
