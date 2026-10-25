import { expect, test } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import { useRegularMode } from "../../helpers/regular-mode";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import type { ApiClient } from "../../helpers/api-client";
import type { Page } from "@playwright/test";

// REQ-TASKS-RUNNER-SWITCH-004.6: desktop and mobile use the same gate, the
// same reason text, and the same interaction — this capability introduces no
// new mobile layout or pattern, so these mirror runner-switch.spec.ts's
// desktop scenarios through the mobile task-actions sheet instead of a
// bespoke mobile flow.
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

async function openEditDialogFromCard(page: Page, mobile: MobileKanbanPage, taskId: string) {
  await mobile.taskCard(taskId).getByRole("button", { name: "More options" }).tap();
  await page
    .locator('[data-slot="dropdown-menu-content"]:visible')
    .getByRole("menuitem", { name: "Edit", exact: true })
    .tap();
  const dialog = page.getByTestId("create-task-dialog");
  await expect(dialog).toBeVisible();
  return dialog;
}

test.describe("Runner switch before materialization on mobile", () => {
  test("switching the executor profile through the mobile task-actions sheet updates the stored runner", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { first, second } = await twoExecutorProfiles(apiClient);
    const task = await apiClient.createTask(
      seedData.workspaceId,
      "Mobile runner switch eligible task",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await apiClient.updateTaskMetadata(task.id, { executor_profile_id: first.id });

    const ws = watchWs(testPage);
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    const dialog = await openEditDialogFromCard(testPage, mobile, task.id);

    // Touch-target sizing for this selector is pre-existing (unchanged by
    // this feature, see task-create-dialog-selectors.tsx) and out of scope
    // here; this test covers only the runner-switch gating and interaction.
    const executorSelector = dialog.getByTestId("executor-profile-selector");
    await expect(executorSelector).toBeVisible();

    await executorSelector.tap();
    await testPage.getByRole("option", { name: new RegExp(second.name) }).tap();

    const switched = ws.waitForResponse("task.runner");
    await dialog.getByRole("button", { name: "Update task", exact: true }).tap();
    await switched;
    await expect(dialog).toHaveCount(0);

    const updated = await apiClient.getTask(task.id);
    expect(updated.metadata?.executor_profile_id).toBe(second.id);
  });

  test("a task with an existing session presents the same reason text as desktop", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile runner switch ineligible task",
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

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    const dialog = await openEditDialogFromCard(testPage, mobile, task.id);

    const note = dialog.getByTestId("runner-ineligible-note");
    await expect(note).toBeVisible();
    await expect(note).toContainText("already has a session");
    await expect(dialog.getByTestId("executor-profile-selector")).toHaveCount(0);
  });
});
