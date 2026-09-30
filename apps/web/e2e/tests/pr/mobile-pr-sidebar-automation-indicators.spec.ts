import { test, expect, type SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

const OWNER = "testorg";
const REPO = "testrepo";
const PR_NUMBER = 190;

async function seedSidebarAutomation(
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<{ navigationTaskId: string; targetTaskId: string }> {
  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("test-user");
  const stepOptions = {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  };
  const navigationTask = await apiClient.seedTask(
    seedData.workspaceId,
    "Mobile automation navigation",
    stepOptions,
  );
  const targetTask = await apiClient.seedTask(
    seedData.workspaceId,
    "Mobile automation target",
    stepOptions,
  );
  const headSHA = "mobile-sidebar-workflow-head";
  await apiClient.mockGitHubAssociateTaskPR({
    task_id: targetTask.task_id,
    workspace_id: seedData.workspaceId,
    repository_id: seedData.repositoryId,
    owner: OWNER,
    repo: REPO,
    pr_number: PR_NUMBER,
    pr_url: `https://github.com/${OWNER}/${REPO}/pull/${PR_NUMBER}`,
    pr_title: "Mobile sidebar automation indicators",
    head_branch: "feat/mobile-sidebar-automation-indicators",
    base_branch: "main",
    author_login: "test-user",
    state: "open",
    review_state: "approved",
    checks_state: "success",
    // Keep auto-merge enabled for the indicator while showing approval alone.
    mergeable_state: "clean",
    has_merge_conflicts: false,
    head_sha: headSHA,
    head_repo_owner: "contributor",
    head_repo_name: "demo-fork",
  });
  await apiClient.mockGitHubSeedPRFeedback({
    owner: OWNER,
    repo: REPO,
    pr_number: PR_NUMBER,
    workflow_runs: [
      {
        id: 3919001,
        run_attempt: 1,
        workflow_id: 3919,
        name: "Mobile fork checks",
        event: "pull_request",
        status: "completed",
        conclusion: "action_required",
        head_sha: headSHA,
        head_branch: "feat/mobile-sidebar-automation-indicators",
        head_repo_owner: "contributor",
        head_repo_name: "demo-fork",
        html_url: "https://github.com/testorg/testrepo/actions/runs/3919001",
        pull_requests: [
          {
            number: PR_NUMBER,
            head_sha: headSHA,
            head_branch: "feat/mobile-sidebar-automation-indicators",
            head_repo_owner: "contributor",
            head_repo_name: "demo-fork",
          },
        ],
      },
    ],
    workflow_jobs: [{ run_id: 3919001, run_attempt: 1, jobs: [] }],
  });
  await apiClient.updateTaskCIAutomationOptions(targetTask.task_id, {
    repository_id: seedData.repositoryId,
    pr_number: PR_NUMBER,
    auto_fix_enabled: true,
    auto_merge_enabled: true,
  });
  return { navigationTaskId: navigationTask.task_id, targetTaskId: targetTask.task_id };
}

test.describe("Mobile sidebar PR automation indicators", () => {
  test("shows touch indicators, details, and terminal-state cleanup", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    const { navigationTaskId, targetTaskId } = await seedSidebarAutomation(apiClient, seedData);
    await apiClient.seedTaskSession(targetTaskId, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
    });

    await testPage.goto(`/t/${targetTaskId}`);
    const targetSession = new SessionPage(testPage);
    await targetSession.waitForLoad();
    await targetSession.tapPRStatusChip();
    const workflowNotice = targetSession
      .prStatusChipPopoverInner()
      .getByTestId("pr-workflow-attention");
    await expect(workflowNotice).toContainText("Awaiting maintainer approval");
    await expect
      .poll(
        async () => {
          const pullRequests = await apiClient.listTaskPRs(targetTaskId);
          const approvalPR = pullRequests.find(
            (pullRequest) => pullRequest.pr_number === PR_NUMBER,
          );
          return {
            has_merge_conflicts: approvalPR?.has_merge_conflicts === true,
            workflow_attention: approvalPR?.workflow_attention?.state,
          };
        },
        { timeout: 15_000 },
      )
      .toMatchObject({
        has_merge_conflicts: false,
        workflow_attention: "approval_required",
      });

    await apiClient.mockGitHubAssociateTaskPR({
      task_id: targetTaskId,
      workspace_id: seedData.workspaceId,
      repository_id: seedData.repositoryId,
      owner: OWNER,
      repo: REPO,
      pr_number: PR_NUMBER + 1,
      pr_url: `https://github.com/${OWNER}/${REPO}/pull/${PR_NUMBER + 1}`,
      pr_title: "Mobile sidebar second pull request",
      head_branch: "feat/mobile-sidebar-second-pr",
      base_branch: "main",
      author_login: "test-user",
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "clean",
      head_sha: "mobile-sidebar-second-head",
    });

    await expect
      .poll(
        async () => {
          const response = await apiClient.listTasks(seedData.workspaceId);
          const pullRequest = response.tasks.find((task) => task.id === targetTaskId)
            ?.status_summary?.pull_request;
          return {
            auto_fix_enabled: pullRequest?.auto_fix_enabled,
            auto_merge_enabled: pullRequest?.auto_merge_enabled,
            has_merge_conflicts: pullRequest?.has_merge_conflicts === true,
            workflow_approval_required: pullRequest?.workflow_approval_required,
          };
        },
        { timeout: 15_000 },
      )
      .toMatchObject({
        auto_fix_enabled: true,
        auto_merge_enabled: true,
        has_merge_conflicts: false,
        workflow_approval_required: true,
      });

    await testPage.goto(`/t/${navigationTaskId}`);
    await new SessionPage(testPage).waitForLoad();
    await testPage.reload();
    await new SessionPage(testPage).waitForLoad();
    await testPage.getByTestId("mobile-task-picker-trigger").tap();

    const sheet = testPage.getByRole("dialog", { name: "Tasks" });
    await waitForFiniteAnimations(sheet);
    const targetRow = sheet.locator(`[data-task-row-id="${targetTaskId}"]`);
    await expect(targetRow).toBeVisible();
    const icon = targetRow.getByTestId(`pr-task-icon-${targetTaskId}`);
    await expect(icon).toBeVisible();
    await expect(icon.getByTestId("pr-task-automation-auto-fix")).toBeVisible();
    await expect(icon.getByTestId("pr-task-automation-auto-merge")).toBeVisible();
    await expect(icon).toHaveAttribute("aria-label", /Awaiting maintainer approval/);
    await expect(icon).not.toHaveAttribute("aria-label", /Conflicts/);
    const approvalWarning = icon.getByTestId("pr-workflow-approval-warning");
    await expect(approvalWarning).toBeVisible();
    await expect(icon.getByTestId("pr-merge-conflict-warning")).toHaveCount(0);
    const approvalContainment = await targetRow.evaluate((row) => {
      const warning = row.querySelector<HTMLElement>(
        '[data-testid="pr-workflow-approval-warning"]',
      );
      if (!warning) return null;
      const rowBox = row.getBoundingClientRect();
      const warningBox = warning.getBoundingClientRect();
      return {
        row: { left: rowBox.left, top: rowBox.top, right: rowBox.right, bottom: rowBox.bottom },
        warning: {
          left: warningBox.left,
          top: warningBox.top,
          right: warningBox.right,
          bottom: warningBox.bottom,
        },
      };
    });
    expect(approvalContainment).not.toBeNull();
    expect(approvalContainment!.warning.left).toBeGreaterThanOrEqual(
      approvalContainment!.row.left - 1,
    );
    expect(approvalContainment!.warning.top).toBeGreaterThanOrEqual(
      approvalContainment!.row.top - 1,
    );
    expect(approvalContainment!.warning.right).toBeLessThanOrEqual(
      approvalContainment!.row.right + 1,
    );
    expect(approvalContainment!.warning.bottom).toBeLessThanOrEqual(
      approvalContainment!.row.bottom + 1,
    );
    await assertNoDocumentHorizontalOverflow(testPage);
    await prCapture.screenshot("sidebar-workflow-approval-only-badge-mobile", {
      caption: "Phone task picker shows the amber approval lock with automation indicators.",
    });

    const navigationURL = testPage.url();
    await icon.tap();
    const drawer = testPage.getByTestId(`pr-task-automation-drawer-${targetTaskId}`);
    await expect(drawer).toBeVisible();
    await waitForFiniteAnimations(drawer);
    const automationDetails = drawer.getByTestId("pr-task-automation-details");
    await expect(automationDetails).toBeVisible();
    await expect(automationDetails.getByText(`PR #${PR_NUMBER}`)).toBeVisible();
    await expect(automationDetails.getByText("Auto-fix")).toBeVisible();
    await expect(automationDetails.getByText("Auto-merge")).toBeVisible();
    const statusSummary = drawer.getByTestId("pr-task-status-summary");
    const approvalEntry = statusSummary
      .getByTestId("pr-task-status-entry")
      .filter({ hasText: `PR #${PR_NUMBER}` });
    await expect(approvalEntry.getByText("Awaiting maintainer approval")).toBeVisible();
    await expect(approvalEntry.getByText("Conflicts", { exact: true })).toHaveCount(0);
    await expect(statusSummary.getByTestId("pr-task-status-entry")).toHaveCount(2);
    await assertNoDocumentHorizontalOverflow(testPage);
    await prCapture.screenshot("sidebar-workflow-approval-only-details-mobile", {
      caption: "Phone PR details show approval and automation without leaving the task.",
    });

    await testPage.keyboard.press("Escape");
    await expect(drawer).toHaveCount(0);
    await expect(icon).toBeFocused();
    await expect(testPage).toHaveURL(navigationURL);

    await apiClient.mockGitHubAddPRs([
      {
        number: PR_NUMBER,
        title: "Mobile sidebar automation indicators",
        state: "open",
        head_branch: "feat/mobile-sidebar-automation-indicators",
        head_sha: "mobile-sidebar-workflow-head",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: OWNER,
        repo_name: REPO,
        head_repo_owner: "contributor",
        head_repo_name: "demo-fork",
        mergeable_state: "dirty",
      },
    ]);
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: targetTaskId,
      workspace_id: seedData.workspaceId,
      repository_id: seedData.repositoryId,
      owner: OWNER,
      repo: REPO,
      pr_number: PR_NUMBER,
      pr_url: `https://github.com/${OWNER}/${REPO}/pull/${PR_NUMBER}`,
      pr_title: "Mobile sidebar automation indicators",
      head_branch: "feat/mobile-sidebar-automation-indicators",
      base_branch: "main",
      author_login: "test-user",
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "dirty",
      has_merge_conflicts: true,
      head_sha: "mobile-sidebar-workflow-head",
      head_repo_owner: "contributor",
      head_repo_name: "demo-fork",
    });
    await expect
      .poll(
        async () => {
          const response = await apiClient.listTasks(seedData.workspaceId);
          const pullRequest = response.tasks.find((task) => task.id === targetTaskId)
            ?.status_summary?.pull_request;
          return {
            hasMergeConflicts: pullRequest?.has_merge_conflicts === true,
            workflowApprovalRequired: pullRequest?.workflow_approval_required === true,
          };
        },
        { timeout: 15_000 },
      )
      .toEqual({ hasMergeConflicts: true, workflowApprovalRequired: true });

    await expect(icon.getByTestId("pr-workflow-approval-warning")).toHaveCount(0);
    await expect(icon.getByTestId("pr-merge-conflict-warning")).toBeVisible();
    await expect(icon).toHaveAttribute("aria-label", /Conflicts/);
    await expect(icon).toHaveAttribute("aria-label", /Awaiting maintainer approval/);
    await prCapture.screenshot("sidebar-workflow-conflict-priority-badge-mobile", {
      caption: "Phone task picker gives the red conflict warning priority over the approval lock.",
    });

    await icon.tap();
    const conflictDrawer = testPage.getByTestId(`pr-task-automation-drawer-${targetTaskId}`);
    await expect(conflictDrawer).toBeVisible();
    await waitForFiniteAnimations(conflictDrawer);
    const conflictSummary = conflictDrawer.getByTestId("pr-task-status-summary");
    const conflictEntry = conflictSummary
      .getByTestId("pr-task-status-entry")
      .filter({ hasText: `PR #${PR_NUMBER}` });
    await expect(conflictEntry.getByText("Awaiting maintainer approval")).toBeVisible();
    await expect(conflictEntry.getByText("Conflicts", { exact: true })).toBeVisible();
    await expect(conflictSummary.getByTestId("pr-task-status-entry")).toHaveCount(2);
    await assertNoDocumentHorizontalOverflow(testPage);
    await prCapture.screenshot("sidebar-workflow-conflict-priority-details-mobile", {
      caption: "Phone PR details retain both approval and conflict explanations.",
    });

    await testPage.keyboard.press("Escape");
    await expect(conflictDrawer).toHaveCount(0);
    await expect(icon).toBeFocused();
    await expect(testPage).toHaveURL(navigationURL);

    await apiClient.mockGitHubAssociateTaskPR({
      task_id: targetTaskId,
      workspace_id: seedData.workspaceId,
      repository_id: seedData.repositoryId,
      owner: OWNER,
      repo: REPO,
      pr_number: PR_NUMBER,
      pr_url: `https://github.com/${OWNER}/${REPO}/pull/${PR_NUMBER}`,
      pr_title: "Mobile sidebar automation indicators",
      head_branch: "feat/mobile-sidebar-automation-indicators",
      base_branch: "main",
      author_login: "test-user",
      state: "closed",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "clean",
      has_merge_conflicts: false,
    });
    await expect
      .poll(async () => {
        const response = await apiClient.listTasks(seedData.workspaceId);
        const pullRequest = response.tasks.find((task) => task.id === targetTaskId)?.status_summary
          ?.pull_request;
        return {
          automationEnabled:
            pullRequest?.auto_fix_enabled === true || pullRequest?.auto_merge_enabled === true,
          workflowApprovalRequired: pullRequest?.workflow_approval_required === true,
        };
      })
      .toEqual({ automationEnabled: false, workflowApprovalRequired: false });
    await expect(icon.getByTestId("pr-task-automation-auto-fix")).toHaveCount(0);
    await expect(icon.getByTestId("pr-task-automation-auto-merge")).toHaveCount(0);
    await expect(icon.getByTestId("pr-merge-conflict-warning")).toHaveCount(0);
    await expect(icon.getByTestId("pr-workflow-approval-warning")).toHaveCount(0);
  });

  // @covers AC-UI-PR-TASK-STATUS-SUMMARY-001.23
  test("reaches the final PR and automation details inside the mobile drawer", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    const { navigationTaskId, targetTaskId } = await seedSidebarAutomation(apiClient, seedData);
    for (let number = PR_NUMBER + 1; number <= PR_NUMBER + 4; number += 1) {
      await apiClient.mockGitHubAssociateTaskPR({
        task_id: targetTaskId,
        workspace_id: seedData.workspaceId,
        repository_id: seedData.repositoryId,
        owner: OWNER,
        repo: REPO,
        pr_number: number,
        pr_url: `https://github.com/${OWNER}/${REPO}/pull/${number}`,
        pr_title: `Mobile sidebar overflow fixture ${number}: a long pull request title that wraps across multiple lines in the status drawer`,
        head_branch: `feat/mobile-sidebar-overflow-${number}`,
        base_branch: "main",
        author_login: "test-user",
        state: "open",
        review_state: "approved",
        checks_state: "success",
        mergeable_state: "dirty",
        has_merge_conflicts: true,
      });
    }

    await expect.poll(async () => (await apiClient.listTaskPRs(targetTaskId)).length).toBe(5);
    await testPage.goto(`/t/${navigationTaskId}`);
    await new SessionPage(testPage).waitForLoad();
    await testPage.getByTestId("mobile-task-picker-trigger").tap();

    const sheet = testPage.getByRole("dialog", { name: "Tasks" });
    const targetRow = sheet.locator(`[data-task-row-id="${targetTaskId}"]`);
    const icon = targetRow.getByTestId(`pr-task-icon-${targetTaskId}`);
    await icon.tap();

    const drawer = testPage.getByTestId(`pr-task-automation-drawer-${targetTaskId}`);
    await expect(drawer).toBeVisible();
    await waitForFiniteAnimations(drawer);
    const scrollBody = drawer.locator("[data-vaul-no-drag]");
    const entries = scrollBody.getByTestId("pr-task-status-entry");
    await expect(entries).toHaveCount(5, { timeout: 15_000 });
    const lastEntry = entries.last();
    const automation = scrollBody.getByTestId("pr-task-automation-details");
    const viewport = testPage.viewportSize();
    const drawerBox = await drawer.boundingBox();
    const headerBox = await drawer.locator('[data-slot="drawer-header"]').boundingBox();
    expect(viewport).not.toBeNull();
    expect(drawerBox).not.toBeNull();
    expect(headerBox).not.toBeNull();
    expect(drawerBox!.y).toBeGreaterThanOrEqual(0);
    expect(drawerBox!.y + drawerBox!.height).toBeLessThanOrEqual(viewport!.height);
    expect(await scrollBody.evaluate((element) => element.scrollHeight)).toBeGreaterThan(
      await scrollBody.evaluate((element) => element.clientHeight),
    );

    await lastEntry.scrollIntoViewIfNeeded();
    await expect.poll(() => scrollBody.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
    await expect(lastEntry).toBeInViewport({ ratio: 0.5 });
    await automation.scrollIntoViewIfNeeded();
    await expect(automation).toBeInViewport({ ratio: 0.5 });
    const scrolledHeaderBox = await drawer.locator('[data-slot="drawer-header"]').boundingBox();
    expect(scrolledHeaderBox).not.toBeNull();
    expect(Math.abs(scrolledHeaderBox!.y - headerBox!.y)).toBeLessThanOrEqual(1);
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
    await prCapture.screenshot("sidebar-pr-summary-overflow-mobile", {
      caption:
        "Mobile PR drawer keeps its header fixed while long PR and automation details scroll",
    });
  });
});
