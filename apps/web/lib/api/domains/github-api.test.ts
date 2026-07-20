import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import {
  clearGitHubToken,
  configureGitHubToken,
  copyGitHubWorkspaceSettings,
  createTaskPR,
  disconnectGitHubAppInstallation,
  disconnectGitHubPersonal,
  disconnectGitHubWorkspace,
  fetchAccessibleRepos,
  fetchGitHubCLIAccounts,
  fetchGitHubStatus,
  fetchIssueInfo,
  fetchPRInfo,
  fetchRepoBranches,
  getPRFeedback,
  getPRStatus,
  getPRStatusesBatch,
  getRepoMergeMethods,
  getTaskCIAutomationOptions,
  GitHubUnavailableError,
  linkTaskIssue,
  listUserOrgs,
  listWorkspaceTaskIssues,
  mergePR,
  searchOrgRepos,
  setGitHubWorkspaceConnection,
  startGitHubAppInstall,
  startGitHubPersonalConnect,
  submitPRReview,
  unlinkTaskIssue,
  updateTaskCIAutomationOptions,
  type AccessibleRepo,
} from "./github-api";
import * as githubAuthApi from "./github-auth-api";

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

describe("workspace GitHub authentication", () => {
  it("scopes status reads to the requested workspace", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({ workspace_id: "workspace/one", automation: null, personal: null }),
    );

    await fetchGitHubStatus("workspace/one", { cache: "no-store" });

    const call = fetchSpy.mock.calls.at(-1);
    expect(String(call?.[0])).toBe(
      "http://api.test/api/v1/github/status?workspace_id=workspace%2Fone",
    );
    expect(call?.[1]).toMatchObject({ cache: "no-store" });
  });

  it("selects an exact named CLI account", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ workspace_id: "workspace one" }));

    await setGitHubWorkspaceConnection("workspace one", {
      source: "gh_cli",
      host: "github.example.com",
      login: "alice",
    });

    const call = fetchSpy.mock.calls.at(-1);
    expect(String(call?.[0])).toBe(
      "http://api.test/api/v1/github/workspace-connection?workspace_id=workspace+one",
    );
    expect(call?.[1]?.method).toBe("PUT");
    expect(JSON.parse(String(call?.[1]?.body))).toEqual({
      source: "gh_cli",
      host: "github.example.com",
      login: "alice",
    });
  });

  it("lists all CLI accounts without collapsing hosts", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        accounts: [
          { host: "github.com", login: "alice", active: true, state: "active" },
          { host: "github.example.com", login: "alice", active: false, state: "inactive" },
        ],
      }),
    );

    await expect(fetchGitHubCLIAccounts("ws/1")).resolves.toHaveLength(2);
    expect(lastCallUrl()).toBe(
      "http://api.test/api/v1/github/auth/gh-cli/accounts?workspace_id=ws%2F1",
    );
  });

  it("uses workspace-scoped App and personal lifecycle routes", async () => {
    fetchSpy
      .mockResolvedValueOnce(
        jsonResponse({ URL: "https://github.com/apps/kandev/installations/new" }),
      )
      .mockResolvedValueOnce(jsonResponse({ url: "https://github.com/login/oauth/authorize" }))
      .mockResolvedValueOnce(jsonResponse({ disconnected: true }))
      .mockResolvedValueOnce(jsonResponse({ disconnected: true }))
      .mockResolvedValueOnce(jsonResponse({ disconnected: true }));

    await startGitHubAppInstall("ws/1");
    await startGitHubPersonalConnect("ws/1");
    await disconnectGitHubAppInstallation("ws/1");
    await disconnectGitHubPersonal("ws/1");
    await disconnectGitHubWorkspace("ws/1");

    expect(fetchSpy.mock.calls.map((call) => [String(call[0]), call[1]?.method])).toEqual([
      ["http://api.test/api/v1/github/app/install/start", "POST"],
      ["http://api.test/api/v1/github/personal-connection/start", "POST"],
      ["http://api.test/api/v1/github/app/installation?workspace_id=ws%2F1", "DELETE"],
      ["http://api.test/api/v1/github/personal-connection?workspace_id=ws%2F1", "DELETE"],
      ["http://api.test/api/v1/github/workspace-connection?workspace_id=ws%2F1", "DELETE"],
    ]);
    expect(JSON.parse(String(fetchSpy.mock.calls[0]?.[1]?.body))).toEqual({
      workspace_id: "ws/1",
    });
    expect(JSON.parse(String(fetchSpy.mock.calls[1]?.[1]?.body))).toEqual({
      workspace_id: "ws/1",
    });
  });

  it("keeps compatibility token mutations workspace-scoped", async () => {
    fetchSpy
      .mockResolvedValueOnce(jsonResponse({ configured: true }))
      .mockResolvedValueOnce(jsonResponse({ cleared: true }));

    await configureGitHubToken("ws/1", "github_pat_test");
    await clearGitHubToken("ws/1");

    expect(fetchSpy.mock.calls.map((call) => [String(call[0]), call[1]?.method])).toEqual([
      ["http://api.test/api/v1/github/token?workspace_id=ws%2F1", "POST"],
      ["http://api.test/api/v1/github/token?workspace_id=ws%2F1", "DELETE"],
    ]);
  });
});

describe("deployment GitHub App registration", () => {
  it("exposes the deployment registration API", () => {
    expect(githubAuthApi).toMatchObject({
      fetchDeploymentAppRegistration: expect.any(Function),
      startDeploymentAppRegistration: expect.any(Function),
      deleteDeploymentAppRegistration: expect.any(Function),
    });
  });

  it("uses the system-level registration routes", async () => {
    fetchSpy
      .mockResolvedValueOnce(
        jsonResponse({ source: "none", state: "unconfigured", ready: false, read_only: false }),
      )
      .mockResolvedValueOnce(
        jsonResponse({
          state: "state",
          expires_at: "2026-07-20T22:00:00Z",
          revision: 1,
          registration_url: "https://github.com/organizations/acme/settings/apps/new",
          manifest: { name: "Kandev acme" },
        }),
      )
      .mockResolvedValueOnce(jsonResponse({ deleted: true }));

    await githubAuthApi.fetchDeploymentAppRegistration();
    await githubAuthApi.startDeploymentAppRegistration({
      owner_type: "organization",
      owner_login: "acme",
      public_base_url: "https://kandev.example",
    });
    await githubAuthApi.deleteDeploymentAppRegistration();

    expect(fetchSpy.mock.calls.map((call) => [String(call[0]), call[1]?.method])).toEqual([
      ["http://api.test/api/v1/github/app/registration", undefined],
      ["http://api.test/api/v1/github/app/registration/start", "POST"],
      ["http://api.test/api/v1/github/app/registration", "DELETE"],
    ]);
    expect(fetchSpy.mock.calls[0]?.[1]?.cache).toBe("no-store");
    expect(JSON.parse(String(fetchSpy.mock.calls[1]?.[1]?.body))).toEqual({
      owner_type: "organization",
      owner_login: "acme",
      public_base_url: "https://kandev.example",
    });
  });
});

describe("workspace-scoped GitHub operations", () => {
  it("scopes PR reads, review, merge, and repository metadata", async () => {
    fetchSpy.mockImplementation(async () => jsonResponse({}));

    await getPRFeedback("ws/1", "acme", "site", 42);
    await getPRStatus("ws/1", "acme", "site", 42);
    await getPRStatusesBatch("ws/1", [{ owner: "acme", repo: "site", number: 42 }]);
    await submitPRReview("ws/1", { owner: "acme", repo: "site", number: 42 }, "APPROVE");
    await mergePR("ws/1", "acme", "site", 42, "squash");
    await getRepoMergeMethods("ws/1", "acme", "site");
    await listUserOrgs("ws/1");
    await searchOrgRepos("ws/1", "acme", "site");

    expect(fetchSpy.mock.calls.map((call) => String(call[0]))).toEqual([
      "http://api.test/api/v1/github/prs/acme/site/42?workspace_id=ws%2F1",
      "http://api.test/api/v1/github/prs/acme/site/42/status?workspace_id=ws%2F1",
      "http://api.test/api/v1/github/prs/statuses",
      "http://api.test/api/v1/github/prs/acme/site/42/reviews?workspace_id=ws%2F1",
      "http://api.test/api/v1/github/prs/acme/site/42/merge?workspace_id=ws%2F1",
      "http://api.test/api/v1/github/repos/acme/site/merge-methods?workspace_id=ws%2F1",
      "http://api.test/api/v1/github/orgs?workspace_id=ws%2F1",
      "http://api.test/api/v1/github/repos/search?workspace_id=ws%2F1&org=acme&q=site",
    ]);
    expect(JSON.parse(String(fetchSpy.mock.calls[2]?.[1]?.body))).toEqual({
      workspace_id: "ws/1",
      refs: [{ owner: "acme", repo: "site", number: 42 }],
    });
  });
});

describe("remote repository reads", () => {
  it("retries a transient network failure when loading branches", async () => {
    fetchSpy.mockRejectedValueOnce(new TypeError("fetch failed"));
    fetchSpy.mockResolvedValueOnce(jsonResponse({ branches: [{ name: "main" }] }));

    await expect(fetchRepoBranches("ws/1", "acme", "site")).resolves.toEqual({
      branches: [{ name: "main" }],
    });
    expect(fetchSpy).toHaveBeenCalledTimes(2);
  });

  it("retries a transient network failure when loading PR title metadata", async () => {
    fetchSpy.mockRejectedValueOnce(new TypeError("fetch failed"));
    fetchSpy.mockResolvedValueOnce(jsonResponse({ number: 42, title: "Recovered" }));

    await expect(fetchPRInfo("ws/1", "acme", "site", 42)).resolves.toMatchObject({
      number: 42,
      title: "Recovered",
    });
    expect(fetchSpy).toHaveBeenCalledTimes(2);
  });
});

describe("fetchAccessibleRepos — URL & parsing", () => {
  it("builds the correct URL with both q and limit", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ repos: [] }));

    await fetchAccessibleRepos({ workspaceId: "ws/1", q: "next", limit: 25 });

    const url = lastCallUrl();
    expect(url).toContain("/api/v1/github/repos");
    expect(url).toContain("q=next");
    expect(url).toContain("limit=25");
    expect(url).toContain("workspace_id=ws%2F1");
  });

  it("omits empty query and missing limit from the URL", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ repos: [] }));

    await fetchAccessibleRepos({ workspaceId: "ws/1" });

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

    const repos: AccessibleRepo[] = await fetchAccessibleRepos({ workspaceId: "ws/1" });

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

    await fetchIssueInfo("workspace/1", "acme org", "site/repo", 1456, {
      cache: "no-store",
    });

    const call = fetchSpy.mock.calls.at(-1);
    expect(String(call?.[0])).toBe(
      "http://api.test/api/v1/github/issues/acme%20org/site%2Frepo/1456/info?workspace_id=workspace%2F1",
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
      workspace_id: "workspace-1",
      task_id: "task-1",
      repository_id: "task-repo-1",
      pr_url: "https://github.com/kdlbs/kandev/pull/1471",
    });

    const call = fetchSpy.mock.calls.at(-1);
    expect(String(call?.[0])).toBe("http://api.test/api/v1/github/task-prs");
    expect(call?.[1]?.method).toBe("POST");
    expect(JSON.parse(String(call?.[1]?.body))).toEqual({
      workspace_id: "workspace-1",
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

    await expect(fetchAccessibleRepos({ workspaceId: "ws-1" })).rejects.toBeInstanceOf(
      GitHubUnavailableError,
    );
  });

  it("throws a plain Error (not GitHubUnavailableError) on 503 without the github_not_configured code", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ error: "transient outage" }), {
        status: 503,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const err = await fetchAccessibleRepos({ workspaceId: "ws-1" }).catch((e) => e);
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

    const err = await fetchAccessibleRepos({ workspaceId: "ws-1" }).catch((e) => e);
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

    const promise = fetchAccessibleRepos({ workspaceId: "ws-1", signal: controller.signal });
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
