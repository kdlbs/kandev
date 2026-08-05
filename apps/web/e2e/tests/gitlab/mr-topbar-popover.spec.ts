import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { GITLAB_HOST, GITLAB_PROJECT } from "../../helpers/gitlab";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

const MR_IID = 320;
const PIPELINE_ID = 9001;

async function seedTaskWithPopoverFixture(apiClient: ApiClient, seedData: SeedData, title: string) {
  await apiClient.configureGitLab(seedData.workspaceId, GITLAB_HOST);
  const now = new Date().toISOString();
  await apiClient.mockGitLabAddMRs(seedData.workspaceId, GITLAB_PROJECT, [
    {
      iid: MR_IID,
      id: MR_IID + 10_000,
      project_id: 101,
      title: "Popover fixture MR",
      url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${MR_IID}`,
      web_url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${MR_IID}`,
      state: "open",
      head_branch: "feature/popover",
      head_sha: "sha-popover",
      base_branch: "main",
      author_username: "contributor",
      project_namespace: "platform",
      project_path: GITLAB_PROJECT,
      body: "Popover fixture description",
      draft: false,
      merge_status: "can_be_merged",
      has_conflicts: false,
      additions: 10,
      deletions: 2,
      reviewers: [
        { id: 1, username: "alice", name: "Alice Reviewer", type: "user" },
        { id: 2, username: "bob", name: "Bob Reviewer", type: "user" },
      ],
      assignees: [],
      created_at: now,
      updated_at: now,
    },
  ]);
  await apiClient.mockGitLabAddPipelines(seedData.workspaceId, GITLAB_PROJECT, [
    {
      id: PIPELINE_ID,
      iid: 1,
      status: "failed",
      source: "push",
      ref: "feature/popover",
      sha: "sha-popover",
      web_url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/pipelines/${PIPELINE_ID}`,
      jobs_total: 10,
      jobs_passing: 6,
    },
  ]);
  await apiClient.mockGitLabAddPipelineJobs(
    seedData.workspaceId,
    PIPELINE_ID,
    Array.from({ length: 10 }, (_, i) => ({
      id: i + 1,
      name: `job-${i + 1}`,
      stage: i < 5 ? "test" : "build",
      status: i < 6 ? "success" : "failed",
      allow_failure: false,
    })),
  );
  await apiClient.mockGitLabAddApprovals(
    seedData.workspaceId,
    GITLAB_PROJECT,
    MR_IID,
    [{ username: "alice", created_at: now }],
    2,
  );
  await apiClient.mockGitLabAddDiscussions(seedData.workspaceId, GITLAB_PROJECT, MR_IID, [
    {
      id: "thread-popover-1",
      resolvable: true,
      resolved: false,
      path: "src/main.ts",
      line: 5,
      notes: [
        {
          id: 9001,
          author: "alice",
          body: "One more thing to fix.",
          created_at: now,
          updated_at: now,
        },
      ],
      created_at: now,
      updated_at: now,
    },
  ]);

  await apiClient.updateRepository(seedData.repositoryId, {
    provider: "gitlab",
    provider_host: GITLAB_HOST,
    provider_owner: "platform",
    provider_name: "kandev",
  });
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await apiClient.linkTaskGitLabMR(seedData.workspaceId, {
    task_id: task.id,
    repository_id: seedData.repositoryId,
    mr_url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${MR_IID}`,
  });
  return task.id;
}

test.describe("GitLab MR topbar hover popover", () => {
  test("AC18/AC20-22/AC23-25: hover reveals the summary, and click still opens the dropdown", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const taskId = await seedTaskWithPopoverFixture(apiClient, seedData, "MR popover desktop");

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const trigger = testPage.getByTestId("mr-topbar-button");
    await expect(trigger).toBeVisible({ timeout: 15_000 });

    const popover = testPage.getByTestId("mr-topbar-popover");
    await expect(popover).not.toBeVisible();

    // AC23: hovering opens the popover without a click.
    await trigger.hover();
    await expect(popover).toBeVisible({ timeout: 5_000 });

    // AC18: pass-rate bar reflects the live per-job breakdown (6/10, 60%).
    const progress = popover.getByTestId("mr-pipeline-progress");
    await expect(progress).toBeVisible();
    await expect(progress).toContainText("6/10");
    await expect(progress).toContainText("60%");

    // AC20/AC21: awaiting review with the unapproved-reviewer suffix.
    const approvalRow = popover.getByTestId("mr-approval-row");
    await expect(approvalRow).toContainText("Awaiting review");
    await expect(approvalRow).toContainText("1 / 2");
    await expect(approvalRow).toContainText("1 awaiting");

    // AC22: singular unresolved-comment pluralization.
    const discussionsRow = popover.getByTestId("mr-discussions-row");
    await expect(discussionsRow).toContainText("1 unresolved comment");

    // AC24 (the anti-flicker "bridge" from trigger to portalled content) is
    // exhaustively covered at the hook level in use-hover-popover.test.ts —
    // the wiring here reuses that hook unchanged (C7), so this spec covers
    // the browser-integration parts a unit test can't: real hover opening
    // the popover (above) and the click-still-opens-the-dropdown handoff
    // (below).

    // AC25: clicking the trigger still opens the existing dropdown, and the
    // hover popover closes while the dropdown is open.
    await trigger.click();
    await expect(testPage.getByTestId("mr-automation-controls")).toBeVisible();
    await expect(popover).not.toBeVisible();
  });

  test("AC19: renders the empty state when the latest pipeline has zero jobs", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await apiClient.configureGitLab(seedData.workspaceId, GITLAB_HOST);
    const now = new Date().toISOString();
    const iid = MR_IID + 1;
    await apiClient.mockGitLabAddMRs(seedData.workspaceId, GITLAB_PROJECT, [
      {
        iid,
        id: iid + 10_000,
        project_id: 101,
        title: "No pipeline MR",
        url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${iid}`,
        web_url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${iid}`,
        state: "open",
        head_branch: "feature/no-pipeline",
        head_sha: "sha-no-pipeline",
        base_branch: "main",
        author_username: "contributor",
        project_namespace: "platform",
        project_path: GITLAB_PROJECT,
        body: "",
        draft: false,
        merge_status: "can_be_merged",
        has_conflicts: false,
        additions: 1,
        deletions: 1,
        reviewers: [],
        assignees: [],
        created_at: now,
        updated_at: now,
      },
    ]);
    await apiClient.updateRepository(seedData.repositoryId, {
      provider: "gitlab",
      provider_host: GITLAB_HOST,
      provider_owner: "platform",
      provider_name: "kandev",
    });
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "MR popover no pipeline",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await apiClient.linkTaskGitLabMR(seedData.workspaceId, {
      task_id: task.id,
      repository_id: seedData.repositoryId,
      mr_url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${iid}`,
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const trigger = testPage.getByTestId("mr-topbar-button");
    await expect(trigger).toBeVisible({ timeout: 15_000 });
    await trigger.hover();

    const popover = testPage.getByTestId("mr-topbar-popover");
    await expect(popover).toBeVisible({ timeout: 5_000 });
    await expect(popover.getByTestId("mr-pipeline-empty")).toBeVisible();
    await expect(popover.getByTestId("mr-pipeline-progress")).toHaveCount(0);
  });
});
