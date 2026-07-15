import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import {
  copyGitHubWorkspaceSettings,
  createTaskPR,
  fetchAccessibleRepos,
  fetchIssueInfo,
  fetchPRInfo,
  fetchRepoBranches,
  getTaskCIAutomationOptions,
  GitHubUnavailableError,
  linkTaskIssue,
  listWorkspaceTaskIssues,
  unlinkTaskIssue,
  updateTaskCIAutomationOptions,
  type AccessibleRepo,
} from "./github-api";

type FetchInput = Parameters<typeof fetch>[0];
type FetchInit = Parameters<typeof fetch>[1];

const fetchSpy = vi.fn<(...args: [FetchInput, FetchInit?]) => Promise<Response>>();

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function jsonResponse(body: unknown, init?: ResponseInit): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
    ...init,
  });
}

function lastCallUrl(): string {
  const call = fetchSpy.mock.calls.at(-1);
  if (!call) throw new Error("expected fetch to have been called");
  return String(call[0]);
}

describe("remote repository reads", () => {
  it("retries a transient network failure when loading branches", async () => {
    fetchSpy.mockRejectedValueOnce(new TypeError("fetch failed"));
    fetchSpy.mockResolvedValueOnce(jsonResponse({ branches: [{ name: "main" }] }));

    await expect(fetchRepoBranches("acme", "site")).resolves.toEqual({
      branches: [{ name: "main" }],
    });
    expect(fetchSpy).toHaveBeenCalledTimes(2);
  });

  it("retries a transient network failure when loading PR title metadata", async () => {
    fetchSpy.mockRejectedValueOnce(new TypeError("fetch failed"));
    fetchSpy.mockResolvedValueOnce(jsonResponse({ number: 42, title: "Recovered" }));

    await expect(fetchPRInfo("acme", "site", 42)).resolves.toMatchObject({
      number: 42,
      title: "Recovered",
    });
    expect(fetchSpy).toHaveBeenCalledTimes(2);
  });
});

describe("fetchAccessibleRepos — URL & parsing", () => {
  it("builds the correct URL with both q and limit", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ repos: [] }));

    await fetchAccessibleRepos({ q: "next", limit: 25 });

    const url = lastCallUrl();
    expect(url).toContain("/api/v1/github/repos");
    expect(url).toContain("q=next");
    expect(url).toContain("limit=25");
  });

  it("omits empty query and missing limit from the URL", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ repos: [] }));

    await fetchAccessibleRepos({});

    const url = lastCallUrl();
    expect(url).toContain("/api/v1/github/repos");
    expect(url).not.toContain("q=");
    expect(url).not.toContain("limit=");
  });

  it("parses the 200 response and injects provider: 'github' on each entry", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        repos: [
          {
            full_name: "kdlbs/kandev",
            owner: "kdlbs",
            name: "kandev",
            private: false,
            default_branch: "main",
            description: "Kandev mainline",
            pushed_at: "2026-05-20T10:00:00Z",
          },
          {
            full_name: "acme/site",
            owner: "acme",
            name: "site",
            private: true,
            default_branch: "trunk",
          },
        ],
      }),
    );

    const repos: AccessibleRepo[] = await fetchAccessibleRepos({});

    expect(repos).toHaveLength(2);
    expect(repos[0]).toMatchObject({
      provider: "github",
      full_name: "kdlbs/kandev",
      owner: "kdlbs",
      name: "kandev",
      private: false,
      default_branch: "main",
      description: "Kandev mainline",
      pushed_at: "2026-05-20T10:00:00Z",
    });
    expect(repos[1]).toMatchObject({
      provider: "github",
      full_name: "acme/site",
      owner: "acme",
      name: "site",
      private: true,
      default_branch: "trunk",
    });
    expect(repos[1].pushed_at).toBeUndefined();
    expect(repos[1].description).toBeUndefined();
  });
});

describe("fetchIssueInfo", () => {
  it("builds the encoded issue info endpoint and forwards options", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ number: 1456, title: "Fix picker" }));

    await fetchIssueInfo("acme org", "site/repo", 1456, { cache: "no-store" });

    const call = fetchSpy.mock.calls.at(-1);
    expect(String(call?.[0])).toBe(
      "http://api.test/api/v1/github/issues/acme%20org/site%2Frepo/1456/info",
    );
    expect(call?.[1]).toMatchObject({ cache: "no-store" });
  });
});

describe("task issue link helpers", () => {
  it("lists task issue links for a workspace", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ task_issues: {} }));

    await listWorkspaceTaskIssues("workspace/1", { cache: "no-store" });

    const call = fetchSpy.mock.calls.at(-1);
    expect(String(call?.[0])).toBe(
      "http://api.test/api/v1/github/task-issues?workspace_id=workspace%2F1",
    );
    expect(call?.[1]).toMatchObject({ cache: "no-store" });
  });

  it("links a GitHub pull request to a task", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        id: "task-pr-1",
        task_id: "task-1",
        repository_id: "task-repo-1",
        owner: "kdlbs",
        repo: "kandev",
        pr_number: 1471,
        pr_url: "https://github.com/kdlbs/kandev/pull/1471",
        pr_title: "Link references",
      }),
    );

    await createTaskPR({
      task_id: "task-1",
      repository_id: "task-repo-1",
      pr_url: "https://github.com/kdlbs/kandev/pull/1471",
    });

    const call = fetchSpy.mock.calls.at(-1);
    expect(String(call?.[0])).toBe("http://api.test/api/v1/github/task-prs");
    expect(call?.[1]?.method).toBe("POST");
    expect(JSON.parse(String(call?.[1]?.body))).toEqual({
      task_id: "task-1",
      repository_id: "task-repo-1",
      pr_url: "https://github.com/kdlbs/kandev/pull/1471",
    });
  });

  it("links a GitHub issue to a task", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        task_id: "task-1",
        owner: "kdlbs",
        repo: "kandev",
        issue_number: 1470,
        issue_url: "https://github.com/kdlbs/kandev/issues/1470",
        issue_title: "Link issue",
      }),
    );

    await linkTaskIssue("task-1", {
      issue: "#1470",
      owner: "kdlbs",
      repo: "kandev",
      number: 1470,
    });

    const call = fetchSpy.mock.calls.at(-1);
    expect(String(call?.[0])).toBe("http://api.test/api/v1/github/tasks/task-1/issue");
    expect(call?.[1]?.method).toBe("PUT");
    expect(JSON.parse(String(call?.[1]?.body))).toEqual({
      issue: "#1470",
      owner: "kdlbs",
      repo: "kandev",
      number: 1470,
    });
  });

  it("unlinks a GitHub issue from a task", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ unlinked: true }));

    await unlinkTaskIssue("task-1");

    const call = fetchSpy.mock.calls.at(-1);
    expect(String(call?.[0])).toBe("http://api.test/api/v1/github/tasks/task-1/issue");
    expect(call?.[1]?.method).toBe("DELETE");
  });
});

describe("fetchAccessibleRepos — errors & signal", () => {
  it("throws GitHubUnavailableError on 503 with code: github_not_configured", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          error: "GitHub is not configured.",
          code: "github_not_configured",
        }),
        { status: 503, headers: { "Content-Type": "application/json" } },
      ),
    );

    await expect(fetchAccessibleRepos({})).rejects.toBeInstanceOf(GitHubUnavailableError);
  });

  it("throws a plain Error (not GitHubUnavailableError) on 503 without the github_not_configured code", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ error: "transient outage" }), {
        status: 503,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const err = await fetchAccessibleRepos({}).catch((e) => e);
    expect(err).toBeInstanceOf(Error);
    expect(err).not.toBeInstanceOf(GitHubUnavailableError);
  });

  it("throws a plain Error on 500", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ error: "boom" }), {
        status: 500,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const err = await fetchAccessibleRepos({}).catch((e) => e);
    expect(err).toBeInstanceOf(Error);
    expect(err).not.toBeInstanceOf(GitHubUnavailableError);
  });

  it("forwards AbortSignal: aborting causes the promise to reject", async () => {
    const controller = new AbortController();
    fetchSpy.mockImplementationOnce((_input, init) => {
      return new Promise((_resolve, reject) => {
        const signal = init?.signal;
        if (signal?.aborted) {
          reject(new DOMException("Aborted", "AbortError"));
          return;
        }
        signal?.addEventListener("abort", () => {
          reject(new DOMException("Aborted", "AbortError"));
        });
      });
    });

    const promise = fetchAccessibleRepos({ signal: controller.signal });
    controller.abort();
    await expect(promise).rejects.toThrow();
  });
});

describe("task CI automation options", () => {
  it("fetches options for a task", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        task_id: "task-1",
        auto_fix_enabled: true,
        auto_merge_enabled: false,
        auto_fix_prompt_override: null,
        effective_auto_fix_prompt: "Fix CI.",
        using_default_prompt: true,
        updated_at: "2026-06-18T10:00:00Z",
        pr_states: [],
      }),
    );

    const response = await getTaskCIAutomationOptions("task-1");

    expect(lastCallUrl()).toBe("http://api.test/api/v1/github/tasks/task-1/ci-options");
    expect(response.auto_fix_enabled).toBe(true);
    expect(response.using_default_prompt).toBe(true);
  });

  it("patches task options and allows clearing the prompt override", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        task_id: "task-1",
        auto_fix_enabled: false,
        auto_merge_enabled: true,
        auto_fix_prompt_override: null,
        effective_auto_fix_prompt: "Default prompt",
        using_default_prompt: true,
        updated_at: "2026-06-18T10:01:00Z",
        pr_states: [],
      }),
    );

    await updateTaskCIAutomationOptions("task-1", {
      auto_fix_enabled: false,
      auto_merge_enabled: true,
      auto_fix_prompt_override: null,
    });

    const call = fetchSpy.mock.calls.at(-1);
    expect(String(call?.[0])).toBe("http://api.test/api/v1/github/tasks/task-1/ci-options");
    expect(call?.[1]?.method).toBe("PATCH");
    expect(JSON.parse(String(call?.[1]?.body))).toEqual({
      auto_fix_enabled: false,
      auto_merge_enabled: true,
      auto_fix_prompt_override: null,
    });
  });
});

describe("copyGitHubWorkspaceSettings", () => {
  it("POSTs targetWorkspaceId to /workspace-settings/copy scoped to the source", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ workspace_id: "ws-dst" }));

    await copyGitHubWorkspaceSettings("ws-dst", { workspaceId: "ws-src" });

    const call = fetchSpy.mock.calls.at(-1);
    expect(String(call?.[0])).toBe(
      "http://api.test/api/v1/github/workspace-settings/copy?workspace_id=ws-src",
    );
    expect(call?.[1]?.method).toBe("POST");
    expect(JSON.parse(String(call?.[1]?.body))).toEqual({ targetWorkspaceId: "ws-dst" });
  });
});
