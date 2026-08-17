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
  await seedQueuedPR(apiClient, task.id);

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await testPage.reload();
  await session.waitForLoad();
  await seedQueuedPR(apiClient, task.id);

  await session.hoverPRTopbar();
  const popover = session.prTopbarPopover();
  const compactMerge = popover.getByRole("button", { name: "Merge PR" });
  await expect(compactMerge).toBeVisible({ timeout: 15_000 });
  await session.prDetailTab().click();
  await seedQueuedPR(apiClient, task.id);

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
    review_count: 2,
    pending_review_count: 0,
    required_reviews: 2,
    checks_total: 3,
    checks_passing: 3,
  });
  await apiClient.mockGitHubSetMergeOutcome(OWNER, REPO, PR_NUMBER, "queued");
}
