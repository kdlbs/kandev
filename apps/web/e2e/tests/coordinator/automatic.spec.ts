// The first automatic class (docs/plans/workspace-coordinator-p3/task-09-automatic.md):
// a coordinator with an earned create_task record is raised to automatic, its
// next proposal is approved without a click, and undoing the created task
// lowers the class again. The 30-day history is seeded through the test seed
// route; the eligibility rules read it exactly as they read a real one.
import { type Locator, type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { eligibleStep, type EligibleStepNode } from "../../../lib/coordinator/eligible-step";
import { stepHasOnEnterAction } from "../../../lib/types/http";
import { linkToCoordinatorQueue, linkToCoordinatorSettings } from "../../../lib/coordinator/links";

const SETTINGS_PATH = /\/coordinators\/[^/]+\/settings$/;
const REVIEW_PATH = /\/coordinators\/[^/]+\/classes\/create_task\/reviews$/;
const COORDINATOR_READ = /\/coordinators\/[^/]+$/;
const CONVERSATION_OPENED = /\/coordinators\/[^/]+\/conversation$/;
const ACTIVITY_UNDO = /\/coordinators\/[^/]+\/activity\/[^/]+\/undo$/;
const AUTOMATIC_RADIO = "#may-do-create_task-automatic";
const APPROVAL_RADIO = "#may-do-create_task-approval";

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

async function propose(page: Page, seedData: SeedData, title: string): Promise<Locator> {
  const popover = await openCopilot(page);
  const args = {
    title,
    description: "Split the work so each part can be reviewed alone.",
    rationale: "Smaller changes are easier to review.",
    workflow_id: seedData.workflowId,
    step_id: pickEligibleStepId(seedData),
    repository_id: seedData.repositoryId,
  };
  const editor = popover.getByTestId("chat-input-editor");
  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 15_000 });
  await editor.fill(`e2e:mcp:kandev:propose_task_kandev(${JSON.stringify(args)})`);
  await editor.press(`${modifier}+Enter`);
  return popover;
}

test.describe("Coordinator automatic create_task", () => {
  test("raise, automatic approval, undo and automatic lowering", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Automatic Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });
    const seeded = await apiClient.rawRequest(
      "POST",
      `/api/v1/_test/coordinators/${coordinator.id}/seed-create-task-history`,
      { window_rows: 20, edited_rows: 0, oldest_days_ago: 31 },
    );
    expect(seeded.status).toBe(201);
    const settings = `${linkToCoordinatorSettings(seedData.workspaceId, coordinator.id)}?section=may-do`;

    // Every log condition is earned, the review is not: Automatic stays off.
    await testPage.goto(settings);
    await expect(testPage.getByTestId("automatic-condition-volume")).toHaveAttribute(
      "data-met",
      "true",
    );
    await expect(testPage.getByTestId("automatic-condition-reviewed_7d")).toHaveAttribute(
      "data-met",
      "false",
    );
    await expect(testPage.locator(AUTOMATIC_RADIO)).toBeDisabled();
    await expect(testPage.locator("#may-do-message-automatic")).toBeDisabled();
    await expect(testPage.getByTestId("may-do-automatic-note-message")).toHaveText(
      "Cannot be raised",
    );

    // Mark as reviewed is offered only after the filtered log was opened.
    await expect(testPage.getByTestId("automatic-mark-reviewed")).toBeDisabled();
    await testPage.getByTestId("may-do-review-create_task").click();
    await expect(testPage).toHaveURL(/\/queue\?class=create_task$/);
    await testPage.goto(settings);
    const reviewed = waitForHttp(testPage, "POST", REVIEW_PATH);
    await testPage.getByTestId("automatic-mark-reviewed").click();
    await reviewed;
    await expect(testPage.getByTestId("automatic-condition-reviewed_7d")).toHaveAttribute(
      "data-met",
      "true",
    );

    // Raise: choose Automatic and save.
    await expect(testPage.locator(AUTOMATIC_RADIO)).toBeEnabled();
    await testPage.locator(AUTOMATIC_RADIO).click();
    const saved = waitForHttp(testPage, "PUT", SETTINGS_PATH);
    await testPage.getByRole("button", { name: "Save changes" }).click();
    await saved;
    await expect(testPage.getByTestId("automatic-raised-record")).toContainText(
      "Raised to automatic",
    );

    // The next proposal is approved with no click and logged as automatic.
    await testPage.goto(linkToCoordinatorQueue(seedData.workspaceId, coordinator.id));
    const popover = await propose(testPage, seedData, "Approved by the automatic path");
    await expect(
      popover.getByTestId("propose-task-renderer").locator('[data-testid^="proposal-card-"]'),
    ).toBeVisible({ timeout: 30_000 });
    await expect(popover.getByRole("button", { name: "Approve" })).toHaveCount(0);
    await popover.getByRole("button", { name: "Close", exact: true }).click();

    await testPage.reload();
    const row = testPage
      .getByTestId("what-it-did")
      .locator('[data-testid^="activity-row-"]')
      .filter({ hasText: "Approved by the automatic path" })
      .first();
    await expect(row).toBeVisible({ timeout: 30_000 });
    await expect(row.getByTestId("activity-authorization")).toContainText("Automatic");

    // Undoing the automatic create lowers the class at once.
    const undone = waitForHttp(testPage, "POST", ACTIVITY_UNDO);
    await row.getByRole("button", { name: "Undo" }).click();
    await testPage.getByTestId("activity-undo-confirm").click();
    await undone;

    await testPage.goto(settings);
    await expect(testPage.locator(APPROVAL_RADIO)).toBeChecked();
    await expect(testPage.getByTestId("automatic-condition-no_undo")).toHaveAttribute(
      "data-met",
      "false",
    );
    await expect(testPage.locator(AUTOMATIC_RADIO)).toBeDisabled();
  });
});
