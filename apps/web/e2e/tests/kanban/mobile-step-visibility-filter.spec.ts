import type { Locator, Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import type { WorkflowStep } from "../../../lib/types/http";

const TASK_A1 = "Mobile Task A1";
const TASK_A2 = "Mobile Task A2";
const TASK_B1 = "Mobile Task B1";
const SECOND_STEP_A_TITLE = "Doing (A)";
// Mirrors ORPHAN_STEP_ID in apps/web/components/kanban/swimlane-kanban-content.tsx.
const ORPHAN_STEP_ID = "__kandev_orphan__";
const MIN_TOUCH_TARGET_PX = 44;

async function openMobileMenu(testPage: Page) {
  await testPage.getByRole("button", { name: "Open menu" }).click();
  await testPage.getByTestId("mobile-home-menu-card").waitFor({ state: "visible" });
}

// Escape is retried: closing the Columns dropdown and closing the drawer are
// two dismissals, and the second key press can land while the first layer is
// still tearing down, leaving the drawer open.
async function closeMobileMenu(testPage: Page) {
  const card = testPage.getByTestId("mobile-home-menu-card");
  await expect(async () => {
    if ((await card.count()) > 0) {
      await testPage.keyboard.press("Escape");
    }
    await expect(card).toHaveCount(0, { timeout: 1_000 });
  }).toPass({ timeout: 15_000 });
}

// The phone's Columns control is scoped to the workflow the board is focused
// on, so there is no group, no disclosure, and no eligibility set here — just
// one workflow's steps.
// See the desktop spec: a click landing while another dismissable layer is
// still tearing down is swallowed, so opening is retried to the same end state.
async function openColumnsMenu(testPage: Page, workflowId: string) {
  const trigger = testPage.getByTestId(`columns-menu-${workflowId}`);
  await expect(async () => {
    if ((await trigger.getAttribute("data-state")) !== "open") {
      await trigger.click();
    }
    await expect(trigger).toHaveAttribute("data-state", "open", { timeout: 1_000 });
  }).toPass({ timeout: 15_000 });
}

async function closeColumnsMenu(testPage: Page, workflowId: string) {
  const trigger = testPage.getByTestId(`columns-menu-${workflowId}`);
  if ((await trigger.getAttribute("data-state")) === "open") {
    await testPage.keyboard.press("Escape");
  }
  await expect(trigger).not.toHaveAttribute("data-state", "open");
}

async function toggleColumnsFromDrawer(testPage: Page, workflowId: string, stepIds: string[]) {
  await openMobileMenu(testPage);
  await openColumnsMenu(testPage, workflowId);
  for (const stepId of stepIds) {
    await testPage.getByTestId(`columns-menu-step-${stepId}`).click();
  }
  await closeColumnsMenu(testPage, workflowId);
  await closeMobileMenu(testPage);
}

// Retried until the box settles. Radix opens menu content with a
// `zoom-in-95` transform, so a box measured mid-animation reports ~95% of the
// laid-out height (44px reads as ~41.9) — a false negative about the element,
// not a real touch-target miss. The asserted end state is unchanged.
async function expectMinTouchTarget(locators: Locator[]) {
  for (const locator of locators) {
    await expect(async () => {
      const box = await locator.boundingBox();
      expect(box).not.toBeNull();
      if (!box) throw new Error("touch target is not laid out");
      expect(box.height).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
    }).toPass({ timeout: 5_000 });
  }
}

test.describe("Mobile Steps visibility filter", () => {
  let workflowAId: string | null = null;
  let workflowBId: string | null = null;
  let stepA1Id: string | null = null;
  let stepA2Id: string | null = null;
  let stepB1Id: string | null = null;
  // The full, ordered step list of each workflow (the seed fixture's default
  // template steps, plus the one added to A below) — used instead of a
  // hardcoded count/order, since the template's step count is not this
  // spec's concern to assume.
  let workflowASteps: WorkflowStep[] = [];

  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  test.beforeEach(async ({ apiClient, seedData, testPage }) => {
    workflowAId = seedData.workflowId;
    stepA1Id = seedData.startStepId;

    // A second step holding a second task, mirroring the desktop spec: hiding
    // the only populated step would zero workflow A's whole visible task
    // count and let the pre-existing "drop workflows with no visible tasks"
    // logic mask whether per-step column collapse actually works.
    const secondStepA = await apiClient.createWorkflowStep(
      seedData.workflowId,
      SECOND_STEP_A_TITLE,
      1,
    );
    stepA2Id = secondStepA.id;
    const stepsA = (await apiClient.listWorkflowSteps(seedData.workflowId)).steps;
    workflowASteps = [...stepsA].sort(
      (a, b) => a.position - b.position || a.id.localeCompare(b.id),
    );

    // A second workflow — mandatory per R2: with only one eligible workflow
    // the section renders inline under the single-workflow rule and no
    // disclosure header (and its >=44px requirement) is ever measured.
    const workflowB = await apiClient.createWorkflow(seedData.workspaceId, "Workflow B", "simple");
    workflowBId = workflowB.id;
    const stepsB = (await apiClient.listWorkflowSteps(workflowB.id)).steps;
    const startB = stepsB.find((s) => s.is_start_step) ?? stepsB[0];
    stepB1Id = startB.id;

    // "All Workflows" — the Steps section's eligible-workflow set follows the
    // same active Workflow filter as the board, and the default seeded
    // filter is scoped to workflow A alone. Without this, eligibleWorkflows
    // stays length 1 and the R2 disclosure header never renders.
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: "",
    });

    await apiClient.createTask(seedData.workspaceId, TASK_A1, {
      workflow_id: seedData.workflowId,
      workflow_step_id: stepA1Id,
    });
    await apiClient.createTask(seedData.workspaceId, TASK_A2, {
      workflow_id: seedData.workflowId,
      workflow_step_id: stepA2Id,
    });
    await apiClient.createTask(seedData.workspaceId, TASK_B1, {
      workflow_id: workflowB.id,
      workflow_step_id: startB.id,
    });
  });

  test.afterEach(async ({ apiClient, seedData }) => {
    if (workflowBId) {
      await apiClient.deleteWorkflow(workflowBId).catch(() => {});
      workflowBId = null;
    }
    if (stepA2Id) {
      await apiClient.deleteWorkflowStep(stepA2Id).catch(() => {});
      stepA2Id = null;
    }
    workflowAId = null;
    stepA1Id = null;
    stepB1Id = null;
    workflowASteps = [];
    // Clear hidden step IDs and restore the default (single-workflow) filter
    // — the phone board navigator is not exercised by these scenarios, but
    // resetting the workflow filter here keeps this suite's state hygiene
    // identical to the desktop spec's afterEach.
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: seedData.workflowId,
      repository_ids: [],
      kanban_hidden_step_ids: {},
    });
  });
  test("1. reachable from the topbar menu; toggling a column hides it and its task, and re-ticking restores them", async ({
    testPage,
  }) => {
    if (!workflowAId || !stepA1Id || !stepA2Id) throw new Error("workflow A ids not set");

    const kanban = new MobileKanbanPage(testPage);
    await kanban.goto();
    await expect(testPage.getByTestId(`kanban-column-${stepA1Id}`)).toHaveCount(1);

    await toggleColumnsFromDrawer(testPage, workflowAId, [stepA1Id]);

    // Every column is mounted inside SwipeableColumns and revealed one at a
    // time, so conformance is judged on COUNT, never on visibility.
    await expect(testPage.getByTestId(`kanban-column-${stepA1Id}`)).toHaveCount(0);
    await expect(testPage.getByTestId(`kanban-column-${stepA2Id}`)).toHaveCount(1);
    await expect(testPage.getByTestId(`kanban-column-${ORPHAN_STEP_ID}`)).toHaveCount(0);

    await toggleColumnsFromDrawer(testPage, workflowAId, [stepA1Id]);
    await expect(testPage.getByTestId(`kanban-column-${stepA1Id}`)).toHaveCount(1);
  });

  test("2. persists across reload, driven through the real UI", async ({ testPage }) => {
    if (!workflowAId || !stepA1Id) throw new Error("workflow A ids not set");

    const kanban = new MobileKanbanPage(testPage);
    await kanban.goto();

    // Toggled through the UI rather than seeded: seeding only proves hydration
    // reads the field back, never that the outbound write path works.
    await toggleColumnsFromDrawer(testPage, workflowAId, [stepA1Id]);
    await expect(testPage.getByTestId(`kanban-column-${stepA1Id}`)).toHaveCount(0);

    await testPage.reload();
    await testPage.getByTestId("mobile-kanban-layout").waitFor({ state: "visible" });

    await expect(testPage.getByTestId(`kanban-column-${stepA1Id}`)).toHaveCount(0);
    await expect(testPage.getByTestId(`kanban-column-${ORPHAN_STEP_ID}`)).toHaveCount(0);

    await openMobileMenu(testPage);
    await openColumnsMenu(testPage, workflowAId);
    await expect(testPage.getByTestId(`columns-menu-step-${stepA1Id}`)).toHaveAttribute(
      "aria-checked",
      "false",
    );
    await closeColumnsMenu(testPage, workflowAId);
    await closeMobileMenu(testPage);
  });

  test("3. the drawer trigger and every menu item clear the 44 CSS px touch target", async ({
    testPage,
  }) => {
    if (!workflowAId) throw new Error("workflowAId not set");

    const kanban = new MobileKanbanPage(testPage);
    await kanban.goto();
    await openMobileMenu(testPage);

    const trigger = testPage.getByTestId(`columns-menu-${workflowAId}`);
    await expect(trigger).toBeVisible();
    await expectMinTouchTarget([trigger]);

    await openColumnsMenu(testPage, workflowAId);
    const items = testPage.locator('[data-testid^="columns-menu-step-"]');
    const count = await items.count();
    expect(count).toBeGreaterThan(0);
    await expectMinTouchTarget(Array.from({ length: count }, (_unused, index) => items.nth(index)));

    await closeColumnsMenu(testPage, workflowAId);
    await closeMobileMenu(testPage);
  });

  test("4. the drawer with the Columns menu open causes no document-level horizontal overflow", async ({
    testPage,
  }) => {
    if (!workflowAId) throw new Error("workflowAId not set");

    const kanban = new MobileKanbanPage(testPage);
    await kanban.goto();
    await openMobileMenu(testPage);
    await openColumnsMenu(testPage, workflowAId);

    const overflow = await testPage.evaluate(() => ({
      scrollWidth: document.documentElement.scrollWidth,
      clientWidth: document.documentElement.clientWidth,
    }));
    expect(overflow.scrollWidth).toBeLessThanOrEqual(overflow.clientWidth);

    await closeColumnsMenu(testPage, workflowAId);
    await closeMobileMenu(testPage);
  });

  test("5. the drawer offers the focused workflow's steps only, never another workflow's", async ({
    testPage,
  }) => {
    if (!workflowAId || !workflowBId || !stepB1Id) throw new Error("workflow ids not set");

    const kanban = new MobileKanbanPage(testPage);
    await kanban.goto();
    await openMobileMenu(testPage);

    // One control, for the board actually on screen. Workflow B has its own
    // steps and its own hidden set, and neither is reachable from here.
    await expect(testPage.getByTestId(`columns-menu-${workflowAId}`)).toBeVisible();
    await expect(testPage.getByTestId(`columns-menu-${workflowBId}`)).toHaveCount(0);

    await openColumnsMenu(testPage, workflowAId);
    await expect(testPage.getByTestId(`columns-menu-step-${stepB1Id}`)).toHaveCount(0);
    for (const step of workflowASteps) {
      await expect(testPage.getByTestId(`columns-menu-step-${step.id}`)).toHaveCount(1);
    }

    await closeColumnsMenu(testPage, workflowAId);
    await closeMobileMenu(testPage);
  });
});
