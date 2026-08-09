import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

const TASK_VISIBLE_TIMEOUT = 10_000;
const TASK_A = "Task in workflow A";
// Deliberately does NOT contain "Task in workflow A" as a substring —
// `taskCardByTitle` matches case-insensitively, and Playwright's toBeVisible()
// requires a locator to resolve to exactly one element, so an accidental
// substring collision with TASK_A would break these assertions with a strict-
// mode violation instead of a meaningful pass/fail.
const TASK_A_OTHER_STEP = "Workflow A doing-column card";
const TASK_B = "Task in workflow B";
// Mirrors ORPHAN_STEP_ID in apps/web/components/kanban/swimlane-kanban-content.tsx —
// the synthetic "Needs Reassignment" column a hidden step's task must NOT resurface in.
const ORPHAN_STEP_ID = "__kandev_orphan__";

async function openDisplayDropdown(kanban: KanbanPage) {
  const trigger = kanban.page.getByTestId("display-button");
  await trigger.click();
  await expect(trigger).toHaveAttribute("data-state", "open");
}

async function closeDisplayDropdown(kanban: KanbanPage) {
  const trigger = kanban.page.getByTestId("display-button");
  if ((await trigger.getAttribute("data-state")) === "open") {
    await trigger.click({ force: true });
  }
  await expect(trigger).not.toHaveAttribute("data-state", "open");
}

test.describe("Kanban step visibility filter", () => {
  let workflowBId: string | null = null;
  let startStepAId: string | null = null;
  let secondStepAId: string | null = null;
  let startStepBId: string | null = null;

  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  test.beforeEach(async ({ apiClient, seedData, testPage }) => {
    // Use the existing seed workflow A start step
    startStepAId = seedData.startStepId;

    // Workflow A needs a SECOND step holding a second task so that hiding the
    // start step does not zero out workflow A's whole task count — otherwise
    // the pre-existing "drop workflows with zero visible tasks" logic in
    // getVisibleWorkflows would remove the entire swimlane, and the test
    // could not tell that apart from the per-step column-collapse code
    // actually working (see spec.md's dual-filter contract).
    const secondStepA = await apiClient.createWorkflowStep(seedData.workflowId, "Doing (A)", 1);
    secondStepAId = secondStepA.id;

    // Create workflow B with its own start step
    const workflowB = await apiClient.createWorkflow(seedData.workspaceId, "Workflow B", "simple");
    workflowBId = workflowB.id;
    const stepsB = (await apiClient.listWorkflowSteps(workflowB.id)).steps;
    const startB = stepsB.find((s) => s.is_start_step) ?? stepsB[0];
    startStepBId = startB.id;

    await apiClient.createTask(seedData.workspaceId, TASK_A, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.createTask(seedData.workspaceId, TASK_A_OTHER_STEP, {
      workflow_id: seedData.workflowId,
      workflow_step_id: secondStepAId,
    });
    await apiClient.createTask(seedData.workspaceId, TASK_B, {
      workflow_id: workflowB.id,
      workflow_step_id: startB.id,
    });
  });

  test.afterEach(async ({ apiClient, seedData }) => {
    if (workflowBId) {
      await apiClient.deleteWorkflow(workflowBId).catch(() => {});
      workflowBId = null;
    }
    if (secondStepAId) {
      await apiClient.deleteWorkflowStep(secondStepAId).catch(() => {});
      secondStepAId = null;
    }
    startStepAId = null;
    startStepBId = null;
    // Clear hidden step IDs and restore default filter
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: seedData.workflowId,
      repository_ids: [],
      kanban_hidden_step_ids: {},
    });
  });

  test("hiding a step removes its tasks and collapses its column without affecting other workflows", async ({
    testPage,
  }) => {
    if (!startStepAId || !secondStepAId) throw new Error("workflow A step ids not set");

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // Switch to All Workflows so both swimlanes are visible
    await testPage.getByTestId("display-button").click();
    await testPage.getByTestId("display-workflow-filter").click();
    const listbox = testPage.getByRole("listbox");
    await listbox.getByRole("option", { name: "All Workflows", exact: true }).click();
    await expect(listbox).toHaveCount(0);
    await closeDisplayDropdown(kanban);

    await expect(kanban.taskCardByTitle(TASK_A)).toBeVisible({ timeout: TASK_VISIBLE_TIMEOUT });
    await expect(kanban.taskCardByTitle(TASK_A_OTHER_STEP)).toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });
    await expect(kanban.taskCardByTitle(TASK_B)).toBeVisible({ timeout: TASK_VISIBLE_TIMEOUT });

    // Hide the start step of workflow A
    await openDisplayDropdown(kanban);
    await testPage.getByTestId(`steps-filter-step-${startStepAId}`).click();
    await closeDisplayDropdown(kanban);

    // Task A is now hidden; column for step A is collapsed (removed, not
    // merely emptied) — but workflow A's OTHER step/task survive, proving
    // this is genuine per-step column collapse and not the whole workflow A
    // swimlane disappearing because it ran out of visible tasks.
    await expect(kanban.taskCardByTitle(TASK_A)).not.toBeVisible();
    await expect(kanban.columnByStepId(startStepAId)).not.toBeVisible();
    await expect(kanban.taskCardByTitle(TASK_A_OTHER_STEP)).toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });
    await expect(kanban.columnByStepId(secondStepAId)).toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });

    // The dual-filter trap: task A must not resurface in the synthetic
    // "Needs Reassignment" orphan column — hiding a step removes its tasks
    // AND its column together, so there is nothing left to orphan-remap.
    await expect(kanban.columnByStepId(ORPHAN_STEP_ID)).toHaveCount(0);

    // Workflow B is unaffected
    await expect(kanban.taskCardByTitle(TASK_B)).toBeVisible({ timeout: TASK_VISIBLE_TIMEOUT });

    // Re-show the step — task A and its column reappear
    await openDisplayDropdown(kanban);
    await testPage.getByTestId(`steps-filter-step-${startStepAId}`).click();
    await closeDisplayDropdown(kanban);

    await expect(kanban.columnByStepId(startStepAId)).toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });
    await expect(kanban.taskCardByTitle(TASK_A)).toBeVisible({ timeout: TASK_VISIBLE_TIMEOUT });
  });

  test("hidden step selection persists across page reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    if (!startStepAId) throw new Error("startStepAId not set");

    // Persist hidden step via API (simulating a prior session choice)
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: seedData.workflowId,
      kanban_hidden_step_ids: { [seedData.workflowId]: [startStepAId] },
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // The task and column for the hidden step should not be visible after reload
    await expect(kanban.taskCardByTitle(TASK_A)).not.toBeVisible();
    await expect(kanban.columnByStepId(startStepAId)).not.toBeVisible();

    // Task A must not resurface as an orphan either.
    await expect(kanban.columnByStepId(ORPHAN_STEP_ID)).toHaveCount(0);

    // The checkbox should reflect the persisted state
    await openDisplayDropdown(kanban);
    const checkbox = testPage.getByTestId(`steps-filter-step-${startStepAId}`);
    await expect(checkbox).not.toBeChecked();
    await closeDisplayDropdown(kanban);
  });

  test("step filter is scoped per workflow — hiding a step in A does not hide the same-named step in B", async ({
    testPage,
  }) => {
    if (!startStepAId || !secondStepAId || !startStepBId) {
      throw new Error("step IDs not set");
    }

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // Switch to All Workflows
    await testPage.getByTestId("display-button").click();
    await testPage.getByTestId("display-workflow-filter").click();
    const listbox = testPage.getByRole("listbox");
    await listbox.getByRole("option", { name: "All Workflows", exact: true }).click();
    await expect(listbox).toHaveCount(0);
    await closeDisplayDropdown(kanban);

    await expect(kanban.taskCardByTitle(TASK_A)).toBeVisible({ timeout: TASK_VISIBLE_TIMEOUT });
    await expect(kanban.taskCardByTitle(TASK_B)).toBeVisible({ timeout: TASK_VISIBLE_TIMEOUT });

    // Hide the start step of workflow A only
    await openDisplayDropdown(kanban);
    await testPage.getByTestId(`steps-filter-step-${startStepAId}`).click();

    // Workflow B's start step checkbox must remain checked
    const checkboxB = testPage.getByTestId(`steps-filter-step-${startStepBId}`);
    await expect(checkboxB).toBeChecked();
    await closeDisplayDropdown(kanban);

    // Task B remains visible; workflow B column is unaffected — isolation confirmed
    await expect(kanban.taskCardByTitle(TASK_B)).toBeVisible({ timeout: TASK_VISIBLE_TIMEOUT });
    await expect(kanban.columnByStepId(startStepBId)).toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });
    await expect(kanban.taskCardByTitle(TASK_A)).not.toBeVisible();
    await expect(kanban.columnByStepId(startStepAId)).not.toBeVisible();

    // Workflow A survives (its other step/task are still visible) — proving
    // A's column specifically collapsed rather than the whole swimlane
    // vanishing for lack of visible tasks.
    await expect(kanban.taskCardByTitle(TASK_A_OTHER_STEP)).toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });
    await expect(kanban.columnByStepId(secondStepAId)).toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });
  });
});
