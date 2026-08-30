import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { Page } from "@playwright/test";

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

type TaskPR = NonNullable<Awaited<ReturnType<ApiClient["getTaskPR"]>>>;

function visibleTaskPRSummary(page: Page) {
  return page
    .locator(
      '[data-slot="tooltip-content"]:not([data-state="closed"]) > [data-testid="pr-task-status-summary"]',
    )
    .first();
}

async function waitForTaskPRFields(
  apiClient: ApiClient,
  taskId: string,
  expected: Partial<Pick<TaskPR, "state" | "review_state" | "checks_state" | "mergeable_state">> & {
    pending_review_count?: number;
  },
) {
  await expect
    .poll(
      async () => {
        const pr = await apiClient.getTaskPR(taskId);
        if (!pr) return false;
        return Object.entries(expected).every(([key, value]) => pr[key as keyof TaskPR] === value);
      },
      {
        timeout: 15_000,
        message: "Expected backend TaskPR fields to match seeded mock state",
      },
    )
    .toBe(true);
}

async function expectTopbarReadyState(
  page: Page,
  session: SessionPage,
  expected: "true" | "false",
) {
  const button = session.prTopbarButton();
  await button.waitFor({ state: "visible", timeout: 15_000 });

  await button
    .waitFor({ state: "attached", timeout: 1_000 })
    .then(() =>
      expect(button).toHaveAttribute("data-pr-ready-to-merge", expected, { timeout: 5_000 }),
    )
    .catch(async () => {
      // The topbar PR button hydrates from task-pr state that can arrive via a
      // github.task_pr.updated WS event. If the event was missed during task
      // navigation, a reload rehydrates from the backend state asserted above.
      await page.reload();
      await session.waitForLoad();
    });

  await expect(session.prTopbarButton()).toHaveAttribute("data-pr-ready-to-merge", expected, {
    timeout: 15_000,
  });
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
    await waitForTaskPRFields(apiClient, task.id, { state: "open", checks_state: "success" });

    await expect(kanban.taskCardInColumn("CI Skipped Task", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    // The global AppSidebar also renders a PRTaskIcon per task, so scope to the
    // kanban board to target the card icon this test asserts on.
    const icon = kanban.board.getByTestId(`pr-task-icon-${task.id}`);
    await expect(icon).toBeVisible({ timeout: 15_000 });

    // No reviews, so ready-to-merge must be false; badge should not be yellow.
    await expect(icon).toHaveAttribute("data-pr-ready-to-merge", "false");
    await expect(icon).not.toHaveClass(/text-yellow-500/);

    // Open the task to verify topbar button mirrors the state.
    await kanban.taskCardInColumn("CI Skipped Task", doneStep.id).click();
    await expect(testPage).toHaveURL(/\/[st]\//, { timeout: 15_000 });
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expectTopbarReadyState(testPage, session, "false");
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
    await waitForTaskPRFields(apiClient, task.id, {
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "clean",
    });

    await expect(kanban.taskCardInColumn("Ready To Merge Task", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    // The global AppSidebar also renders a PRTaskIcon per task, so scope to the
    // kanban board to target the card icon this test asserts on.
    const icon = kanban.board.getByTestId(`pr-task-icon-${task.id}`);
    await expect(icon).toBeVisible({ timeout: 15_000 });
    await expect(icon).toHaveAttribute("data-pr-ready-to-merge", "true");

    await kanban.taskCardInColumn("Ready To Merge Task", doneStep.id).click();
    await expect(testPage).toHaveURL(/\/[st]\//, { timeout: 15_000 });
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expectTopbarReadyState(testPage, session, "true");
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
    await waitForTaskPRFields(apiClient, task.id, {
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "blocked",
    });

    await expect(kanban.taskCardInColumn("Blocked Task", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    // The global AppSidebar also renders a PRTaskIcon per task, so scope to the
    // kanban board to target the card icon this test asserts on.
    const icon = kanban.board.getByTestId(`pr-task-icon-${task.id}`);
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
    await waitForTaskPRFields(apiClient, task.id, {
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "blocked",
      pending_review_count: 1,
    });

    await expect(kanban.taskCardInColumn("Awaiting Review Task", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    // The global AppSidebar also renders a PRTaskIcon per task, so scope to the
    // kanban board to target the card icon this test asserts on.
    const icon = kanban.board.getByTestId(`pr-task-icon-${task.id}`);
    await expect(icon).toBeVisible({ timeout: 15_000 });
    await expect(icon).toHaveAttribute("data-pr-ready-to-merge", "false");
    await expect(icon).toHaveClass(/text-sky-400/);
    await expect(icon).not.toHaveClass(/text-emerald-400/);
    await expect(icon).not.toHaveClass(/text-green-500/);
  });

  test("renders readable task PR summary", async ({ testPage, apiClient, seedData, prCapture }) => {
    test.setTimeout(120_000);

    const taskTitle = "Readable PR Summary Task";
    const prTitle =
      "Improve pull request status readability with a clear title and separate review, CI, and merge rows";
    const { task } = await seedBadgeTest(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      taskTitle,
    );
    const settings = await apiClient.getUserSettings();
    const sidebarViews = settings.settings.sidebar_views as Array<Record<string, unknown>>;
    await apiClient.saveUserSettings({
      sidebar_views: sidebarViews.map((view) => ({
        ...view,
        task_row: {
          details_enabled: true,
          detail_order: ["relative_time", "repository", "pull_request_number"],
          visible_details: ["relative_time", "repository", "pull_request_number"],
          trailing: "change_request_status",
        },
      })),
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 2966,
      pr_url: "https://github.com/testorg/testrepo/pull/2966",
      pr_title: prTitle,
      head_branch: "feat/readable-pr-summary",
      base_branch: "main",
      author_login: "test-user",
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "clean",
    });
    await waitForTaskPRFields(apiClient, task.id, {
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "clean",
    });

    const taskRow = testPage.getByTestId("sidebar-task-item").filter({ hasText: taskTitle });
    await expect(taskRow).toBeVisible({ timeout: 15_000 });
    const trailingStatus = taskRow.getByTestId("sidebar-task-change-request-status");
    await expect(trailingStatus).toBeVisible();
    const icon = taskRow.getByTestId(`pr-task-icon-${task.id}`);
    await expect(icon).toHaveAttribute("data-pr-ready-to-merge", "true");
    await icon.hover();

    const summary = visibleTaskPRSummary(testPage);
    await expect(summary).toBeVisible();
    await expect(summary.getByTestId("pr-task-status-number")).toHaveText("PR #2966");
    const title = summary.getByTestId("pr-task-status-title");
    await expect(title).toHaveText(prTitle);

    const reviewRow = summary.getByTestId("pr-task-status-review");
    await expect(reviewRow).toContainText("Review");
    await expect(reviewRow).toContainText("Approved");
    const ciRow = summary.getByTestId("pr-task-status-ci");
    await expect(ciRow).toContainText("CI");
    await expect(ciRow).toContainText("Passed");
    const mergeRow = summary.getByTestId("pr-task-status-merge");
    await expect(mergeRow).toContainText("Merge");
    await expect(mergeRow).toContainText("Ready to merge");
    await prCapture.screenshot("readable-task-pr-summary", {
      caption: "Structured pull request summary in the task sidebar",
    });

    const [summaryBox, titleBox] = await Promise.all([summary.boundingBox(), title.boundingBox()]);
    expect(summaryBox).not.toBeNull();
    expect(titleBox).not.toBeNull();
    expect(titleBox!.height).toBeGreaterThan(24);
    const viewport = testPage.viewportSize();
    expect(viewport).not.toBeNull();
    expect(summaryBox!.x).toBeGreaterThanOrEqual(0);
    expect(summaryBox!.y).toBeGreaterThanOrEqual(0);
    expect(summaryBox!.x + summaryBox!.width).toBeLessThanOrEqual(viewport!.width);
    expect(summaryBox!.y + summaryBox!.height).toBeLessThanOrEqual(viewport!.height);
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);

    await taskRow.focus();
    // The sidebar no longer mounts the task-title preview, so the first tab
    // after the row reaches the PR badge.
    await testPage.keyboard.press("Tab");
    await expect(icon).toBeFocused();
    const focusedSummary = visibleTaskPRSummary(testPage);
    await expect(focusedSummary).toBeVisible();
    await expect(focusedSummary.getByTestId("pr-task-status-title")).toHaveText(prTitle);

    await testPage.mouse.move(viewport!.width - 1, viewport!.height - 1);
    await expect(focusedSummary).toBeVisible();
    await testPage.keyboard.press("Escape");
    await expect(focusedSummary).toBeHidden();
    await apiClient.mockGitHubAssociateTaskPR({
      workspace_id: seedData.workspaceId,
      task_id: task.id,
      owner: "testorg",
      repo: "api",
      pr_number: 2967,
      pr_url: "https://github.com/testorg/api/pull/2967",
      pr_title: "Resolve the failing API checks",
      head_branch: "fix/api-checks",
      base_branch: "main",
      author_login: "test-user",
      state: "open",
      review_state: "changes_requested",
      checks_state: "failure",
      mergeable_state: "dirty",
    });
    await expect(icon).toHaveAttribute("data-pr-count", "2", { timeout: 15_000 });
    await icon.hover();

    const multiSummary = visibleTaskPRSummary(testPage);
    const entries = multiSummary.getByTestId("pr-task-status-entry");
    await expect(entries).toHaveCount(2);
    await expect(entries.nth(0).getByTestId("pr-task-status-number")).toHaveText("PR #2966");
    await expect(entries.nth(1).getByTestId("pr-task-status-number")).toHaveText("PR #2967");
    await expect(entries.nth(1).getByTestId("pr-task-status-title")).toHaveText(
      "Resolve the failing API checks",
    );
  });
});
