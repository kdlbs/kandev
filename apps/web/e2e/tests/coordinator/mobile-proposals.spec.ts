// AC-COORDINATOR-PROPOSALS-005.9: proposal card actions and forms on a
// phone-sized viewport (docs/specs/coordinator/requirements/proposals.md).
// The `mobile-chrome` Playwright project matches this filename
// (apps/web/e2e/playwright.config.ts) and applies the Pixel 5 emulation the
// assertions below rely on, per the work order's note that the 390px
// assertions live in their own file rather than a rerun of
// proposals.spec.ts under a different project.
//
// A proposal cannot be seeded through HTTP; it is created through the
// coordinator's own copilot chat via the `e2e:mcp:kandev:propose_task_kandev`
// script-mode mechanism (see proposals.spec.ts for the full explanation).
import { type Locator, type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { linkToCoordinatorNeedsYou } from "../../../lib/coordinator/links";
import { eligibleStep, type EligibleStepNode } from "../../../lib/coordinator/eligible-step";
import { stepHasOnEnterAction } from "../../../lib/types/http";
import type { SeedData } from "../../fixtures/test-base";

/**
 * The seeded workflow's start step auto-starts an agent on enter, which the
 * backend refuses as a proposal placement (AC-COORDINATOR-PROPOSALS-001.4).
 * Pick a different step from the seeded graph using the same eligibility
 * walk the Edit form uses (lib/coordinator/eligible-step.ts), rather than
 * hardcoding a fixture step name/position.
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

const MIN_TOUCH_TARGET_PX = 44;
const COORDINATOR_READ = /\/coordinators\/[^/]+$/;
const CONVERSATION_OPENED = /\/coordinators\/[^/]+\/conversation$/;

async function expectNoHorizontalScroll(page: Page): Promise<void> {
  const overflow = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(overflow.scrollWidth).toBeLessThanOrEqual(overflow.clientWidth);
}

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

// On a coarse-pointer device Enter inserts a newline instead of submitting
// (see e2e/tests/chat/mobile-clarification.spec.ts); the send affordance is
// the inline "Send" button, not the desktop Meta/Control+Enter shortcut.
async function sendMessage(popover: Locator, text: string) {
  const editor = popover.getByTestId("chat-input-editor");
  await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 15_000 });
  await editor.fill(text);
  await popover.getByTestId("submit-message-button").tap();
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
  await sendMessage(popover, `e2e:mcp:kandev:propose_task_kandev(${JSON.stringify(args)})`);
  const card = popover
    .getByTestId("propose-task-renderer")
    .locator('[data-testid^="proposal-card-"]');
  await expect(card).toBeVisible({ timeout: 30_000 });
  const testId = await card.getAttribute("data-testid");
  if (!testId) throw new Error("expected the chat proposal card to have a data-testid");
  return testId.replace("proposal-card-", "");
}

test.describe("Coordinator proposal card on a phone viewport", () => {
  test("Pending card actions stack with 44px touch targets and no horizontal scroll (AC .005.9)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Mobile Proposals Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
      task_agent_profile_id: seedData.agentProfileId,
      task_executor_profile_id: seedData.worktreeExecutorProfileId,
    });

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
    const popover = await openCopilot(testPage);
    const proposalId = await proposeTask(popover, seedData, "Mobile pending proposal");
    // Escape closes the panel only while focus is inside it.
    await popover.getByRole("button", { name: "Close" }).focus();
    await testPage.keyboard.press("Escape");
    await expect(popover).toBeHidden();

    const needsYouCard = testPage.getByTestId(`needs-you-item-${proposalId}`);
    await expect(needsYouCard).toBeVisible();

    const approve = needsYouCard.getByRole("button", { name: "Approve" });
    const edit = needsYouCard.getByRole("button", { name: "Edit" });
    const reject = needsYouCard.getByRole("button", { name: "Reject" });

    const approveBox = await approve.boundingBox();
    const editBox = await edit.boundingBox();
    const rejectBox = await reject.boundingBox();
    expect(approveBox, "Approve should have a box").not.toBeNull();
    expect(editBox, "Edit should have a box").not.toBeNull();
    expect(rejectBox, "Reject should have a box").not.toBeNull();

    for (const box of [approveBox, editBox, rejectBox]) {
      expect(box!.height).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
    }

    await expectNoHorizontalScroll(testPage);
  });

  test("The Edit form's fields and actions stack in one column with no horizontal scroll (AC .005.9)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Mobile Proposals Edit Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
      task_agent_profile_id: seedData.agentProfileId,
      task_executor_profile_id: seedData.worktreeExecutorProfileId,
    });

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
    const popover = await openCopilot(testPage);
    const proposalId = await proposeTask(popover, seedData, "Mobile edit proposal");
    // Escape closes the panel only while focus is inside it.
    await popover.getByRole("button", { name: "Close" }).focus();
    await testPage.keyboard.press("Escape");
    await expect(popover).toBeHidden();

    const needsYouCard = testPage.getByTestId(`needs-you-item-${proposalId}`);
    await needsYouCard.getByRole("button", { name: "Edit" }).click();

    const titleField = needsYouCard.getByLabel("Title");
    const descriptionField = needsYouCard.getByLabel("Description");
    await expect(titleField).toBeVisible();
    await expect(descriptionField).toBeVisible();

    const titleBox = await titleField.boundingBox();
    const descriptionBox = await descriptionField.boundingBox();
    expect(titleBox, "Title field should have a box").not.toBeNull();
    expect(descriptionBox, "Description field should have a box").not.toBeNull();
    // One column: the description field sits below the title field rather
    // than beside it.
    expect(descriptionBox!.y).toBeGreaterThanOrEqual(titleBox!.y + titleBox!.height);

    const approveWithEdits = needsYouCard.getByRole("button", { name: "Approve with edits" });
    const cancel = needsYouCard.getByRole("button", { name: "Cancel" });
    const approveBox = await approveWithEdits.boundingBox();
    const cancelBox = await cancel.boundingBox();
    expect(approveBox, "Approve with edits should have a box").not.toBeNull();
    expect(cancelBox, "Cancel should have a box").not.toBeNull();
    expect(approveBox!.height).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
    expect(cancelBox!.height).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);

    await expectNoHorizontalScroll(testPage);
  });

  test("The Reject form's reason field and actions have no horizontal scroll and 44px touch targets (AC .005.9)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Mobile Proposals Reject Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
      task_agent_profile_id: seedData.agentProfileId,
      task_executor_profile_id: seedData.worktreeExecutorProfileId,
    });

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
    const popover = await openCopilot(testPage);
    const proposalId = await proposeTask(popover, seedData, "Mobile reject proposal");
    // Escape closes the panel only while focus is inside it.
    await popover.getByRole("button", { name: "Close" }).focus();
    await testPage.keyboard.press("Escape");
    await expect(popover).toBeHidden();

    const needsYouCard = testPage.getByTestId(`needs-you-item-${proposalId}`);
    await needsYouCard.getByRole("button", { name: "Reject" }).click();

    const reason = needsYouCard.getByLabel("Reason (optional)");
    await expect(reason).toBeVisible();

    const confirmReject = needsYouCard.getByRole("button", { name: "Confirm reject" });
    const cancel = needsYouCard.getByRole("button", { name: "Cancel" });
    const confirmBox = await confirmReject.boundingBox();
    const cancelBox = await cancel.boundingBox();
    expect(confirmBox, "Confirm reject should have a box").not.toBeNull();
    expect(cancelBox, "Cancel should have a box").not.toBeNull();
    expect(confirmBox!.height).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
    expect(cancelBox!.height).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);

    await expectNoHorizontalScroll(testPage);
  });
});
