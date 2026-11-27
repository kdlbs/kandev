import type { Page } from "@playwright/test";
import { expect, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { KanbanPage } from "../../pages/kanban-page";

// @covers AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.7
export async function verifyForkReviewStart(
  { testPage, apiClient, seedData }: { testPage: Page; apiClient: ApiClient; seedData: SeedData },
  mobile: boolean,
) {
  const { executors } = await apiClient.listExecutors();
  const worktree = executors.find((executor) => executor.type === "worktree");
  if (!worktree) throw new Error("Missing worktree executor");
  const runner = await apiClient.createExecutorProfile(worktree.id, "Review Watch runner");
  await apiClient.saveUserSettings({
    workspace_id: seedData.workspaceId,
    workflow_filter_id: seedData.workflowId,
    task_create_last_used: {
      repository_id: seedData.repositoryId,
      branch: "main",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    },
  });
  const task = await apiClient.createTask(seedData.workspaceId, "Fork review awaiting start", {
    description: "/e2e:simple-message",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
    agent_profile_id: seedData.agentProfileId,
    executor_profile_id: runner.id,
    metadata: {
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: runner.id,
      fork_pr_requires_manual_start: true,
      auto_start_guard: true,
    },
  });
  if (!mobile) {
    await apiClient.saveUserSettings({ enable_preview_on_click: true });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.taskCardByTitle(task.title).click();
    const previewChat = testPage.getByTestId("preview-sessionless-chat");
    await expect(previewChat.getByTestId("fork-review-start-warning")).toBeVisible();
  }
  await testPage.goto(`/t/${task.id}`);
  const warning = testPage.getByTestId("session-chat").getByTestId("fork-review-start-warning");
  await expect(warning).toContainText("Manual start required");
  await expect(warning).toContainText("executor credentials");
  expect((await apiClient.listTaskSessions(task.id)).sessions).toHaveLength(0);
  const start = warning.getByRole("button", { name: "Start task", exact: true });
  if (mobile) {
    expect((await start.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await start.tap();
  } else {
    await start.click();
  }
  const dialog = testPage.getByTestId("create-task-dialog");
  await expect(dialog.getByTestId("fork-review-start-warning")).toContainText(
    "executor credentials",
  );
  await dialog.getByTestId("submit-cancel").click();
  await expect(dialog).toBeHidden();
  expect((await apiClient.listTaskSessions(task.id)).sessions).toHaveLength(0);

  if (mobile) {
    await start.tap();
  } else {
    await testPage.getByTestId("task-topbar-actions-menu").click();
    await testPage.getByRole("menuitem", { name: "Edit", exact: true }).click();
    await expect(dialog.getByTestId("fork-review-start-warning")).toContainText(
      "executor credentials",
    );
  }
  await expect(dialog.getByTestId("executor-profile-selector")).toContainText(runner.name);
  await dialog.getByTestId("submit-start-agent").click();
  await expect
    .poll(async () => (await apiClient.listTaskSessions(task.id)).sessions.length)
    .toBe(1);
  const {
    sessions: [session],
  } = await apiClient.listTaskSessions(task.id);
  expect(session.executor_profile_id).toBe(runner.id);
  await testPage.reload();
  await expect(
    testPage.locator(`[data-testid="session-chat"][data-session-id="${session.id}"]`),
  ).toBeVisible();
  await expect(warning).toBeHidden();
  expect((await apiClient.listTaskSessions(task.id)).sessions).toHaveLength(1);
}
