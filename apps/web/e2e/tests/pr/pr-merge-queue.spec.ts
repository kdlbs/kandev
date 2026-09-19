import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

const OWNER = "northstar-labs";
const REPO = "relay-console";
const PR_NUMBER = 842;

test("adds an eligible GitHub PR to its merge queue", async ({ testPage, apiClient, seedData }) => {
  test.setTimeout(120_000);
  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("maya-chen");
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Ship resilient deployment controls",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await seedEligiblePR(apiClient, task.id);

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await testPage.reload();
  await session.waitForLoad();
  await seedEligiblePR(apiClient, task.id);

  await session.hoverPRTopbar();
  const popover = session.prTopbarPopover();
  const compactMerge = popover.getByRole("button", { name: "Merge PR" });
  await expect(compactMerge).toBeVisible({ timeout: 15_000 });
  // The persisted layout can reopen the detail panel as a dock tab, but a
  // reload is also allowed to leave it closed. The topbar button is the
  // canonical fallback for opening the same panel.
  const detailTab = session.prDetailTab();
  if (await detailTab.isVisible()) {
    await detailTab.click();
  } else {
    await session.prTopbarButton().click();
  }
  await seedEligiblePR(apiClient, task.id);

  const detail = testPage.getByTestId("change-request-detail");
  const merge = detail.getByRole("button", { name: "Merge PR" });
  await expect(merge).toBeVisible({ timeout: 15_000 });
  const [response] = await Promise.all([
    testPage.waitForResponse((candidate) => candidate.url().includes(`/${PR_NUMBER}/merge`)),
    merge.click(),
  ]);
  const responseBody = await response.json();
  expect(response.ok(), JSON.stringify(responseBody)).toBe(true);
  expect(responseBody).toMatchObject({ status: "queued" });
  await expect(testPage.getByText("PR added to merge queue", { exact: true })).toBeVisible();
  await expect(detail.getByTestId("pr-merge-button")).toHaveCount(0);
});

test("surfaces queued PR metadata across desktop status surfaces", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("maya-chen");
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Ship resilient deployment controls",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await seedQueuedPR(apiClient, task.id);

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await testPage.reload();
  await session.waitForLoad();
  await seedQueuedPR(apiClient, task.id);

  const taskIcon = testPage.getByTestId("pr-task-icon-" + task.id);
  await expect(taskIcon).toBeVisible({ timeout: 15_000 });
  await expect(taskIcon).toHaveClass(/text-\[#966600\]/);
  await taskIcon.hover();
  const taskSummary = testPage
    .locator('[data-slot="tooltip-content"]:not([data-state="closed"])')
    .getByTestId("pr-task-status-summary")
    .first();
  await expect(taskSummary).toBeVisible({ timeout: 5_000 });
  await expect(taskSummary.getByTestId("pr-task-status-merge")).toContainText("Queued");
  await expect(taskSummary.getByTestId("pr-task-status-merge-detail")).toContainText("Position 2");
  await expect(taskSummary.getByTestId("pr-task-status-merge-detail")).toContainText("2 minutes");
  await prCapture.screenshot("desktop-pr-merge-queue-status", {
    caption: "Desktop pull request queue status with position and estimate",
  });

  await session.hoverPRTopbar();
  const popover = session.prTopbarPopover();
  await expect(popover.getByTestId("pr-merge-queue-status")).toContainText("Queued");
  await expect(popover.getByTestId("pr-merge-queue-status")).toContainText("Position 2");
  await expect(popover.getByTestId("pr-merge-queue-status")).toContainText("2 minutes");

  await session.hoverPRChip();
  const compactPopover = session.prChipPopover();
  await expect(compactPopover.getByTestId("pr-merge-queue-status")).toContainText("Queued");
  await expect(compactPopover.getByTestId("pr-merge-queue-status")).toContainText("Position 2");
  await expect(compactPopover.getByTestId("pr-merge-queue-status")).toContainText("2 minutes");

  // The default layout can reopen PR details as a dock tab or through the
  // existing topbar affordance. Keep the display scenario independent from
  // the preceding merge-action test's accepted-state layout.
  const detailTab = session.prDetailTab();
  if (await detailTab.isVisible()) {
    await detailTab.click();
  } else {
    await session.prTopbarButton().click();
  }
  await seedQueuedPR(apiClient, task.id);

  const detail = testPage.getByTestId("change-request-detail");
  await expect(detail).toBeVisible({ timeout: 15_000 });
  await expect(detail.getByTestId("pr-merge-queue-status")).toContainText("Queued");
  await expect(detail.getByTestId("pr-merge-queue-status")).toContainText("Position 2");
  await expect(detail.getByTestId("pr-merge-queue-status")).toContainText("2 minutes");
});

async function seedQueuedPR(apiClient: ApiClient, taskId: string) {
  await apiClient.mockGitHubAddRepos(OWNER, [
    { full_name: `${OWNER}/${REPO}`, owner: OWNER, name: REPO },
  ]);
  await apiClient.mockGitHubAssociateTaskPR({
    task_id: taskId,
    owner: OWNER,
    repo: REPO,
    pr_number: PR_NUMBER,
    pr_url: `https://github.com/${OWNER}/${REPO}/pull/${PR_NUMBER}`,
    pr_title: "Make deployment rollbacks deterministic",
    head_branch: "feat/resilient-rollbacks",
    base_branch: "main",
    author_login: "maya-chen",
    state: "open",
    review_state: "approved",
    checks_state: "success",
    mergeable_state: "blocked",
    merge_queue_state: "queued",
    merge_queue_position: 2,
    merge_queue_estimated_time_to_merge_seconds: 61,
    review_count: 2,
    pending_review_count: 0,
    required_reviews: 2,
    checks_total: 3,
    checks_passing: 3,
  });
  await apiClient.mockGitHubSetMergeOutcome(OWNER, REPO, PR_NUMBER, "queued");
}

async function seedEligiblePR(apiClient: ApiClient, taskId: string) {
  await apiClient.mockGitHubAddRepos(OWNER, [
    { full_name: `${OWNER}/${REPO}`, owner: OWNER, name: REPO },
  ]);
  await apiClient.mockGitHubAssociateTaskPR({
    task_id: taskId,
    owner: OWNER,
    repo: REPO,
    pr_number: PR_NUMBER,
    pr_url: `https://github.com/${OWNER}/${REPO}/pull/${PR_NUMBER}`,
    pr_title: "Make deployment rollbacks deterministic",
    head_branch: "feat/resilient-rollbacks",
    base_branch: "main",
    author_login: "maya-chen",
    state: "open",
    review_state: "approved",
    checks_state: "success",
    mergeable_state: "blocked",
    review_count: 2,
    pending_review_count: 0,
    required_reviews: 2,
    checks_total: 3,
    checks_passing: 3,
  });
  await apiClient.mockGitHubSetMergeOutcome(OWNER, REPO, PR_NUMBER, "queued");
}
