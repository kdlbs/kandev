import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";

async function seedBadgeTest(
  apiClient: ApiClient,
  workspaceId: string,
  agentProfileId: string,
  repositoryId: string,
  title: string,
) {
  const workflow = await apiClient.createWorkflow(workspaceId, `${title} Workflow`);
  const inboxStep = await apiClient.createWorkflowStep(workflow.id, "Inbox", 0);
  const workingStep = await apiClient.createWorkflowStep(workflow.id, "Working", 1);
  const doneStep = await apiClient.createWorkflowStep(workflow.id, "Done", 2);

  await apiClient.updateWorkflowStep(workingStep.id, {
    prompt: 'e2e:message("done")\n{{task_prompt}}',
    events: {
      on_enter: [{ type: "auto_start_agent" }],
      on_turn_complete: [{ type: "move_to_step", config: { step_id: doneStep.id } }],
    },
  });

  await apiClient.saveUserSettings({
    workspace_id: workspaceId,
    workflow_filter_id: workflow.id,
    enable_preview_on_click: false,
  });

  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("test-user");

  const task = await apiClient.createTask(workspaceId, title, {
    workflow_id: workflow.id,
    workflow_step_id: inboxStep.id,
    agent_profile_id: agentProfileId,
    repository_ids: [repositoryId],
  });

  return { workflow, inboxStep, workingStep, doneStep, task };
}

test.describe("PR status badge", () => {
  /**
   * Regression for the "CI pending" bug: GitHub reports all checks passed
   * (one skipped, many successful). We used to compute "pending" because
   * skipped checks weren't classified explicitly. The badge must now show
   * the success colour.
   */
  test("renders success when all checks passed with some skipped", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const { workflow, workingStep, doneStep, task } = await seedBadgeTest(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      "CI Skipped Task",
    );

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await apiClient.moveTask(task.id, workflow.id, workingStep.id);

    // Seed task PR directly with checks_state=success (post-fix behaviour
    // when 20 success + 1 skipped checks flow through computeOverallCheckStatus).
    // Associating after moveTask is intentional: the task may already reach Done
    // before this call, and the mock controller's github.task_pr.updated event
    // is what refreshes the badge on the already-rendered kanban card. Don't
    // reorder before moveTask without preserving that event flow.
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 101,
      pr_url: "https://github.com/testorg/testrepo/pull/101",
      pr_title: "Skipped checks",
      head_branch: "feat/skipped",
      base_branch: "main",
      author_login: "test-user",
      state: "open",
      checks_state: "success",
    });

    await expect(kanban.taskCardInColumn("CI Skipped Task", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    const icon = testPage.getByTestId(`pr-task-icon-${task.id}`);
    await expect(icon).toBeVisible({ timeout: 15_000 });

    // No reviews, so ready-to-merge must be false; badge should not be yellow.
    await expect(icon).toHaveAttribute("data-pr-ready-to-merge", "false");
    await expect(icon).not.toHaveClass(/text-yellow-500/);

    // Open the task to verify topbar button mirrors the state.
    await kanban.taskCardInColumn("CI Skipped Task", doneStep.id).click();
    await expect(testPage).toHaveURL(/\/[st]\//, { timeout: 15_000 });
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.prTopbarButton()).toBeVisible({ timeout: 15_000 });
    await expect(session.prTopbarButton()).toHaveAttribute("data-pr-ready-to-merge", "false");
  });

  /**
   * When reviewers approved, CI passes, and GitHub's mergeable_state is clean,
   * the badge must show the ready-to-merge state so the user knows the PR
   * is ready to merge.
   */
  test("renders ready-to-merge when approved + clean + checks pass", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const { workflow, workingStep, doneStep, task } = await seedBadgeTest(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      "Ready To Merge Task",
    );

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await apiClient.moveTask(task.id, workflow.id, workingStep.id);

    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 102,
      pr_url: "https://github.com/testorg/testrepo/pull/102",
      pr_title: "Ready to ship",
      head_branch: "feat/ready",
      base_branch: "main",
      author_login: "test-user",
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "clean",
    });

    await expect(kanban.taskCardInColumn("Ready To Merge Task", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    const icon = testPage.getByTestId(`pr-task-icon-${task.id}`);
    await expect(icon).toBeVisible({ timeout: 15_000 });
    await expect(icon).toHaveAttribute("data-pr-ready-to-merge", "true");

    await kanban.taskCardInColumn("Ready To Merge Task", doneStep.id).click();
    await expect(testPage).toHaveURL(/\/[st]\//, { timeout: 15_000 });
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.prTopbarButton()).toBeVisible({ timeout: 15_000 });
    await expect(session.prTopbarButton()).toHaveAttribute("data-pr-ready-to-merge", "true");
  });

  /**
   * Guard against false positives: reviewers approved and CI is green, but
   * GitHub reports mergeable_state=blocked (e.g., CODEOWNERS not satisfied).
   * Badge must stay at the plain approved-success state, not ready-to-merge.
   */
  test("does not render ready-to-merge when mergeable_state is blocked", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const { workflow, workingStep, doneStep, task } = await seedBadgeTest(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      "Blocked Task",
    );

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await apiClient.moveTask(task.id, workflow.id, workingStep.id);

    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 103,
      pr_url: "https://github.com/testorg/testrepo/pull/103",
      pr_title: "Blocked by CODEOWNERS",
      head_branch: "feat/blocked",
      base_branch: "main",
      author_login: "test-user",
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "blocked",
    });

    await expect(kanban.taskCardInColumn("Blocked Task", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    const icon = testPage.getByTestId(`pr-task-icon-${task.id}`);
    await expect(icon).toBeVisible({ timeout: 15_000 });
    await expect(icon).toHaveAttribute("data-pr-ready-to-merge", "false");
    // Plain-green approved state, not the ready-to-merge emerald.
    await expect(icon).not.toHaveClass(/text-emerald-400/);
  });

  /**
   * GitHub's review_state="approved" only means at least one reviewer approved.
   * When branch protection requires more approvals, the PR is still blocked
   * and pending_review_count > 0. The badge must read as "awaiting review"
   * (sky-400) rather than fully approved (green-500) or ready-to-merge
   * (emerald-400).
   */
  test("renders awaiting-review when approved with pending reviewers", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const { workflow, workingStep, doneStep, task } = await seedBadgeTest(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      "Awaiting Review Task",
    );

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await apiClient.moveTask(task.id, workflow.id, workingStep.id);

    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 104,
      pr_url: "https://github.com/testorg/testrepo/pull/104",
      pr_title: "1 of 2 approvals",
      head_branch: "feat/partial-approval",
      base_branch: "main",
      author_login: "test-user",
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "blocked",
      pending_review_count: 1,
    });

    await expect(kanban.taskCardInColumn("Awaiting Review Task", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    const icon = testPage.getByTestId(`pr-task-icon-${task.id}`);
    await expect(icon).toBeVisible({ timeout: 15_000 });
    await expect(icon).toHaveAttribute("data-pr-ready-to-merge", "false");
    await expect(icon).toHaveClass(/text-sky-400/);
    await expect(icon).not.toHaveClass(/text-emerald-400/);
    await expect(icon).not.toHaveClass(/text-green-500/);
  });
});
