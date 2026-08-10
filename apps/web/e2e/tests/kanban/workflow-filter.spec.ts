import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import type { Page } from "@playwright/test";

const TASK_VISIBLE_TIMEOUT = 10_000;
const ALPHA_TASK = "Alpha task";
const BETA_TASK = "Beta task";

async function pickListboxOption(page: Page, optionLabel: string): Promise<void> {
  // Radix Select renders the currently-selected item twice (once in the trigger
  // for SelectValue display, once in the listbox). Scope to the listbox so we
  // click the real option.
  const listbox = page.getByRole("listbox");
  await listbox.getByRole("option", { name: optionLabel, exact: true }).click();
  await expect(listbox).toHaveCount(0);
}

async function closeDisplayDropdown(page: Page): Promise<void> {
  const trigger = page.getByTestId("display-button");
  if ((await trigger.getAttribute("data-state")) === "open") {
    await trigger.click({ force: true });
  }
  await expect(trigger).not.toHaveAttribute("data-state", "open");
  await expect(page.getByRole("menu")).toHaveCount(0);
}

async function selectWorkflowFilter(page: Page, optionLabel: string): Promise<void> {
  await page.getByTestId("display-button").click();
  await page.getByTestId("display-workflow-filter").click();
  await pickListboxOption(page, optionLabel);
  await closeDisplayDropdown(page);
}

test.describe("Kanban workflow filter", () => {
  let workflowBId: string | null = null;
  let betaTaskId: string | null = null;

  // Pull `testPage` so its fixture (which runs `e2eReset` and resets user
  // settings) is set up before this hook seeds workflows/tasks — otherwise
  // the reset wipes the seed data the moment a test first reads testPage.
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  test.beforeEach(async ({ apiClient, seedData, testPage }) => {
    const workflowB = await apiClient.createWorkflow(seedData.workspaceId, "Workflow B", "simple");
    workflowBId = workflowB.id;
    const stepsB = (await apiClient.listWorkflowSteps(workflowB.id)).steps;
    const startB = stepsB.find((s) => s.is_start_step) ?? stepsB[0];

    await apiClient.createTask(seedData.workspaceId, ALPHA_TASK, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const beta = await apiClient.createTask(seedData.workspaceId, BETA_TASK, {
      workflow_id: workflowB.id,
      workflow_step_id: startB.id,
    });
    betaTaskId = beta.id;
  });

  test.afterEach(async ({ apiClient, seedData }) => {
    if (workflowBId) {
      await apiClient.deleteWorkflow(workflowBId).catch(() => {});
      workflowBId = null;
    }
    betaTaskId = null;
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: seedData.workflowId,
      repository_ids: [],
    });
  });

  test("keeps All Workflows selected for a one-workflow workspace after reopening and reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    if (workflowBId) {
      await apiClient.deleteWorkflow(workflowBId);
      workflowBId = null;
    }
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: seedData.workflowId,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await selectWorkflowFilter(testPage, "All Workflows");
    await testPage.getByTestId("display-button").click();
    await expect(testPage.getByTestId("display-workflow-filter")).toContainText("All Workflows");
    await closeDisplayDropdown(testPage);

    await testPage.reload();
    await testPage.getByTestId("display-button").click();
    await expect(testPage.getByTestId("display-workflow-filter")).toContainText("All Workflows");
  });

  // Regression: c64e835 made resolveDesiredWorkflowId fall back to the first
  // visible workflow whenever both the active id and persisted setting were
  // null. The kanban page's useWorkflowSelection effect then silently
  // overwrote a freshly-picked "All Workflows" choice on the next render.
  // The /tasks list page does not run that effect, so the existing
  // task-list-filters spec missed this — pin the kanban path explicitly.
  test("'All Workflows' selection persists on the kanban board", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await expect(kanban.taskCardByTitle(ALPHA_TASK)).toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });
    await expect(kanban.taskCardByTitle(BETA_TASK)).not.toBeVisible();

    await selectWorkflowFilter(testPage, "All Workflows");

    // Both tasks visible — useWorkflowSelection must not overwrite the null choice.
    await expect(kanban.taskCardByTitle(ALPHA_TASK)).toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });
    await expect(kanban.taskCardByTitle(BETA_TASK)).toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });
  });

  // Regression: SSR resolveActiveId in app/page.tsx fell back to the first
  // visible workflow when settings.workflow_filter_id was empty, so a saved
  // "All Workflows" preference reverted on hard refresh.
  test("'All Workflows' selection survives a hard refresh", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Persist "All Workflows" directly via the API to mimic the post-selection
    // state, then load the kanban page from scratch (no in-flight client state).
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: "",
      repository_ids: [],
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await expect(kanban.taskCardByTitle(ALPHA_TASK)).toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });
    await expect(kanban.taskCardByTitle(BETA_TASK)).toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });
  });

  // Regression: hidden workflows (e.g. Improve Kandev, whose tasks the user
  // creates through the Improve dialog) were excluded from the board's swimlane
  // selection even when explicitly picked in the display dropdown — the user
  // saw "No tasks yet" for a workflow that had tasks, and the filter reverted
  // to All Workflows on the next navigation.
  test("renders a hidden workflow's tasks when the user selects it", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const hiddenWorkflow = await apiClient.e2eCreateHiddenWorkflow(
      seedData.workspaceId,
      "Hidden Flow",
    );
    // The e2e hidden-workflow endpoint creates a bare workflow (no template),
    // so seed a start step for the task to land in.
    const startStep = await apiClient.createWorkflowStep(hiddenWorkflow.id, "Backlog", 0, {
      is_start_step: true,
    });
    await apiClient.createTask(seedData.workspaceId, "Hidden flow task", {
      workflow_id: hiddenWorkflow.id,
      workflow_step_id: startStep.id,
    });
    try {
      const kanban = new KanbanPage(testPage);
      await kanban.goto();

      // Hidden workflows stay off the All Workflows board by design...
      await selectWorkflowFilter(testPage, "All Workflows");
      await expect(kanban.taskCardByTitle("Hidden flow task")).not.toBeVisible();

      // ...but an explicit selection renders their board.
      await selectWorkflowFilter(testPage, "Hidden Flow");
      await expect(kanban.taskCardByTitle("Hidden flow task")).toBeVisible({
        timeout: TASK_VISIBLE_TIMEOUT,
      });
    } finally {
      await apiClient.deleteWorkflow(hiddenWorkflow.id).catch(() => {});
    }
  });

  // Regression: SSR wrote workflows.activeId from the task's workflow_id, clobbering the "All Workflows" filter on return. Pins the cross-page flow.
  test("'All Workflows' selection survives navigating into a task and back", async ({
    testPage,
  }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await selectWorkflowFilter(testPage, "All Workflows");

    await expect(kanban.taskCardByTitle(ALPHA_TASK)).toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });
    await expect(kanban.taskCardByTitle(BETA_TASK)).toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });

    // Visit a task that belongs to Workflow B — the SSR build path that used
    // to poison the global filter.
    if (!betaTaskId) throw new Error("beta task was not seeded");
    await testPage.goto(`/t/${betaTaskId}`);
    await expect(testPage).toHaveURL(new RegExp(`/t/${betaTaskId}`));

    // AppSidebar Home link = client-side nav: goto("/") re-runs SSR and re-resolves activeId, masking the bug.
    // `exact: true` disambiguates from the brand "Kandev home" link, which also
    // points at "/" and would otherwise match the accessible-name substring.
    await testPage
      .getByTestId("app-sidebar")
      .getByRole("link", { name: "Home", exact: true })
      .click();
    await expect(testPage).toHaveURL(/\/$|\?/);
    await expect(kanban.board).toBeVisible();

    await expect(kanban.taskCardByTitle(ALPHA_TASK)).toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });
    await expect(kanban.taskCardByTitle(BETA_TASK)).toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });
  });
});
