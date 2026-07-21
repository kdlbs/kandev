import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  configureGitLabHost,
  configureGitLabToken,
  clearGitLabToken,
  fetchGitLabStatus,
  listTaskMRs,
  listWorkspaceTaskMRs,
  searchUserIssues,
  searchUserMRs,
  syncTaskMR,
} from "./gitlab-api";

const originalFetch = global.fetch;
const SELF_MANAGED_HOST = "https://gitlab.acme.corp";

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

  it("fetchGitLabStatus calls /api/v1/gitlab/status", async () => {
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
    const status = await fetchGitLabStatus();
    expect(fetchSpy).toHaveBeenCalledTimes(1);
    const url = fetchSpy.mock.calls[0]![0] as string;
    expect(url).toContain("/api/v1/gitlab/status");
    expect(status.username).toBe("alice");
    expect(status.auth_method).toBe("pat");
  });

  it("configureGitLabToken POSTs the token", async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse({ configured: true }));
    const result = await configureGitLabToken("glpat-123");
    expect(result.configured).toBe(true);
    const init = fetchSpy.mock.calls[0]![1] as RequestInit;
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body as string)).toEqual({ token: "glpat-123" });
  });

  it("clearGitLabToken issues DELETE", async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse({ cleared: true }));
    await clearGitLabToken();
    const init = fetchSpy.mock.calls[0]![1] as RequestInit;
    expect(init.method).toBe("DELETE");
  });

  it("configureGitLabHost POSTs the host", async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse({ configured: true, host: SELF_MANAGED_HOST }));
    const result = await configureGitLabHost(SELF_MANAGED_HOST);
    expect(result.host).toBe(SELF_MANAGED_HOST);
    const init = fetchSpy.mock.calls[0]![1] as RequestInit;
    expect(JSON.parse(init.body as string)).toEqual({ host: SELF_MANAGED_HOST });
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
      filter: "assigned_to_me",
      customQuery: "labels=bug",
      page: 2,
      perPage: 25,
    });
    const url = fetchSpy.mock.calls[0]![0] as string;
    expect(url).toContain("/api/v1/gitlab/user/mrs");
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
    await searchUserIssues({ filter: "created_by_me", perPage: 10 });
    const url = fetchSpy.mock.calls[0]![0] as string;
    expect(url).toContain("/api/v1/gitlab/user/issues");
    expect(url).toContain("filter=created_by_me");
    expect(url).toContain("per_page=10");
    const init = fetchSpy.mock.calls[0]![1] as RequestInit;
    expect(init.cache).toBe("no-store");
  });
});
