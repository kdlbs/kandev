// Shared by answer-proposals.spec.ts and mobile-answer-proposals.spec.ts: seeds a
// coordinator, a task blocked on a clarification, and has the mock agent propose
// an answer through the copilot chat (a proposal cannot be seeded over HTTP).
import type { Locator } from "@playwright/test";
import { expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { waitForSessionState } from "../../helpers/session";
import { linkToCoordinatorNeedsYou } from "../../../lib/coordinator/links";
import type { test } from "../../fixtures/test-base";

export const ANSWER_DRAFT_TEXT = "PostgreSQL, the team already runs it.";
export const ANSWER_RATIONALE = "The architecture doc standardises on PostgreSQL.";
export const ANSWER_SOURCE_URL = "https://example.com/architecture";

export async function setupAnswerProposal(
  submit: (popover: Locator, editor: Locator) => Promise<void>,
  ctx: Pick<
    Parameters<Parameters<typeof test>[2]>[0],
    "testPage" | "apiClient" | "backend" | "seedData"
  >,
) {
  const { testPage, apiClient, backend, seedData } = ctx;
  const release = await backend.useEnv({
    KANDEV_FEATURES_COORDINATOR: "true",
    KANDEV_FEATURES_COORDINATOR_PHASE2: "true",
    KANDEV_FEATURES_COORDINATOR_PHASE3: "true",
    KANDEV_FEATURES_COORDINATOR_ANSWER: "true",
  });
  const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
    name: "Answer UI Coordinator",
    agent_profile_id: seedData.agentProfileId,
    executor_profile_id: seedData.worktreeExecutorProfileId,
    task_agent_profile_id: seedData.agentProfileId,
    task_executor_profile_id: seedData.worktreeExecutorProfileId,
  });
  const workspacePath = `/api/v1/workspaces/${seedData.workspaceId}/coordinators/${coordinator.id}`;
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Task with a question",
    seedData.agentProfileId,
    {
      description: "/e2e:clarification",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("expected an active session for the question task");
  await waitForSessionState(apiClient, {
    taskId: task.id,
    sessionId: task.session_id,
    expectedState: "WAITING_FOR_INPUT",
    message: "the task's agent is blocked on its question before the answer is proposed",
    timeout: 60_000,
  });
  const relay = await apiClient.rawRequest("GET", `${workspacePath}/relay/${task.id}`);
  expect(relay.status).toBe(200);
  const pendingId = ((await relay.json()) as { clarification: { pending_id: string } })
    .clarification.pending_id;

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
  const args = {
    task_id: task.id,
    target: "clarification",
    pending_id: pendingId,
    confidence: "med",
    needs_decision: true,
    draft: { answers: [{ question_id: "db", custom_text: ANSWER_DRAFT_TEXT }] },
    sources: [{ ref: ANSWER_SOURCE_URL, label: "Architecture doc" }],
    rationale: ANSWER_RATIONALE,
  };
  await editor.fill(`e2e:mcp:kandev:propose_answer_kandev(${JSON.stringify(args)})`);
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
        const body = (await listed.json()) as { proposals: Array<{ id: string; kind: string }> };
        proposalId = body.proposals.find((p) => p.kind === "answer")?.id ?? "";
        return proposalId;
      },
      { timeout: 30_000, message: "the answer proposal should be stored" },
    )
    .not.toBe("");

  await popover.getByRole("button", { name: "Close" }).focus();
  await testPage.keyboard.press("Escape");
  await expect(popover).toBeHidden();
  return { release, task, proposalId, workspacePath };
}
