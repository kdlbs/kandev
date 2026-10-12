// The Queue's "What it did" section: an approved proposal lists as a row, Undo
// archives the task and the row reads "Undone by", and the class filter lives in
// the address (docs/specs/coordinator/requirements/activity-log.md).
//
// A proposal cannot be seeded through HTTP; it is created through the
// coordinator's own copilot chat with the `e2e:mcp:kandev:propose_task_kandev`
// script (see proposals.spec.ts).
import { type Locator, type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { linkToCoordinatorQueue } from "../../../lib/coordinator/links";
import { eligibleStep, type EligibleStepNode } from "../../../lib/coordinator/eligible-step";
import { stepHasOnEnterAction } from "../../../lib/types/http";
import type { SeedData } from "../../fixtures/test-base";

const COORDINATOR_READ = /\/coordinators\/[^/]+$/;
const CONVERSATION_OPENED = /\/coordinators\/[^/]+\/conversation$/;
const PROPOSAL_APPROVE = /\/proposals\/[^/]+\/approve$/;
const ACTIVITY_UNDO = /\/coordinators\/[^/]+\/activity\/[^/]+\/undo$/;

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

async function proposeAndApprove(page: Page, seedData: SeedData, title: string) {
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
  const card = popover
    .getByTestId("propose-task-renderer")
    .locator('[data-testid^="proposal-card-"]');
  await expect(card).toBeVisible({ timeout: 30_000 });
  const approved = waitForHttp(page, "POST", PROPOSAL_APPROVE);
  await card.getByRole("button", { name: "Approve" }).click();
  await approved;
  await popover.getByRole("button", { name: "Close", exact: true }).click();
  await expect(popover).toBeHidden();
}

test.describe("Coordinator What it did", () => {
  test("an approved proposal lists, Undo archives the task, and the class filter lives in the address", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Activity Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });

    await testPage.goto(linkToCoordinatorQueue(seedData.workspaceId, coordinator.id));
    const section = testPage.getByTestId("what-it-did");
    await expect(section).toBeVisible();
    await expect(testPage.getByTestId("activity-empty")).toBeVisible();

    await proposeAndApprove(testPage, seedData, "Undo me from the log");

    const firstRow = section.locator('[data-testid^="activity-row-"]').first();
    await expect(firstRow).toBeVisible();
    const rowTestId = await firstRow.getAttribute("data-testid");
    if (!rowTestId) throw new Error("expected the activity row to have a data-testid");
    const row = section.getByTestId(rowTestId);
    await expect(row).toContainText("Undo me from the log");
    await expect(row.getByTestId("activity-class")).toHaveText("Create task");
    await expect(row.getByTestId("activity-authorization")).toContainText("Approved");

    const undone = waitForHttp(testPage, "POST", ACTIVITY_UNDO);
    await row.getByRole("button", { name: "Undo" }).click();
    await testPage.getByTestId("activity-undo-confirm").click();
    await undone;
    await expect(row.getByTestId("activity-undo-cell")).toContainText("Undone");
    await expect(row.getByRole("button", { name: "Undo" })).toHaveCount(0);

    await section.getByTestId("activity-filter").click();
    await testPage.getByRole("option", { name: "Move task" }).click();
    await expect(testPage).toHaveURL(/class=move/);
    await expect(testPage.getByTestId("activity-empty")).toHaveText("Nothing matches this filter.");
  });
});
