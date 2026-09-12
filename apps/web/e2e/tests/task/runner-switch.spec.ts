import type { Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";
import type { ApiClient } from "../../helpers/api-client";

// REQ-TASKS-RUNNER-SWITCH-004: the executor-profile picker in the task edit
// dialog, gated by the server-projected runner_editable verdict rather than
// task state. Exercises the regular task-create/edit dialog, so run with the
// office feature disabled (see useRegularMode).
useRegularMode();

type ExecutorProfile = { id: string; name: string };
type ExecutorSummary = { id: string; type: string; profiles?: ExecutorProfile[] };

async function twoExecutorProfiles(
  apiClient: ApiClient,
): Promise<{ first: ExecutorProfile; second: ExecutorProfile }> {
  const { executors } = await apiClient.listExecutors();
  const worktreeExecutor = executors.find(
    (executor: ExecutorSummary) => executor.type === "worktree",
  );
  const localExecutor = executors.find((executor: ExecutorSummary) =>
    ["local", "local_pc"].includes(executor.type),
  );
  const first = worktreeExecutor?.profiles?.[0];
  const second = localExecutor?.profiles?.[0];
  expect(first, "a worktree executor profile is required by the fixture").toBeDefined();
  expect(second, "a local executor profile is required by the fixture").toBeDefined();
  return { first: first!, second: second! };
}

async function openEditDialog(page: Page, kanban: KanbanPage, taskId: string) {
  await kanban.openTaskActionsMenu(taskId);
  await page.getByRole("menuitem", { name: "Edit", exact: true }).click();
  const dialog = page.getByTestId("create-task-dialog");
  await expect(dialog).toBeVisible();
  return dialog;
}

// A not-started task's edit dialog also auto-picks a default agent, which
// flips the footer to the split "Start task" button (showStartTask in
// task-create-dialog-footer.tsx) instead of a plain "Update task" button.
// The plain button only appears once an agent has been explicitly cleared.
// Saving without launching a session means opening the split button's
// dropdown and choosing its "Update task" alt action either way.
async function saveEditWithoutStartingAgent(
  page: Page,
  dialog: import("@playwright/test").Locator,
) {
  const chevron = dialog.getByTestId("submit-start-agent-chevron");
  if (await chevron.isVisible()) {
    await chevron.click();
    await page.getByTestId("submit-create-without-agent").click();
    return;
  }
  await dialog.getByRole("button", { name: "Update task", exact: true }).click();
}

test.describe("Runner switch before materialization", () => {
  test("switching the executor profile on an eligible task updates the stored runner", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { first, second } = await twoExecutorProfiles(apiClient);
    const task = await apiClient.createTask(seedData.workspaceId, "Runner switch eligible task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    // `executor_profile_id` on the create body is only persisted to metadata
    // when an agent_profile_id is also set (task_http_handlers.go), so a
    // plain task needs the stored profile written directly to give the
    // dialog a genuine "stored choice" to seed from, distinct from the
    // "resolved default" case covered below.
    await apiClient.updateTaskMetadata(task.id, { executor_profile_id: first.id });

    const ws = watchWs(testPage);
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const dialog = await openEditDialog(testPage, kanban, task.id);

    const executorSelector = dialog.getByTestId("executor-profile-selector");
    await expect(executorSelector).toBeVisible();
    await expect(dialog.getByTestId("runner-ineligible-note")).toHaveCount(0);
    await expect(executorSelector).toContainText(first.name);

    await executorSelector.click();
    await testPage.getByRole("option", { name: new RegExp(second.name) }).click();

    const switched = ws.waitForResponse("task.runner");
    await saveEditWithoutStartingAgent(testPage, dialog);
    await switched;
    await expect(dialog).toHaveCount(0);

    const updated = await apiClient.getTask(task.id);
    expect(updated.metadata?.executor_profile_id).toBe(second.id);
  });

  test("a task with an existing session presents the reason instead of the picker", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Runner switch ineligible task",
      seedData.agentProfileId,
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await expect
      .poll(async () => (await apiClient.getTask(task.id)).primary_session_id ?? null)
      .not.toBeNull();

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const dialog = await openEditDialog(testPage, kanban, task.id);

    const note = dialog.getByTestId("runner-ineligible-note");
    await expect(note).toBeVisible();
    await expect(note).toContainText("already has a session");
    await expect(dialog.getByTestId("executor-profile-selector")).toHaveCount(0);
  });

  test("a switch rejected after the dialog opened leaves the dialog open with no partial save", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { first, second } = await twoExecutorProfiles(apiClient);
    const task = await apiClient.createTask(seedData.workspaceId, "Runner switch race task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await apiClient.updateTaskMetadata(task.id, { executor_profile_id: first.id });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const dialog = await openEditDialog(testPage, kanban, task.id);

    const executorSelector = dialog.getByTestId("executor-profile-selector");
    await executorSelector.click();
    await testPage.getByRole("option", { name: new RegExp(second.name) }).click();

    // Simulate a concurrent change that makes the task ineligible between the
    // dialog opening (with a stale eligible snapshot) and Save: the server
    // re-evaluates the gate inside the switch transaction and must reject.
    await apiClient.archiveTask(task.id);

    await saveEditWithoutStartingAgent(testPage, dialog);

    const toast = testPage.getByTestId("toast-message");
    await expect(toast).toBeVisible();
    await expect(toast).toContainText("archived");
    await expect(dialog).toBeVisible();

    const afterRejection = await apiClient.getTask(task.id);
    expect(afterRejection.metadata?.executor_profile_id).toBe(first.id);
  });

  test("saving without changing a seeded default profile issues no switch", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Runner switch untouched task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    expect((await apiClient.getTask(task.id)).metadata?.executor_profile_id).toBeUndefined();

    const ws = watchWs(testPage);
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const dialog = await openEditDialog(testPage, kanban, task.id);

    // The picker seeds a resolved default (AC-TASKS-RUNNER-SWITCH-004.5a);
    // leaving it untouched must not pin that default as an explicit switch.
    await expect(dialog.getByTestId("executor-profile-selector")).toBeVisible();
    await dialog.getByTestId("task-title-input").fill("Runner switch untouched task (renamed)");

    const rejectedRunnerCall = ws
      .waitForResponse("task.runner", { timeout: 3_000 })
      .then(() => "sent" as const)
      .catch(() => "not-sent" as const);
    await saveEditWithoutStartingAgent(testPage, dialog);
    await expect(dialog).toHaveCount(0);
    expect(await rejectedRunnerCall).toBe("not-sent");

    const updated = await apiClient.getTask(task.id);
    expect(updated.title).toBe("Runner switch untouched task (renamed)");
    expect(updated.metadata?.executor_profile_id).toBeUndefined();
  });
});
