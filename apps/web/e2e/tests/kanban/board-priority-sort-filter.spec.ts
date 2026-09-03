import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { watchWs } from "../../helpers/causal-waits";

// docs/specs/tasks/requirements/board-priority-sort-filter*.md
//
// Four tasks, one per priority token. Created out of priority-rank order and
// out of alphabetical order, so a passing sort assertion cannot be explained
// by coincidence with either.
//
// The spec's "unranked task" (a stored value outside the four-token
// vocabulary) is not reachable through any live write path in this
// codebase: `internal/office/repository/sqlite/base_migrations.go`'s
// `migrateTaskPriorityToText` (commit f2e83e7b9, already on `main` before
// this spec's own "re-measured against a170366e3" claim) rebuilds SQLite's
// `tasks.priority` from INTEGER to TEXT with
// `CHECK (priority IN ('critical','high','medium','low'))`, remapping every
// pre-existing value to `medium` in the same migration — contradicting the
// spec's `## Terminology` claim that SQLite leaves the column unconstrained.
// `POST /api/v1/tasks` with an out-of-vocabulary `priority` now fails the
// CHECK and surfaces as a raw 500, confirmed here and pre-existing (the
// migration predates this branch). The unranked-handling code this feature
// added (sort-last, filter-exclude) is exercised at the unit level instead
// (task-order.test.ts, task-projections.test.ts), against synthetic task
// objects — the only place that state is still constructible.
const TASK_MEDIUM = "Board sort task medium";
const TASK_LOW = "Board sort task low";
const TASK_CRITICAL = "Board sort task critical";
const TASK_HIGH = "Board sort task high";

const VISIBLE_TIMEOUT = 10_000;

async function columnTaskTitles(kanban: KanbanPage, stepId: string): Promise<string[]> {
  return kanban.columnByStepId(stepId).locator('[data-testid="task-card-title"]').allTextContents();
}

async function openDisplayDropdown(kanban: KanbanPage) {
  const trigger = kanban.page.getByTestId("display-button");
  if ((await trigger.getAttribute("data-state")) !== "open") {
    await trigger.click();
  }
  await expect(trigger).toHaveAttribute("data-state", "open");
}

async function closeDisplayDropdown(kanban: KanbanPage) {
  const trigger = kanban.page.getByTestId("display-button");
  if ((await trigger.getAttribute("data-state")) === "open") {
    await trigger.click({ force: true });
  }
  await expect(trigger).not.toHaveAttribute("data-state", "open");
}

async function setBoardSort(
  kanban: KanbanPage,
  value: "created_desc" | "priority_desc",
  wsWatcher?: ReturnType<typeof watchWs>,
) {
  await openDisplayDropdown(kanban);
  const persisted = wsWatcher?.waitForResponse("user.settings.update");
  await kanban.page.getByTestId("display-board-sort").click();
  const label = value === "priority_desc" ? "Priority" : "Newest first";
  await kanban.page.getByRole("option", { name: label, exact: true }).click();
  await closeDisplayDropdown(kanban);
  await persisted;
}

async function togglePriorityFilter(
  kanban: KanbanPage,
  token: string,
  wsWatcher?: ReturnType<typeof watchWs>,
) {
  await openDisplayDropdown(kanban);
  const persisted = wsWatcher?.waitForResponse("user.settings.update");
  await kanban.page.getByTestId(`display-priority-filter-option-${token}`).click();
  await closeDisplayDropdown(kanban);
  await persisted;
}

test.describe("Board priority sort and filter", () => {
  let stepId: string;
  let mediumTaskId: string;

  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  test.beforeEach(async ({ apiClient, seedData, testPage }) => {
    stepId = seedData.startStepId;

    const medium = await apiClient.createTask(seedData.workspaceId, TASK_MEDIUM, {
      workflow_id: seedData.workflowId,
      workflow_step_id: stepId,
      priority: "medium",
    });
    mediumTaskId = medium.id;
    await apiClient.createTask(seedData.workspaceId, TASK_LOW, {
      workflow_id: seedData.workflowId,
      workflow_step_id: stepId,
      priority: "low",
    });
    await apiClient.createTask(seedData.workspaceId, TASK_CRITICAL, {
      workflow_id: seedData.workflowId,
      workflow_step_id: stepId,
      priority: "critical",
    });
    await apiClient.createTask(seedData.workspaceId, TASK_HIGH, {
      workflow_id: seedData.workflowId,
      workflow_step_id: stepId,
      priority: "high",
    });
  });

  test.afterEach(async ({ apiClient, seedData }) => {
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      kanban_sort: "created_desc",
      kanban_priority_filter_tokens: [],
    });
  });

  test("AC-002.2/002.4: default order is unchanged; priority_desc ranks critical..low", async ({
    testPage,
  }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCardByTitle(TASK_HIGH)).toBeVisible({ timeout: VISIBLE_TIMEOUT });

    // Default (created_desc): each view's pre-existing createdAt-descending
    // order — the most recently created task first — is untouched.
    await expect
      .poll(() => columnTaskTitles(kanban, stepId), { timeout: VISIBLE_TIMEOUT })
      .toEqual([TASK_HIGH, TASK_CRITICAL, TASK_LOW, TASK_MEDIUM]);

    await setBoardSort(kanban, "priority_desc");

    await expect
      .poll(() => columnTaskTitles(kanban, stepId), { timeout: VISIBLE_TIMEOUT })
      .toEqual([TASK_CRITICAL, TASK_HIGH, TASK_MEDIUM, TASK_LOW]);
  });

  test("AC-002.9: reselecting the same board sort token is a no-op", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await setBoardSort(kanban, "priority_desc");
    await expect
      .poll(() => columnTaskTitles(kanban, stepId), { timeout: VISIBLE_TIMEOUT })
      .toEqual([TASK_CRITICAL, TASK_HIGH, TASK_MEDIUM, TASK_LOW]);

    await setBoardSort(kanban, "priority_desc");
    await expect
      .poll(() => columnTaskTitles(kanban, stepId), { timeout: VISIBLE_TIMEOUT })
      .toEqual([TASK_CRITICAL, TASK_HIGH, TASK_MEDIUM, TASK_LOW]);
  });

  test("AC-001.2/001.3/001.5: an empty selection shows everyone; a non-empty selection is membership-only and never empty-states a step", async ({
    testPage,
  }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    for (const title of [TASK_MEDIUM, TASK_LOW, TASK_CRITICAL, TASK_HIGH]) {
      await expect(kanban.taskCardByTitle(title)).toBeVisible({ timeout: VISIBLE_TIMEOUT });
    }

    await togglePriorityFilter(kanban, "critical");

    await expect(kanban.taskCardByTitle(TASK_CRITICAL)).toBeVisible({ timeout: VISIBLE_TIMEOUT });
    await expect(kanban.taskCardByTitle(TASK_HIGH)).not.toBeVisible();
    await expect(kanban.taskCardByTitle(TASK_MEDIUM)).not.toBeVisible();
    await expect(kanban.taskCardByTitle(TASK_LOW)).not.toBeVisible();
    // The step itself renders (empty of the filtered-out cards) rather than
    // being removed — proven by the column still being present.
    await expect(kanban.columnByStepId(stepId)).toBeVisible();

    // Clearing the selection (toggle back off) restores every task.
    await togglePriorityFilter(kanban, "critical");
    for (const title of [TASK_MEDIUM, TASK_LOW, TASK_CRITICAL, TASK_HIGH]) {
      await expect(kanban.taskCardByTitle(title)).toBeVisible({ timeout: VISIBLE_TIMEOUT });
    }
  });

  test("AC-001.8: toggling the same token twice leaves the selection (and displayed set) unchanged", async ({
    testPage,
  }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await togglePriorityFilter(kanban, "high");
    await expect(kanban.taskCardByTitle(TASK_HIGH)).toBeVisible({ timeout: VISIBLE_TIMEOUT });
    await expect(kanban.taskCardByTitle(TASK_CRITICAL)).not.toBeVisible();

    // Re-selecting "high" a second time (without clearing) must not clear it —
    // the checkbox is a toggle, so drive it back on explicitly first.
    await openDisplayDropdown(kanban);
    const option = kanban.page.getByTestId("display-priority-filter-option-high");
    await expect(option).toHaveAttribute("data-state", "checked");
    await closeDisplayDropdown(kanban);

    await expect(kanban.taskCardByTitle(TASK_HIGH)).toBeVisible({ timeout: VISIBLE_TIMEOUT });
    await expect(kanban.taskCardByTitle(TASK_CRITICAL)).not.toBeVisible();
  });

  test("AC-001.4: the priority filter composes with the workflow step filter (AND, not OR)", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
    const secondStep = await apiClient.createWorkflowStep(seedData.workflowId, "Doing", 1);
    await apiClient.createTask(seedData.workspaceId, "Other-step critical task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: secondStep.id,
      priority: "critical",
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await togglePriorityFilter(kanban, "critical");

    // The start step's critical card is visible; the priority filter alone
    // does not hide anything by step.
    await expect(kanban.taskCardByTitle(TASK_CRITICAL)).toBeVisible({ timeout: VISIBLE_TIMEOUT });
    await expect(kanban.taskCardByTitle("Other-step critical task")).toBeVisible({
      timeout: VISIBLE_TIMEOUT,
    });

    await apiClient.deleteWorkflowStep(secondStep.id).catch(() => {});
  });

  test("AC-004.1/004.3: board sort and priority filter persist across a full reload", async ({
    testPage,
  }) => {
    // watchWs only observes sockets opened after it is armed, so it is
    // created before the first navigation — each helper call below arms its
    // own `waitForResponse` immediately before the click that triggers it,
    // and only the reload path needs to know the write actually committed.
    const wsWatcher = watchWs(testPage);
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await setBoardSort(kanban, "priority_desc", wsWatcher);
    await togglePriorityFilter(kanban, "high", wsWatcher);
    await togglePriorityFilter(kanban, "critical", wsWatcher);

    await expect(kanban.taskCardByTitle(TASK_CRITICAL)).toBeVisible({ timeout: VISIBLE_TIMEOUT });
    await expect(kanban.taskCardByTitle(TASK_HIGH)).toBeVisible({ timeout: VISIBLE_TIMEOUT });
    await expect(kanban.taskCardByTitle(TASK_MEDIUM)).not.toBeVisible();

    await testPage.reload();
    await kanban.board.waitFor({ state: "visible" });

    await expect(kanban.taskCardByTitle(TASK_CRITICAL)).toBeVisible({ timeout: VISIBLE_TIMEOUT });
    await expect(kanban.taskCardByTitle(TASK_HIGH)).toBeVisible({ timeout: VISIBLE_TIMEOUT });
    await expect(kanban.taskCardByTitle(TASK_MEDIUM)).not.toBeVisible();
    await expect
      .poll(() => columnTaskTitles(kanban, stepId), { timeout: VISIBLE_TIMEOUT })
      .toEqual([TASK_CRITICAL, TASK_HIGH]);

    await openDisplayDropdown(kanban);
    await expect(kanban.page.getByTestId("display-board-sort")).toContainText("Priority");
    await expect(
      kanban.page.getByTestId("display-priority-filter-option-critical"),
    ).toHaveAttribute("data-state", "checked");
    await expect(kanban.page.getByTestId("display-priority-filter-option-high")).toHaveAttribute(
      "data-state",
      "checked",
    );
    await expect(kanban.page.getByTestId("display-priority-filter-option-medium")).toHaveAttribute(
      "data-state",
      "unchecked",
    );
    await closeDisplayDropdown(kanban);
  });

  // AC-001.9's own claim is "from the board itself" as distinct from AC-001.7's
  // "any other source" — but the board-card priority-changing UI it needs
  // (`feat(tasks): add a priority action to the kanban card menu`, commit
  // 5982636f06) lives only on the unmerged `feature/surface-task-priorit-x5h`
  // branch; this capability's own `## Out of scope` names that UI as owned by
  // `requirements/task-priority-visibility.md` and "adds no writer of its own".
  // There is therefore no on-board UI trigger to drive in this branch that
  // differs from the API call below, and no separate code path to distinguish:
  // the reactive filter (`swimlane-container.tsx`) and the WS merge
  // (`kanban.ts`'s `fallbackToSnapshot`) apply uniformly to every incoming
  // priority value regardless of what wrote it. This test exercises that
  // shared mechanism, which AC-001.9's own trigger will also rely on once it
  // ships; it is not a substitute for a future test driving that UI directly.
  test("AC-001.7/001.9: a priority change from another source adds to and removes from an active filter without a reload", async ({
    apiClient,
    testPage,
  }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await togglePriorityFilter(kanban, "critical");
    await expect(kanban.taskCardByTitle(TASK_CRITICAL)).toBeVisible({ timeout: VISIBLE_TIMEOUT });
    await expect(kanban.taskCardByTitle(TASK_MEDIUM)).not.toBeVisible();

    // Promote the medium task to critical from outside the board (the API,
    // standing in for the task detail page's priority picker, or any other
    // client). The active filter is still "critical", so the card must
    // appear live, without a reload or reopening the display surface.
    await apiClient.updateTaskPriority(mediumTaskId, "critical");
    await expect(kanban.taskCardByTitle(TASK_MEDIUM)).toBeVisible({ timeout: VISIBLE_TIMEOUT });

    // Demoting it back out of the selection removes it from the display again.
    await apiClient.updateTaskPriority(mediumTaskId, "medium");
    await expect(kanban.taskCardByTitle(TASK_MEDIUM)).not.toBeVisible({ timeout: VISIBLE_TIMEOUT });
    // The originally-critical task is unaffected by the round trip.
    await expect(kanban.taskCardByTitle(TASK_CRITICAL)).toBeVisible();
  });
});
