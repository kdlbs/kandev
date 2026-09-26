import type { ApiClient, MockGitLabMRSeed } from "./api-client";

export const GITLAB_HOST = "https://gitlab.example.test";
export const GITLAB_PROJECT = "platform/kandev";

type GitLabTaskSeedData = {
  workspaceId: string;
  repositoryId: string;
  agentProfileId: string;
  workflowId: string;
  startStepId: string;
};

export function gitLabMR(
  iid: number,
  title: string,
  overrides: Partial<MockGitLabMRSeed> = {},
): MockGitLabMRSeed {
  const now = new Date().toISOString();
  return {
    id: iid + 10_000,
    iid,
    project_id: 101,
    title,
    url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${iid}`,
    web_url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${iid}`,
    state: "opened",
    head_branch: "main",
    head_sha: `sha-${iid}`,
    base_branch: "main",
    author_username: "contributor",
    project_namespace: "platform",
    project_path: GITLAB_PROJECT,
    body: `Description for ${title}`,
    draft: false,
    merge_status: "can_be_merged",
    has_conflicts: false,
    additions: 12,
    deletions: 3,
    reviewers: [{ id: 1, username: "kandev-tester", name: "Kandev Tester", type: "user" }],
    assignees: [],
    labels: ["backend"],
    created_at: now,
    updated_at: now,
    ...overrides,
  };
}

export async function seedGitLabReview(
  apiClient: ApiClient,
  workspaceId: string,
  iid: number,
  title: string,
  host = GITLAB_HOST,
): Promise<void> {
  await apiClient.configureGitLab(workspaceId, host);
  await seedGitLabMRData(apiClient, workspaceId, iid, title, host);
}

// The MR-data half of seedGitLabReview, without the `configureGitLab` call.
// Configuring the connection invalidates and rebuilds the workspace's cached
// GitLab client (SetConfigForWorkspace -> invalidateWorkspaceClient), which
// discards the in-memory MockClient's previously seeded state — so seeding
// two-or-more MRs on one workspace must configure once, then call this for
// each MR, rather than calling seedGitLabReview (and its configureGitLab)
// per MR.
export async function seedGitLabMRData(
  apiClient: ApiClient,
  workspaceId: string,
  iid: number,
  title: string,
  host = GITLAB_HOST,
): Promise<void> {
  const mr = gitLabMR(iid, title, {
    url: `${host}/${GITLAB_PROJECT}/-/merge_requests/${iid}`,
    web_url: `${host}/${GITLAB_PROJECT}/-/merge_requests/${iid}`,
  });
  await apiClient.mockGitLabAddMRs(workspaceId, GITLAB_PROJECT, [mr]);
  await apiClient.mockGitLabAddMembers(workspaceId, GITLAB_PROJECT, [
    { id: 1, username: "kandev-tester", name: "Kandev Tester", avatar_url: "" },
    { id: 42, username: "alice", name: "Alice Reviewer", avatar_url: "" },
  ]);
  await apiClient.mockGitLabAddApprovals(workspaceId, GITLAB_PROJECT, iid, [], 1);
  await apiClient.mockGitLabAddPipelines(workspaceId, GITLAB_PROJECT, [
    {
      id: 501,
      iid: 1,
      status: "success",
      source: "merge_request_event",
      ref: "main",
      sha: `sha-${iid}`,
      web_url: `${host}/${GITLAB_PROJECT}/-/pipelines/501`,
      jobs_total: 4,
      jobs_passing: 4,
    },
  ]);
  const now = new Date().toISOString();
  await apiClient.mockGitLabAddDiscussions(workspaceId, GITLAB_PROJECT, iid, [
    {
      id: "thread-1",
      resolvable: true,
      resolved: false,
      path: "src/main.ts",
      line: 12,
      notes: [
        {
          id: 701,
          author: "alice",
          body: "Please cover this branch with a regression test.",
          created_at: now,
          updated_at: now,
        },
      ],
      created_at: now,
      updated_at: now,
    },
  ]);
  await apiClient.mockGitLabAddFiles(workspaceId, GITLAB_PROJECT, iid, [
    {
      filename: "src/main.ts",
      status: "modified",
      additions: 8,
      deletions: 2,
      patch: "@@ -1 +1 @@\n-old\n+new",
    },
  ]);
  await apiClient.mockGitLabAddCommits(workspaceId, GITLAB_PROJECT, iid, [
    {
      sha: "1234567890abcdef",
      message: "feat: add GitLab parity",
      author_name: "Alice Reviewer",
      author_date: now,
    },
  ]);
}

export async function seedTaskWithLinkedGitLabMRs(
  apiClient: ApiClient,
  seedData: GitLabTaskSeedData,
  title: string,
  iids: number[],
  mrTitlePrefix: string,
): Promise<string> {
  // Configuring the connection invalidates and rebuilds the workspace's cached
  // mock client, so it must happen before seeding every MR for this task.
  await apiClient.configureGitLab(seedData.workspaceId, GITLAB_HOST);
  for (const iid of iids) {
    await seedGitLabMRData(apiClient, seedData.workspaceId, iid, `${mrTitlePrefix} ${iid}`);
  }
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
  for (const iid of iids) {
    await apiClient.linkTaskGitLabMR(seedData.workspaceId, {
      task_id: task.id,
      repository_id: seedData.repositoryId,
      mr_url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${iid}`,
    });
  }
  return task.id;
}
