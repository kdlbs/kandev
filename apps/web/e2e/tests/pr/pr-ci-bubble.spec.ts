import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { Page } from "@playwright/test";

async function seedCITest(
  apiClient: ApiClient,
  workspaceId: string,
  agentProfileId: string,
  repositoryId: string,
  title: string,
) {
  const workflow = await apiClient.createWorkflow(workspaceId, `${title} Workflow`);
  const inboxStep = await apiClient.createWorkflowStep(workflow.id, "Inbox", 0);
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
  return { workflow, inboxStep, task };
}

function basePR(taskId: string, workspaceId: string, repositoryId: string, number: number) {
  return {
    task_id: taskId,
    workspace_id: workspaceId,
    repository_id: repositoryId,
    owner: "testorg",
    repo: "testrepo",
    pr_number: number,
    pr_url: `https://github.com/testorg/testrepo/pull/${number}`,
    pr_title: "CI bubble",
    head_branch: "feat/ci-bubble",
    base_branch: "main",
    author_login: "test-user",
    state: "open",
    head_sha: `head-${number}`,
  } as const;
}

async function seedCheck(
  apiClient: ApiClient,
  prNumber: number,
  conclusion: "success" | "failure" | "skipped",
) {
  await apiClient.mockGitHubSeedPRFeedback({
    owner: "testorg",
    repo: "testrepo",
    pr_number: prNumber,
    checks: [
      {
        name: `ci-${conclusion}`,
        source: "check_run",
        status: "completed",
        conclusion,
        html_url: "",
        output: "",
      },
    ],
  });
}

async function waitForCIState(
  apiClient: ApiClient,
  workspaceId: string,
  taskId: string,
  expected: string,
) {
  await expect
    .poll(
      async () => {
        const response = await apiClient.listTasks(workspaceId);
        return response.tasks.find((candidate) => candidate.id === taskId)?.status_summary
          ?.pull_request?.ci_state;
      },
      { timeout: 15_000, message: "Expected the compact CI projection to converge" },
    )
    .toBe(expected);
}

function sidebarIcon(page: Page, taskId: string, title: string) {
  return page
    .getByTestId("app-sidebar")
    .getByTestId("sidebar-task-item")
    .filter({ hasText: title })
    .getByTestId(`pr-task-icon-${taskId}`);
}

test.describe("PR CI bubble", () => {
  test.afterEach(async ({ apiClient, seedData }) => {
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: seedData.workflowId,
      enable_preview_on_click: false,
    });
  });

  test("shows a draft CI failure before hover and clears on success", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    const title = "Draft CI bubble";
    const { task } = await seedCITest(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      title,
    );
    await apiClient.mockGitHubAssociateTaskPR({
      ...basePR(task.id, seedData.workspaceId, seedData.repositoryId, 601),
      checks_state: "failure",
      mergeable_state: "draft",
    });
    await seedCheck(apiClient, 601, "failure");
    await waitForCIState(apiClient, seedData.workspaceId, task.id, "failed");

    await testPage.goto("/tasks");
    const icon = sidebarIcon(testPage, task.id, title);
    await expect(icon).toBeVisible({ timeout: 15_000 });
    await expect(icon).toHaveClass(/text-muted-foreground/);
    await expect(icon.getByTestId("pr-ci-bubble-failed")).toBeVisible();
    await expect(icon).toHaveAttribute("aria-label", /Checks failed/);
    await prCapture.screenshot("desktop-draft-ci-failure-bubble", {
      caption: "Draft task keeps the muted PR glyph and shows a separate red CI bubble.",
    });

    await icon.hover();
    const summary = testPage
      .locator('[data-slot="tooltip-content"]:not([data-state="closed"])')
      .getByTestId("pr-task-summary-scroll-body")
      .getByTestId("pr-task-status-summary")
      .filter({ hasText: "PR #601" })
      .first();
    await expect(summary.getByTestId("pr-task-status-ci")).toContainText("Failed");

    await apiClient.mockGitHubAssociateTaskPR({
      ...basePR(task.id, seedData.workspaceId, seedData.repositoryId, 601),
      checks_state: "success",
      mergeable_state: "draft",
    });
    await seedCheck(apiClient, 601, "success");
    await waitForCIState(apiClient, seedData.workspaceId, task.id, "none");
    await expect(icon.getByTestId("pr-ci-bubble-failed")).toHaveCount(0);
    await expect(icon).toHaveClass(/text-muted-foreground/);
  });

  test("keeps conflict, CI, and automation visible together", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    const title = "Conflicted CI bubble";
    const { task } = await seedCITest(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      title,
    );
    const pr = basePR(task.id, seedData.workspaceId, seedData.repositoryId, 602);
    await apiClient.mockGitHubAssociateTaskPR({
      ...pr,
      checks_state: "failure",
      mergeable_state: "dirty",
      has_merge_conflicts: true,
    });
    await seedCheck(apiClient, 602, "failure");
    await apiClient.updateTaskCIAutomationOptions(task.id, {
      repository_id: seedData.repositoryId,
      pr_number: 602,
      auto_fix_enabled: true,
      auto_merge_enabled: true,
    });
    await waitForCIState(apiClient, seedData.workspaceId, task.id, "failed");
    await expect
      .poll(async () => {
        const response = await apiClient.listTasks(seedData.workspaceId);
        const pullRequest = response.tasks.find((candidate) => candidate.id === task.id)
          ?.status_summary?.pull_request;
        return {
          conflicts: pullRequest?.has_merge_conflicts === true,
          autoFix: pullRequest?.auto_fix_enabled === true,
          autoMerge: pullRequest?.auto_merge_enabled === true,
        };
      })
      .toMatchObject({ conflicts: true, autoFix: true, autoMerge: true });

    await testPage.goto("/tasks");
    const icon = sidebarIcon(testPage, task.id, title);
    await expect(icon).toBeVisible({ timeout: 15_000 });
    await expect(icon.getByTestId("pr-ci-bubble-failed")).toBeVisible();
    await expect(icon.getByTestId("pr-merge-conflict-warning")).toBeVisible();
    await expect(icon.getByTestId("pr-task-automation-auto-fix")).toBeVisible();
    await expect(icon.getByTestId("pr-task-automation-auto-merge")).toBeVisible();
    await expect(icon).toHaveAttribute("aria-label", /Checks failed/);
    await expect(icon).toHaveAttribute("aria-label", /Conflicts/);

    const geometry = await icon.evaluate((element) => {
      const bubble = element.querySelector<HTMLElement>('[data-testid="pr-ci-bubble-failed"]');
      const conflict = element.querySelector<HTMLElement>(
        '[data-testid="pr-merge-conflict-warning"]',
      );
      if (!bubble || !conflict) return null;
      const bubbleBox = bubble.getBoundingClientRect();
      const conflictBox = conflict.getBoundingClientRect();
      return {
        bubble: { left: bubbleBox.left, right: bubbleBox.right, top: bubbleBox.top },
        conflict: { left: conflictBox.left, right: conflictBox.right, top: conflictBox.top },
      };
    });
    expect(geometry).not.toBeNull();
    expect(geometry!.conflict.left).toBeGreaterThanOrEqual(geometry!.bubble.right - 1);
    expect(geometry!.conflict.top).toBeGreaterThanOrEqual(geometry!.bubble.top - 1);
    await prCapture.screenshot("desktop-ci-conflict-automation", {
      caption: "CI bubble, inline conflict triangle, and automation dots stay independent.",
    });
  });

  test("explains all-skipped checks without a bubble or a passed claim", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const title = "All skipped CI";
    const { task } = await seedCITest(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      title,
    );
    const pr = basePR(task.id, seedData.workspaceId, seedData.repositoryId, 603);
    await apiClient.mockGitHubAssociateTaskPR(pr);
    await apiClient.mockGitHubSeedPRFeedback({
      owner: "testorg",
      repo: "testrepo",
      pr_number: 603,
      checks: [
        {
          name: "docs",
          source: "check_run",
          status: "completed",
          conclusion: "skipped",
          html_url: "",
          output: "",
        },
        {
          name: "lint",
          source: "check_run",
          status: "completed",
          conclusion: "skipped",
          html_url: "",
          output: "",
        },
      ],
    });
    await waitForCIState(apiClient, seedData.workspaceId, task.id, "none");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.hoverPRTopbar();
    await expect
      .poll(async () => (await apiClient.listTaskPRs(task.id))[0]?.check_summary?.skipped ?? 0, {
        timeout: 30_000,
        message: "Expected selected skipped checks to persist",
      })
      .toBe(2);

    await testPage.goto("/tasks");
    const icon = sidebarIcon(testPage, task.id, title);
    await expect(icon).toBeVisible({ timeout: 15_000 });
    await expect(icon.getByTestId("pr-ci-bubble-failed")).toHaveCount(0);
    await expect(icon.getByTestId("pr-ci-bubble-in-progress")).toHaveCount(0);

    await icon.hover();
    const summary = testPage
      .locator('[data-slot="tooltip-content"]:not([data-state="closed"])')
      .getByTestId("pr-task-summary-scroll-body")
      .getByTestId("pr-task-status-summary")
      .filter({ hasText: "PR #603" })
      .first();
    const ciRow = summary.getByTestId("pr-task-status-ci");
    await expect(ciRow).toContainText("All checks skipped");
    await expect(ciRow).not.toContainText("Passed");
    await expect(ciRow).toContainText("Last detailed observation");
  });

  test("mirrors draft CI failure in the task topbar", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(120_000);
    const title = "Topbar draft CI bubble";
    const { task } = await seedCITest(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      title,
    );
    await apiClient.mockGitHubAssociateTaskPR({
      ...basePR(task.id, seedData.workspaceId, seedData.repositoryId, 604),
      checks_state: "failure",
      mergeable_state: "draft",
    });
    await seedCheck(apiClient, 604, "failure");
    await waitForCIState(apiClient, seedData.workspaceId, task.id, "failed");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const button = session.prTopbarButton();
    await expect(button).toBeVisible({ timeout: 15_000 });
    await expect(button.getByTestId("pr-ci-bubble-failed")).toBeVisible();
    await expect(button).toHaveAttribute("aria-label", /Checks failed/);
  });
});

test.describe("PR CI bubble on Kanban", () => {
  test("keeps a passing draft card free of bubbles", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(120_000);
    const title = "Passing draft CI";
    const { task } = await seedCITest(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      title,
    );
    await apiClient.mockGitHubAssociateTaskPR({
      ...basePR(task.id, seedData.workspaceId, seedData.repositoryId, 605),
      checks_state: "success",
      mergeable_state: "draft",
    });
    await seedCheck(apiClient, 605, "success");
    await waitForCIState(apiClient, seedData.workspaceId, task.id, "none");

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const card = kanban.board.getByTestId(`pr-task-icon-${task.id}`);
    await expect(card).toBeVisible({ timeout: 15_000 });
    await expect(card.getByTestId("pr-ci-bubble-failed")).toHaveCount(0);
    await expect(card.getByTestId("pr-ci-bubble-in-progress")).toHaveCount(0);
  });
});
