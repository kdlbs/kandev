import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  configureGitLabHost,
  configureGitLabToken,
  clearGitLabToken,
  createReviewWatch,
  deleteIssueWatch,
  previewResetReviewWatch,
  resetIssueWatch,
  triggerReviewWatch,
  updateIssueWatch,
  createTaskMR,
  deleteTaskMR,
  fetchGitLabStatus,
  listTaskMRs,
  listWorkspaceTaskMRs,
  searchUserIssues,
  searchUserMRs,
  syncTaskMR,
  getMRFeedback,
  getMRFiles,
  getMRCommits,
  createMRDiscussionNote,
  resolveMRDiscussion,
  approveMR,
  unapproveMR,
  mergeMR,
  setMRLabels,
  setMRAssignees,
  listProjectMembers,
  setMRReviewers,
  getMRSubscription,
  setMRSubscription,
  getIssueSubscription,
  setIssueSubscription,
  listMRWatches,
  deleteMRWatch,
  listUserProjects,
  searchProjects,
  listProjectBranches,
  getProjectMergeMethods,
  fetchGitLabStats,
  updateActionPresets,
} from "./gitlab-api";

const originalFetch = global.fetch;
const SELF_MANAGED_HOST = "https://gitlab.acme.corp";
const WORKSPACE_WITH_SPACE = "workspace one";
const PROJECT_PATH = "group/project";

function mockResponse(data: unknown, status = 200) {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("gitlab-api — auth", () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchSpy = vi.fn();
    global.fetch = fetchSpy as unknown as typeof fetch;
  });

  afterEach(() => {
    global.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it("fetchGitLabStatus scopes status to the selected workspace", async () => {
    fetchSpy.mockResolvedValueOnce(
      mockResponse({
        authenticated: true,
        username: "alice",
        auth_method: "pat",
        host: "https://gitlab.com",
        token_configured: true,
        required_scopes: ["api"],
      }),
    );
    const status = await fetchGitLabStatus({ workspaceId: WORKSPACE_WITH_SPACE });
    expect(fetchSpy).toHaveBeenCalledTimes(1);
    const url = fetchSpy.mock.calls[0]![0] as string;
    expect(url).toContain("/api/v1/gitlab/status");
    expect(url).toContain(`workspace_id=${encodeURIComponent(WORKSPACE_WITH_SPACE)}`);
    expect(status.username).toBe("alice");
    expect(status.auth_method).toBe("pat");
  });

  it("configureGitLabToken PUTs a workspace config with the token", async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse({ configured: true }));
    const result = await configureGitLabToken("glpat-123", { workspaceId: "workspace-1" });
    expect(result.configured).toBe(true);
    const init = fetchSpy.mock.calls[0]![1] as RequestInit;
    expect(init.method).toBe("PUT");
    expect(JSON.parse(init.body as string)).toEqual({
      host: "",
      auth_method: "pat",
      token: "glpat-123",
    });
  });

  it("clearGitLabToken issues DELETE", async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse({ cleared: true }));
    await clearGitLabToken({ workspaceId: "workspace-1" });
    const init = fetchSpy.mock.calls[0]![1] as RequestInit;
    expect(init.method).toBe("DELETE");
  });

  it("configureGitLabHost PUTs the workspace host", async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse({ configured: true, host: SELF_MANAGED_HOST }));
    const result = await configureGitLabHost(SELF_MANAGED_HOST, { workspaceId: "workspace-1" });
    expect(result.host).toBe(SELF_MANAGED_HOST);
    const init = fetchSpy.mock.calls[0]![1] as RequestInit;
    expect(init.method).toBe("PUT");
    expect(JSON.parse(init.body as string)).toEqual({
      host: SELF_MANAGED_HOST,
      auth_method: "pat",
    });
  });
});

describe("gitlab-api — task MRs", () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchSpy = vi.fn();
    global.fetch = fetchSpy as unknown as typeof fetch;
  });

  afterEach(() => {
    global.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it("listWorkspaceTaskMRs encodes the workspace id and hits /workspaces/:id/task-mrs", async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse({ task_mrs: {} }));
    await listWorkspaceTaskMRs("ws id/with slash");
    const url = fetchSpy.mock.calls[0]![0] as string;
    expect(url).toContain("/api/v1/gitlab/workspaces/");
    expect(url).toContain(encodeURIComponent("ws id/with slash"));
    expect(url).toContain("/task-mrs");
  });

  it("listTaskMRs encodes the task id and hits /tasks/:id/mrs", async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse({ task_mrs: [] }));
    await listTaskMRs("task/123");
    const url = fetchSpy.mock.calls[0]![0] as string;
    expect(url).toContain(`/api/v1/gitlab/tasks/${encodeURIComponent("task/123")}/mrs`);
  });

  it("syncTaskMR POSTs the project/iid body to /tasks/:id/mrs/sync", async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse({ id: "1", task_id: "t-1", mr_iid: 99 }));
    await syncTaskMR("t-1", {
      project_path: "acme/api",
      iid: 99,
      repository_id: "repo-a",
    });
    const url = fetchSpy.mock.calls[0]![0] as string;
    expect(url).toContain("/api/v1/gitlab/tasks/t-1/mrs/sync");
    const init = fetchSpy.mock.calls[0]![1] as RequestInit;
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body as string)).toEqual({
      project_path: "acme/api",
      iid: 99,
      repository_id: "repo-a",
    });
  });

  it("createTaskMR scopes the explicit link to the workspace", async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse({ id: "association-1", task_id: "task-1" }));
    await createTaskMR(
      {
        task_id: "task-1",
        repository_id: "repo-1",
        mr_url: `${SELF_MANAGED_HOST}/group/project/-/merge_requests/7`,
      },
      "ws-1",
    );
    const url = fetchSpy.mock.calls[0]![0] as string;
    expect(url).toContain("/api/v1/gitlab/task-mrs?workspace_id=ws-1");
    const init = fetchSpy.mock.calls[0]![1] as RequestInit;
    expect(init.method).toBe("POST");
  });

  it("deleteTaskMR scopes unlink to the workspace and association", async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse({ deleted: true }));
    await deleteTaskMR("association/1", "ws-1");
    const url = fetchSpy.mock.calls[0]![0] as string;
    expect(url).toContain(`/api/v1/gitlab/task-mrs/${encodeURIComponent("association/1")}`);
    expect(url).toContain("workspace_id=ws-1");
    const init = fetchSpy.mock.calls[0]![1] as RequestInit;
    expect(init.method).toBe("DELETE");
  });
});

describe("gitlab-api — user search", () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchSpy = vi.fn();
    global.fetch = fetchSpy as unknown as typeof fetch;
  });

  afterEach(() => {
    global.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it("searchUserMRs builds the query string and disables cache", async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse({ mrs: [], total_count: 0 }));
    await searchUserMRs({
      workspaceId: "ws-1",
      filter: "assigned_to_me",
      customQuery: "labels=bug",
      page: 2,
      perPage: 25,
    });
    const url = fetchSpy.mock.calls[0]![0] as string;
    expect(url).toContain("/api/v1/gitlab/user/mrs");
    expect(url).toContain("workspace_id=ws-1");
    expect(url).toContain("filter=assigned_to_me");
    // custom_query gets URL-encoded by URLSearchParams.
    expect(url).toContain(`custom_query=${encodeURIComponent("labels=bug")}`);
    expect(url).toContain("page=2");
    expect(url).toContain("per_page=25");
    const init = fetchSpy.mock.calls[0]![1] as RequestInit;
    // `cache` must be at the top level of fetch init; reading from
    // options.init.cache (as an earlier revision did) silently no-ops.
    expect(init.cache).toBe("no-store");
  });

  it("searchUserIssues builds the query string and disables cache", async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse({ issues: [], total_count: 0 }));
    await searchUserIssues({ workspaceId: "ws-2", filter: "created_by_me", perPage: 10 });
    const url = fetchSpy.mock.calls[0]![0] as string;
    expect(url).toContain("/api/v1/gitlab/user/issues");
    expect(url).toContain("workspace_id=ws-2");
    expect(url).toContain("filter=created_by_me");
    expect(url).toContain("per_page=10");
    const init = fetchSpy.mock.calls[0]![1] as RequestInit;
    expect(init.cache).toBe("no-store");
  });
});

describe("gitlab-api — workspace watch actions", () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchSpy = vi.fn().mockImplementation(() => Promise.resolve(mockResponse({})));
    global.fetch = fetchSpy as unknown as typeof fetch;
  });

  afterEach(() => {
    global.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it("scopes create and every review action to the workspace", async () => {
    await createReviewWatch({
      workspace_id: WORKSPACE_WITH_SPACE,
      workflow_id: "workflow",
      workflow_step_id: "step",
      projects: [],
      agent_profile_id: "",
      executor_profile_id: "",
      prompt: "Review {{mr_url}}",
      repository_id: "repo",
      base_branch: "main",
      review_scope: "user",
      custom_query: "state=opened",
      poll_interval_seconds: 300,
      cleanup_policy: "auto",
      max_inflight_tasks: 3,
    });
    await triggerReviewWatch("watch/1", WORKSPACE_WITH_SPACE);
    await previewResetReviewWatch("watch/1", WORKSPACE_WITH_SPACE);

    for (const call of fetchSpy.mock.calls) {
      expect(call[0]).toContain(`workspace_id=${encodeURIComponent(WORKSPACE_WITH_SPACE)}`);
    }
  });

  it("scopes issue update, delete, and reset to the workspace", async () => {
    await updateIssueWatch("watch/2", "ws-2", { enabled: false });
    await deleteIssueWatch("watch/2", "ws-2");
    await resetIssueWatch("watch/2", "ws-2");

    for (const call of fetchSpy.mock.calls) {
      expect(call[0]).toContain("workspace_id=ws-2");
    }
  });

  it("scopes legacy MR watches, projects, branches, merge methods, stats, and presets", async () => {
    await listMRWatches(WORKSPACE_WITH_SPACE, { taskId: "task-1" });
    await deleteMRWatch("watch/1", WORKSPACE_WITH_SPACE);
    await listUserProjects(WORKSPACE_WITH_SPACE);
    await searchProjects(WORKSPACE_WITH_SPACE, "group project");
    await listProjectBranches(WORKSPACE_WITH_SPACE, PROJECT_PATH);
    await getProjectMergeMethods(WORKSPACE_WITH_SPACE, PROJECT_PATH);
    await fetchGitLabStats(WORKSPACE_WITH_SPACE);
    await updateActionPresets(WORKSPACE_WITH_SPACE, { mr: [] });

    for (const call of fetchSpy.mock.calls) {
      expect(new URL(call[0] as string).searchParams.get("workspace_id")).toBe(
        WORKSPACE_WITH_SPACE,
      );
    }
  });
});

describe("gitlab-api — merge request review", () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchSpy = vi.fn().mockImplementation(() => Promise.resolve(mockResponse({})));
    global.fetch = fetchSpy as unknown as typeof fetch;
  });

  afterEach(() => {
    global.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it("scopes every review read and write to the active workspace", async () => {
    const identity = {
      workspaceId: WORKSPACE_WITH_SPACE,
      project: "group/sub/project",
      iid: 17,
      host: SELF_MANAGED_HOST,
    };
    await getMRFeedback(identity);
    await getMRFiles(identity);
    await getMRCommits(identity);
    await createMRDiscussionNote({ ...identity, discussionId: "thread/1", body: "Fixed" });
    await resolveMRDiscussion({ ...identity, discussionId: "thread/1" });
    await approveMR(identity);
    await unapproveMR(identity);
    await mergeMR({ ...identity, squash: true, squashCommitMessage: "Squashed" });
    await setMRLabels({ ...identity, labels: ["bug", "backend"] });
    await setMRAssignees({ ...identity, assigneeIds: [12, 44] });
    await listProjectMembers(identity.workspaceId, identity.project, "ali ce", identity.host);
    await setMRReviewers({ ...identity, reviewerIds: [31, 45] });
    await getMRSubscription(identity);
    await setMRSubscription({ ...identity, subscribed: true });
    await getIssueSubscription(identity);
    await setIssueSubscription({ ...identity, subscribed: true });

    for (const call of fetchSpy.mock.calls) {
      expect(new URL(call[0] as string).searchParams.get("workspace_id")).toBe(
        WORKSPACE_WITH_SPACE,
      );
      expect(new URL(call[0] as string).searchParams.get("expected_host")).toBe(SELF_MANAGED_HOST);
    }
  });

  it("uses GitLab numeric IDs and discussion identity in mutation bodies", async () => {
    const identity = { workspaceId: "ws-1", project: PROJECT_PATH, iid: 8 };
    await createMRDiscussionNote({ ...identity, discussionId: "discussion-9", body: "Done" });
    await setMRReviewers({ ...identity, reviewerIds: [101, 202] });
    await setMRAssignees({ ...identity, assigneeIds: [303] });

    expect(JSON.parse((fetchSpy.mock.calls[0]![1] as RequestInit).body as string)).toEqual({
      project: PROJECT_PATH,
      iid: 8,
      discussion_id: "discussion-9",
      body: "Done",
    });
    expect(JSON.parse((fetchSpy.mock.calls[1]![1] as RequestInit).body as string)).toMatchObject({
      reviewer_ids: [101, 202],
    });
    expect(JSON.parse((fetchSpy.mock.calls[2]![1] as RequestInit).body as string)).toMatchObject({
      assignee_ids: [303],
    });
  });
});
