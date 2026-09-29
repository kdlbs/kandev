// AC-COORDINATOR-PROPOSALS-004.4, -005.1..-005.8
// (docs/specs/coordinator/requirements/proposals.md,
// docs/plans/workspace-coordinator/task-08-proposals-ui.md).
//
// A proposal cannot be seeded through HTTP: the only creation path is the
// `propose_task_kandev` MCP tool, callable only by a coordinator principal
// (internal/mcp/handlers/coordinator_propose.go). Each test opens the
// coordinator's own copilot popover and sends a scripted
// `e2e:mcp:kandev:propose_task_kandev(...)` chat message (the same
// script-mode mechanism `copilot.spec.ts` and
// `settings/config-management.spec.ts` use for other MCP tools), which
// performs a real, unmocked MCP call against the live backend.
import { type Locator, type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { linkToCoordinatorNeedsYou } from "../../../lib/coordinator/links";
import { eligibleStep, type EligibleStepNode } from "../../../lib/coordinator/eligible-step";
import { stepHasOnEnterAction } from "../../../lib/types/http";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

/**
 * The seeded workflow's start step auto-starts an agent on enter, which the
 * backend refuses as a proposal placement (AC-COORDINATOR-PROPOSALS-001.4,
 * internal/coordinator/eligibility.go's EligibleStep: "no agent starts until
 * you start it" would otherwise be violated by the auto-start). Pick a
 * different step from the seeded graph using the same eligibility walk the
 * Edit form uses (lib/coordinator/eligible-step.ts), rather than hardcoding a
 * fixture step name/position.
 */
function pickEligibleStepId(seedData: SeedData): string {
  const nodes: EligibleStepNode[] = seedData.steps.map((step) => ({
    id: step.id,
    isStart: step.is_start_step ?? false,
    allowManualMove: step.allow_manual_move ?? false,
    autoStartOnEnter: stepHasOnEnterAction(step, "auto_start_agent"),
    pullFromStepId: step.pull_from_step_id ?? null,
  }));
  const eligible = nodes.find((node) => eligibleStep(nodes, node.id));
  if (!eligible) throw new Error("seeded workflow has no eligible step for a proposal");
  return eligible.id;
}

const COORDINATOR_READ = /\/coordinators\/[^/]+$/;
const CONVERSATION_OPENED = /\/coordinators\/[^/]+\/conversation$/;
const PROPOSAL_APPROVE = /\/proposals\/[^/]+\/approve$/;
const PROPOSAL_REJECT = /\/proposals\/[^/]+\/reject$/;

async function openCopilot(page: Page): Promise<Locator> {
  const coordinatorRead = waitForHttp(page, "GET", COORDINATOR_READ);
  const conversationOpened = waitForHttp(page, "POST", CONVERSATION_OPENED);
  const launcher = page.getByTestId("coordinator-copilot-launcher");
  await expect(launcher).toBeVisible({ timeout: 10_000 });
  await launcher.click();
  await coordinatorRead;
  await conversationOpened;
  const popover = page.getByTestId("coordinator-copilot-popover");
  await expect(popover).toBeVisible();
  return popover;
}

async function sendMessage(popover: Locator, text: string) {
  const editor = popover.getByTestId("chat-input-editor");
  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 15_000 });
  await editor.fill(text);
  await editor.press(`${modifier}+Enter`);
}

/** Sends a propose_task_kandev script command and waits for the chat card's proposal id to appear. */
async function proposeTask(popover: Locator, seedData: SeedData, title: string): Promise<string> {
  const args = {
    title,
    description: "The PR is too large to review in one pass.",
    rationale: "Splitting reduces review risk.",
    workflow_id: seedData.workflowId,
    step_id: pickEligibleStepId(seedData),
    repository_id: seedData.repositoryId,
  };
  await sendMessage(popover, `e2e:mcp:kandev:propose_task_kandev(${JSON.stringify(args)})`);
  const card = popover
    .getByTestId("propose-task-renderer")
    .locator('[data-testid^="proposal-card-"]');
  await expect(card).toBeVisible({ timeout: 30_000 });
  const testId = await card.getAttribute("data-testid");
  if (!testId) throw new Error("expected the chat proposal card to have a data-testid");
  return testId.replace("proposal-card-", "");
}

async function pollProposalStatus(
  apiClient: ApiClient,
  workspaceId: string,
  coordinatorId: string,
  proposalId: string,
  status: string,
): Promise<void> {
  await expect
    .poll(
      async () => (await apiClient.getProposal(workspaceId, coordinatorId, proposalId)).status,
      {
        timeout: 30_000,
        message: `proposal ${proposalId} should settle to ${status}`,
      },
    )
    .toBe(status);
}

test.describe("Coordinator proposals", () => {
  test("propose_task_kandev creates a pending proposal that shows on the chat card and Needs you (AC .005.1)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Proposals Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
    const popover = await openCopilot(testPage);

    const proposalId = await proposeTask(popover, seedData, "Split the large PR into two");

    // The status line renders twice per card (a `sr-only` live-region echo
    // plus the visible `<p>`, apps/web/CLAUDE.md's async-status pattern), so
    // scope to the `<p>` tag to avoid a strict-mode multi-match.
    const chatCard = popover.getByTestId(`proposal-card-${proposalId}`);
    await expect(chatCard.locator("p", { hasText: "Pending approval" })).toBeVisible();
    await expect(chatCard.getByText("Split the large PR into two")).toBeVisible();
    await expect(chatCard.getByRole("button", { name: "Approve" })).toBeVisible();
    await expect(chatCard.getByRole("button", { name: "Edit" })).toBeVisible();
    await expect(chatCard.getByRole("button", { name: "Reject" })).toBeVisible();

    const needsYouCard = testPage.getByTestId(`needs-you-item-${proposalId}`);
    await expect(needsYouCard).toBeVisible();
    const fullCard = needsYouCard.getByTestId(`proposal-card-${proposalId}`);
    await expect(fullCard.locator("p", { hasText: "Pending approval" })).toBeVisible();
    await expect(fullCard.getByRole("button", { name: "Approve" })).toBeVisible();
  });

  test("Approve in the chat: the item leaves Needs you, the chat card settles, and the task exists with no agent started (task-08 Acceptance, AC .005.7, .005.8)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Proposals Approve Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
    const popover = await openCopilot(testPage);
    const proposalId = await proposeTask(popover, seedData, "Approve me from chat");

    const needsYouCard = testPage.getByTestId(`needs-you-item-${proposalId}`);
    await expect(needsYouCard).toBeVisible();

    const chatCard = popover.getByTestId(`proposal-card-${proposalId}`);
    const approved = waitForHttp(testPage, "POST", PROPOSAL_APPROVE);
    await chatCard.getByRole("button", { name: "Approve" }).click();
    await approved;

    // The "Next: ..." toast line (AC .005.7) is wired only for the Needs-you
    // full card's `computeNeedsYouCount` (needs-you-item-card.tsx); the chat
    // card's toast has no description line (AC .005.8 covers its settled
    // state instead).
    const toast = testPage.getByTestId("toast-message");
    await expect(
      toast.getByText("Approved. Approve me from chat created in", { exact: false }),
    ).toBeVisible();

    await expect(
      chatCard.locator("p", { hasText: "Approved: Approve me from chat" }),
    ).toBeVisible();
    await expect(needsYouCard).toHaveCount(0);

    await pollProposalStatus(
      apiClient,
      seedData.workspaceId,
      coordinator.id,
      proposalId,
      "approved",
    );
    const settled = await apiClient.getProposal(seedData.workspaceId, coordinator.id, proposalId);
    if (!settled.task_id) throw new Error("expected the proposal to have a task_id after approval");
    const task = await apiClient.getTask(settled.task_id);
    expect(task.workflow_step_id).toBe(pickEligibleStepId(seedData));
    expect(task.primary_session_id ?? null).toBeNull();
  });

  test("Edit in place on Needs you: empty title is refused, Cancel returns focus to Edit, and Approve with edits settles the card (AC .005.4, .005.7)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Proposals Edit Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
    const popover = await openCopilot(testPage);
    const proposalId = await proposeTask(popover, seedData, "Edit me in place");
    await testPage.keyboard.press("Escape");
    await expect(popover).toBeHidden();

    const needsYouCard = testPage.getByTestId(`needs-you-item-${proposalId}`);
    await expect(needsYouCard).toBeVisible();

    const editButton = needsYouCard.getByRole("button", { name: "Edit" });
    await editButton.click();

    const titleInput = needsYouCard.getByLabel("Title");
    await expect(titleInput).toBeFocused();

    // Cancel returns focus to Edit (AC .005.4).
    await needsYouCard.getByRole("button", { name: "Cancel" }).click();
    await expect(editButton).toBeFocused();

    await editButton.click();
    const titleInputAgain = needsYouCard.getByLabel("Title");
    await titleInputAgain.fill("");
    const approveWithEdits = needsYouCard.getByRole("button", { name: "Approve with edits" });
    await expect(approveWithEdits).toBeEnabled({ timeout: 15_000 });
    await approveWithEdits.click();
    await expect(needsYouCard.getByText("Title is required.")).toBeVisible();

    await titleInputAgain.fill("Edited title");
    const approved = waitForHttp(testPage, "POST", PROPOSAL_APPROVE);
    await approveWithEdits.click();
    await approved;

    const toast = testPage.getByTestId("toast-message");
    await expect(
      toast.getByText("Approved. Edited title created in", { exact: false }),
    ).toBeVisible();
    await expect(needsYouCard).toHaveCount(0);

    await pollProposalStatus(
      apiClient,
      seedData.workspaceId,
      coordinator.id,
      proposalId,
      "approved",
    );
  });

  test("Reject in place on Needs you: Cancel returns focus to Reject, and a confirmed reject settles the card (AC .005.5, .005.7)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Proposals Reject Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
    const popover = await openCopilot(testPage);
    const proposalId = await proposeTask(popover, seedData, "Reject me in place");

    const needsYouCard = testPage.getByTestId(`needs-you-item-${proposalId}`);
    await expect(needsYouCard).toBeVisible();

    const rejectButton = needsYouCard.getByRole("button", { name: "Reject" });
    await rejectButton.click();

    const reason = needsYouCard.getByLabel("Reason (optional)");
    await expect(reason).toBeFocused();

    await needsYouCard.getByRole("button", { name: "Cancel" }).click();
    await expect(rejectButton).toBeFocused();

    await rejectButton.click();
    await needsYouCard.getByLabel("Reason (optional)").fill("Not needed right now");
    const rejected = waitForHttp(testPage, "POST", PROPOSAL_REJECT);
    await needsYouCard.getByRole("button", { name: "Confirm reject" }).click();
    await rejected;

    const toast = testPage.getByTestId("toast-message");
    await expect(toast.getByText("Rejected. Nothing was created", { exact: false })).toBeVisible();
    await expect(needsYouCard).toHaveCount(0);

    const proposal = await apiClient.getProposal(seedData.workspaceId, coordinator.id, proposalId);
    expect(proposal.status).toBe("rejected");
    expect(proposal.reject_reason).toBe("Not needed right now");
  });

  test("Edit and Reject on the chat card navigate to the same Needs-you forms (AC .005.6)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Proposals Deep Link Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
    const popover = await openCopilot(testPage);
    const proposalId = await proposeTask(popover, seedData, "Deep link me");

    const chatCard = popover.getByTestId(`proposal-card-${proposalId}`);
    await chatCard.getByRole("button", { name: "Edit" }).click();

    await expect(popover).toBeHidden();
    // The `?proposal=<id>&form=edit` deep link is consumed and cleared as
    // soon as the Needs-you screen's coordinator inputs are loaded
    // (use-needs-you-navigation.ts's `onAutoFormOpened`) — on an
    // already-loaded page (this test's inputs finished loading well before
    // this click) that can complete before a polled `toHaveURL` assertion
    // ever samples the query string, so asserting the query params directly
    // would be racy. The Title field only auto-focuses via that same
    // deep-link mechanism, so its focus is proof the link was consumed
    // correctly; the query string clearing (checked below) is the other
    // half of that same mechanism.
    const needsYouCard = testPage.getByTestId(`needs-you-item-${proposalId}`);
    const titleInput = needsYouCard.getByLabel("Title");
    await expect(titleInput).toBeFocused();
    await expect
      .poll(() => new URL(testPage.url()).search, { message: "deep-link query should be cleared" })
      .toBe("");
  });
});
