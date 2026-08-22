import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow, requireBox } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";

test("adds a GitHub PR to the merge queue from mobile Review", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await testPage.setViewportSize({ width: 393, height: 852 });
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
  await apiClient.mockGitHubAddRepos("northstar-labs", [
    {
      full_name: "northstar-labs/relay-console",
      owner: "northstar-labs",
      name: "relay-console",
    },
  ]);
  await apiClient.mockGitHubAssociateTaskPR({
    task_id: task.id,
    owner: "northstar-labs",
    repo: "relay-console",
    pr_number: 843,
    pr_url: "https://github.com/northstar-labs/relay-console/pull/843",
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
  await apiClient.mockGitHubSetMergeOutcome("northstar-labs", "relay-console", 843, "queued");

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await testPage.reload();
  await session.waitForLoad();
  await testPage.getByRole("button", { name: "Review", exact: true }).tap();

  const panel = testPage.getByTestId("mobile-review-panel");
  await apiClient.mockGitHubAssociateTaskPR({
    task_id: task.id,
    owner: "northstar-labs",
    repo: "relay-console",
    pr_number: 843,
    pr_url: "https://github.com/northstar-labs/relay-console/pull/843",
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
  const merge = panel.getByRole("button", { name: "Merge PR" });
  await expect(merge).toBeVisible({ timeout: 15_000 });
  expect((await requireBox(merge, "mobile merge queue action")).height).toBeGreaterThanOrEqual(44);
  await assertNoDocumentHorizontalOverflow(testPage);
  await merge.tap();
  await expect(testPage.getByText("PR added to merge queue", { exact: true })).toBeVisible();
});

test("surfaces queued PR metadata in the mobile drawer and Review", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  await testPage.setViewportSize({ width: 393, height: 852 });
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
  await apiClient.mockGitHubAddRepos("northstar-labs", [
    {
      full_name: "northstar-labs/relay-console",
      owner: "northstar-labs",
      name: "relay-console",
    },
  ]);
  await apiClient.mockGitHubAssociateTaskPR({
    task_id: task.id,
    owner: "northstar-labs",
    repo: "relay-console",
    pr_number: 843,
    pr_url: "https://github.com/northstar-labs/relay-console/pull/843",
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
  await apiClient.mockGitHubSetMergeOutcome("northstar-labs", "relay-console", 843, "queued");

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await testPage.reload();
  await session.waitForLoad();
  await session.tapPRStatusChip();
  const drawer = session.prStatusChipPopoverInner();
  await expect(drawer.getByTestId("pr-merge-queue-status")).toContainText("Queued");
  await expect(drawer.getByTestId("pr-merge-queue-status")).toContainText("Position 2");
  await expect(drawer.getByTestId("pr-merge-queue-status")).toContainText("2 minutes");
  await prCapture.screenshot("mobile-pr-merge-queue-status", {
    caption: "Mobile pull request queue status in the touch drawer",
  });
  await session.prStatusChipDrawerClose().tap();
  await testPage.getByRole("button", { name: "Review", exact: true }).tap();

  const panel = testPage.getByTestId("mobile-review-panel");
  await apiClient.mockGitHubAssociateTaskPR({
    task_id: task.id,
    owner: "northstar-labs",
    repo: "relay-console",
    pr_number: 843,
    pr_url: "https://github.com/northstar-labs/relay-console/pull/843",
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
  const queueStatus = panel.getByTestId("pr-merge-queue-status");
  await expect(queueStatus).toContainText("Queued");
  await expect(queueStatus).toContainText("Position 2");
  await expect(queueStatus).toContainText("2 minutes");
  await assertNoDocumentHorizontalOverflow(testPage);
});
