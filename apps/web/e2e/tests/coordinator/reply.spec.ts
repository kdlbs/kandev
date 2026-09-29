// Reply with a condition (docs/specs/coordinator/system-design/relay.md
// "Reply with a condition"): a manager returns a pending proposal with a
// condition, the coordinator's conversation receives it once, and the held
// Needs you item shows the returned state.
import { type Locator, type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { linkToCoordinatorNeedsYou } from "../../../lib/coordinator/links";
import { eligibleStep, type EligibleStepNode } from "../../../lib/coordinator/eligible-step";
import { stepHasOnEnterAction } from "../../../lib/types/http";
import type { SeedData } from "../../fixtures/test-base";

const COORDINATOR_READ = /\/coordinators\/[^/]+$/;
const CONVERSATION_OPENED = /\/coordinators\/[^/]+\/conversation$/;
const PROPOSAL_REPLY = /\/proposals\/[^/]+\/reply$/;
const CONDITION = "Split it into two smaller cards first.";

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

async function openCopilot(page: Page): Promise<{ popover: Locator; sessionId: string }> {
  const coordinatorRead = waitForHttp(page, "GET", COORDINATOR_READ);
  const conversationOpened = waitForHttp(page, "POST", CONVERSATION_OPENED);
  const launcher = page.getByTestId("coordinator-copilot-launcher");
  await expect(launcher).toBeVisible({ timeout: 10_000 });
  await launcher.click();
  await coordinatorRead;
  const opened = (await (await conversationOpened).json()) as { session_id: string };
  const popover = page.getByTestId("coordinator-copilot-popover");
  await expect(popover).toBeVisible();
  return { popover, sessionId: opened.session_id };
}

async function proposeTask(popover: Locator, seedData: SeedData, title: string): Promise<string> {
  const args = {
    title,
    description: "The PR is too large to review in one pass.",
    rationale: "Splitting reduces review risk.",
    workflow_id: seedData.workflowId,
    step_id: pickEligibleStepId(seedData),
    repository_id: seedData.repositoryId,
  };
  const editor = popover.getByTestId("chat-input-editor");
  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 15_000 });
  await editor.fill(`e2e:mcp:kandev:propose_task_kandev(${JSON.stringify(args)})`);
  await editor.press(`${modifier}+Enter`);
  const card = popover
    .getByTestId("propose-task-renderer")
    .locator('[data-testid^="proposal-card-"]');
  await expect(card).toBeVisible({ timeout: 30_000 });
  const testId = await card.getAttribute("data-testid");
  if (!testId) throw new Error("expected the chat proposal card to have a data-testid");
  return testId.replace("proposal-card-", "");
}

test.describe("Coordinator reply with a condition", () => {
  test("replying on the Needs you item returns the proposal, delivers once and holds the item", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Reply Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
    const { popover, sessionId } = await openCopilot(testPage);
    const proposalId = await proposeTask(popover, seedData, "Reply to me");

    await testPage.getByTestId("coordinator-copilot-popover-backdrop").click();
    await expect(popover).toBeHidden();

    const item = testPage.getByTestId(`needs-you-item-${proposalId}`);
    await expect(item).toBeVisible();
    const card = item.getByTestId(`proposal-card-${proposalId}`);
    await card.getByRole("button", { name: "Reply with a condition" }).click();

    const send = card.getByRole("button", { name: "Send reply" });
    await expect(send).toBeDisabled();
    await card.getByLabel("Your condition").fill(CONDITION);
    await expect(card.getByTestId("proposal-reply-counter")).toHaveText(
      `${CONDITION.length} of 2000`,
    );
    const replied = waitForHttp(testPage, "POST", PROPOSAL_REPLY);
    await send.click();
    await replied;

    await expect(
      card.locator("p", { hasText: `Returned with your condition: ${CONDITION}` }),
    ).toBeVisible();
    await expect(item).toBeVisible();
    await expect(card.getByRole("button", { name: "Send again" })).toHaveCount(0);

    await expect
      .poll(
        async () => {
          const row = await apiClient.getProposal(seedData.workspaceId, coordinator.id, proposalId);
          return { status: row.status, delivered: Boolean(row.reply_delivered_at) };
        },
        { message: "the reply should be recorded as returned and delivered" },
      )
      .toEqual({ status: "returned", delivered: true });

    await expect
      .poll(
        async () =>
          (await apiClient.listSessionMessages(sessionId)).messages.filter(
            (message) => message.author_type === "user" && message.content.includes(CONDITION),
          ).length,
        {
          timeout: 30_000,
          message: "the condition should reach the coordinator conversation once",
        },
      )
      .toBe(1);

    await testPage.getByTestId("coordinator-copilot-launcher").click();
    await expect(popover).toBeVisible();
    const chatCard = popover.getByTestId(`proposal-card-${proposalId}`);
    await expect(
      chatCard.locator("p", { hasText: `Returned with your condition: ${CONDITION}` }),
    ).toBeVisible();
  });
});
