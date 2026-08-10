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

async function closeMobileMenu(testPage: Page) {
  await testPage.keyboard.press("Escape");
  await expect(testPage.getByTestId("mobile-home-menu-card")).toHaveCount(0);
}

// R2: with a second workflow seeded, the Steps section shows a disclosure
// header per workflow group and starts collapsed by default (nothing hidden
// yet) — interacting with a collapsed group's steps matches nothing and
// passes vacuously, so scenarios expand first.
async function expandStepsGroup(testPage: Page, workflowId: string) {
  const toggle = testPage.getByTestId(`steps-filter-group-toggle-${workflowId}`);
  if ((await toggle.getAttribute("aria-expanded")) !== "true") {
    await toggle.click();
  }
  await expect(toggle).toHaveAttribute("aria-expanded", "true");
}

async function expectMinTouchTarget(locators: Locator[]) {
  for (const locator of locators) {
    const box = await locator.boundingBox();
    expect(box).not.toBeNull();
    if (!box) throw new Error("touch target is not laid out");
    expect(box.height).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
  }
}

/** Document-order check via `compareDocumentPosition`, per the R2 placement AC. */
async function isBefore(first: Locator, second: Locator): Promise<boolean> {
  const secondHandle = await second.elementHandle();
  if (!secondHandle) throw new Error("second locator did not resolve to an element");
  try {
    return await first.evaluate((firstEl, secondEl) => {
      const position = firstEl.compareDocumentPosition(secondEl as Node);
      return (position & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;
    }, secondHandle);
  } finally {
    await secondHandle.dispose();
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
  let workflowBSteps: WorkflowStep[] = [];

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
    workflowBSteps = [...stepsB].sort(
      (a, b) => a.position - b.position || a.id.localeCompare(b.id),
    );

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
    workflowBSteps = [];
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

  test("1. reachable from the topbar menu; toggling a step hides its column and task, and re-ticking restores them", async ({
    testPage,
    seedData,
  }) => {
    if (!workflowAId || !stepA1Id) throw new Error("fixture not set");
    const startStep = seedData.steps.find((s) => s.id === stepA1Id);
    if (!startStep) throw new Error("seeded start step not found");

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    await openMobileMenu(testPage);
    const menu = testPage.getByTestId("mobile-home-menu-card");

    // R2 placement AC: the Steps section sits after Repository and before
    // the Preview-panel field, mirroring the dropdown's own field order.
    const repositoryLabel = menu.getByText("Repository", { exact: true });
    const previewLabel = menu.getByText("Preview Panel", { exact: true });
    const stepsSection = menu.getByTestId("steps-filter-section");
    await expect(stepsSection).toBeVisible();
    expect(await isBefore(repositoryLabel, stepsSection)).toBe(true);
    expect(await isBefore(stepsSection, previewLabel)).toBe(true);

    await expandStepsGroup(testPage, workflowAId);
    await testPage.getByTestId(`steps-filter-step-${stepA1Id}`).click();
    await closeMobileMenu(testPage);

    await expect(testPage.getByTestId(`kanban-column-${stepA1Id}`)).toHaveCount(0);
    await expect(mobile.taskCardByTitle(TASK_A1)).toHaveCount(0);
    await expect(mobile.taskCardByTitle(TASK_A2)).toBeVisible();
    await expect(testPage.getByTestId(`kanban-column-${ORPHAN_STEP_ID}`)).toHaveCount(0);

    // The mobile board navigator no longer lists the hidden step by title.
    await mobile.boardNavigator.click();
    const navigatorDrawer = testPage.getByTestId("mobile-board-navigator-drawer");
    await expect(navigatorDrawer.getByText(startStep.name, { exact: true })).toHaveCount(0);
    await testPage.keyboard.press("Escape");
    await expect(navigatorDrawer).toHaveCount(0);

    // Re-ticking restores the column and its task. The group auto-expands
    // this visit because workflow A now holds a live hidden step.
    await openMobileMenu(testPage);
    await expandStepsGroup(testPage, workflowAId);
    await testPage.getByTestId(`steps-filter-step-${stepA1Id}`).click();
    await closeMobileMenu(testPage);

    await expect(testPage.getByTestId(`kanban-column-${stepA1Id}`)).toBeVisible();
    await expect(mobile.taskCardByTitle(TASK_A1)).toBeVisible();
  });

  test("2. persists across reload, driven through the real UI", async ({ testPage }) => {
    if (!workflowAId || !stepA1Id) throw new Error("fixture not set");

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    // Untick through the real UI — checkbox click -> normalize -> WS/REST
    // persist — rather than seeding via the API, so the outbound write path
    // is exercised.
    await openMobileMenu(testPage);
    await expandStepsGroup(testPage, workflowAId);
    await testPage.getByTestId(`steps-filter-step-${stepA1Id}`).click();
    await closeMobileMenu(testPage);

    await expect(testPage.getByTestId(`kanban-column-${stepA1Id}`)).toHaveCount(0);

    await testPage.reload();
    await mobile.board.waitFor({ state: "visible" });
    await testPage.getByTestId("mobile-kanban-layout").waitFor({ state: "visible" });

    await expect(testPage.getByTestId(`kanban-column-${stepA1Id}`)).toHaveCount(0);
    await expect(testPage.getByTestId(`kanban-column-${ORPHAN_STEP_ID}`)).toHaveCount(0);

    await openMobileMenu(testPage);
    await expandStepsGroup(testPage, workflowAId);
    await expect(testPage.getByTestId(`steps-filter-step-${stepA1Id}`)).not.toBeChecked();
    await closeMobileMenu(testPage);
  });

  test("3. every interactive row measures at least 44 CSS px — the group toggle and, after expanding, the step row", async ({
    testPage,
  }) => {
    if (!workflowAId) throw new Error("fixture not set");

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await openMobileMenu(testPage);

    const toggles = testPage.locator('[data-testid^="steps-filter-group-toggle-"]');
    await expect(toggles).toHaveCount(2);
    await expectMinTouchTarget(await toggles.all());

    await expandStepsGroup(testPage, workflowAId);
    const rows = testPage.locator('[data-testid^="steps-filter-step-row-"]');
    // Only workflow A is expanded — exactly its steps are revealed.
    await expect(rows).toHaveCount(workflowASteps.length);
    await expectMinTouchTarget(await rows.all());
  });

  test("4. the drawer with the Steps section open causes no document-level horizontal overflow", async ({
    testPage,
  }) => {
    if (!workflowAId) throw new Error("fixture not set");

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await openMobileMenu(testPage);
    await expandStepsGroup(testPage, workflowAId);

    const scrollWidth = await testPage.evaluate(() => document.documentElement.scrollWidth);
    const clientWidth = await testPage.evaluate(() => document.documentElement.clientWidth);
    expect(scrollWidth).toBeLessThanOrEqual(clientWidth);
  });

  test("5. a collapsed group hides its checkboxes but keeps its container and toggle present; expanding reveals its steps in position order", async ({
    testPage,
  }) => {
    if (!workflowAId || !stepA1Id || !stepA2Id) throw new Error("fixture not set");

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await openMobileMenu(testPage);

    // Nothing hidden yet — both groups start collapsed.
    await expect(testPage.getByTestId(`steps-filter-group-${workflowAId}`)).toBeVisible();
    await expect(testPage.getByTestId(`steps-filter-group-toggle-${workflowAId}`)).toHaveAttribute(
      "aria-expanded",
      "false",
    );
    await expect(testPage.getByTestId(`steps-filter-step-${stepA1Id}`)).toHaveCount(0);
    await expect(testPage.getByTestId(`steps-filter-step-${stepA2Id}`)).toHaveCount(0);

    await expandStepsGroup(testPage, workflowAId);

    const group = testPage.getByTestId(`steps-filter-group-${workflowAId}`);
    const rows = group.locator('[data-testid^="steps-filter-step-row-"]');
    await expect(rows).toHaveCount(workflowASteps.length);
    for (const [index, step] of workflowASteps.entries()) {
      await expect(rows.nth(index)).toHaveAttribute(
        "data-testid",
        `steps-filter-step-row-${step.id}`,
      );
    }
  });

  test("6. hiding every step of every seeded workflow renders zero columns on the phone board, no error and no empty-state message; re-ticking restores a column", async ({
    testPage,
  }) => {
    if (!workflowAId || !workflowBId || !stepA1Id || !stepA2Id || !stepB1Id) {
      throw new Error("fixture not set");
    }

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    // Genuinely globally empty (F24): every step of BOTH seeded workflows is
    // hidden, so `getVisibleWorkflows`'s every-workflow-empty branch fires
    // regardless of which workflow the mobile navigator currently focuses —
    // independent of whether the navigator itself writes the workflow filter.
    await openMobileMenu(testPage);
    await expandStepsGroup(testPage, workflowAId);
    for (const step of workflowASteps) {
      await testPage.getByTestId(`steps-filter-step-${step.id}`).click();
    }
    await expandStepsGroup(testPage, workflowBId);
    for (const step of workflowBSteps) {
      await testPage.getByTestId(`steps-filter-step-${step.id}`).click();
    }
    await closeMobileMenu(testPage);

    await expect(testPage.locator('[data-testid^="kanban-column-"]')).toHaveCount(0);
    await expect(testPage.getByText("No tasks yet")).toHaveCount(0);
    // No crash — the mobile layout is still rendered, not an error boundary.
    await expect(testPage.getByTestId("mobile-kanban-layout")).toBeVisible();

    // Re-ticking restores a column. The group auto-expands this visit
    // because workflow A now holds live hidden steps.
    await openMobileMenu(testPage);
    await expandStepsGroup(testPage, workflowAId);
    await testPage.getByTestId(`steps-filter-step-${stepA1Id}`).click();
    await closeMobileMenu(testPage);

    await expect(testPage.getByTestId(`kanban-column-${stepA1Id}`)).toBeVisible();
  });
});
