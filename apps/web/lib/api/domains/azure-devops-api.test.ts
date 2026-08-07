import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import {
  associateAzureDevOpsPullRequest,
  associateAzureDevOpsWorkItem,
  copyAzureDevOpsConfig,
  deleteAzureDevOpsConfig,
  getAzureDevOpsConfig,
  getAzureDevOpsWorkspaceSettings,
  getAzureDevOpsPullRequestFeedback,
  listAzureDevOpsProjects,
  listAzureDevOpsBranches,
  listAzureDevOpsTeams,
  listAzureDevOpsBoards,
  getAzureDevOpsBoardSnapshot,
  updateAzureDevOpsBoardWorkItem,
  listAzureDevOpsPullRequests,
  listAzureDevOpsRepositories,
  listWorkspaceAzureDevOpsTaskPullRequests,
  listWorkspaceAzureDevOpsTaskWorkItems,
  searchAzureDevOpsWorkItems,
  setAzureDevOpsConfig,
  updateAzureDevOpsWorkspaceSettings,
  syncAzureDevOpsTaskPullRequest,
  testAzureDevOpsConnection,
} from "./azure-devops-api";

const BASE = "http://api.test/api/v1/azure-devops";
type FetchInput = Parameters<typeof fetch>[0];
type FetchInit = Parameters<typeof fetch>[1];
const fetchSpy = vi.fn<(...args: [FetchInput, FetchInit?]) => Promise<Response>>();

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function lastCall(): { url: string; init: FetchInit | undefined } {
  const call = fetchSpy.mock.calls.at(-1);
  if (!call) throw new Error("expected fetch to have been called");
  return { url: String(call[0]), init: call[1] };
}

describe("Azure DevOps config API", () => {
  it("scopes config reads to the selected workspace and maps 204 to null", async () => {
    fetchSpy.mockResolvedValueOnce(new Response(null, { status: 204 }));

    await expect(getAzureDevOpsConfig("ws one")).resolves.toBeNull();
    expect(lastCall().url).toBe(`${BASE}/config?workspace_id=ws+one`);
  });

  it("posts config and connection tests without changing the PAT field", async () => {
    const payload = {
      organizationUrl: "https://dev.azure.com/acme",
      defaultProjectId: "project-1",
      defaultProjectName: "Platform",
      authMethod: "pat" as const,
      pat: "secret",
    };
    fetchSpy.mockImplementation(async () => jsonResponse({ ...payload, hasSecret: true }));

    await setAzureDevOpsConfig("ws-1", payload);
    expect(lastCall()).toMatchObject({
      url: `${BASE}/config?workspace_id=ws-1`,
      init: { method: "POST", body: JSON.stringify(payload) },
    });

    await testAzureDevOpsConnection("ws-1", payload);
    expect(lastCall().url).toBe(`${BASE}/config/test?workspace_id=ws-1`);
  });

  it("copies and deletes workspace configuration", async () => {
    fetchSpy.mockImplementation(async () => jsonResponse({ deleted: true }));

    await copyAzureDevOpsConfig("source", "target");
    expect(lastCall()).toMatchObject({
      url: `${BASE}/config/copy?workspace_id=source`,
      init: { method: "POST", body: JSON.stringify({ targetWorkspaceId: "target" }) },
    });

    await deleteAzureDevOpsConfig("source");
    expect(lastCall().init?.method).toBe("DELETE");
  });
});

describe("Azure DevOps workspace settings API", () => {
  it("reads and patches workspace-scoped query and action presets", async () => {
    fetchSpy.mockImplementation(async () => jsonResponse({ workspaceId: "ws-1" }));
    await getAzureDevOpsWorkspaceSettings("ws / one");
    expect(lastCall().url).toBe(`${BASE}/workspace-settings?workspace_id=ws+%2F+one`);

    const payload = { workItemActions: [] };
    await updateAzureDevOpsWorkspaceSettings("ws-1", payload);
    expect(lastCall()).toMatchObject({
      url: `${BASE}/workspace-settings?workspace_id=ws-1`,
      init: { method: "PATCH", body: JSON.stringify(payload) },
    });
  });
});

describe("Azure DevOps read API", () => {
  it("reads team boards and updates a board work item with workspace scope", async () => {
    fetchSpy.mockResolvedValue(jsonResponse({ teams: [] }));
    await listAzureDevOpsTeams("ws-1", "project / one");
    expect(lastCall().url).toBe(`${BASE}/teams?project=project+%2F+one&workspace_id=ws-1`);

    fetchSpy.mockResolvedValue(jsonResponse({ boards: [] }));
    await listAzureDevOpsBoards("ws-1", "project-1", "team/one");
    expect(lastCall().url).toBe(
      `${BASE}/boards?project=project-1&team=team%2Fone&workspace_id=ws-1`,
    );

    fetchSpy.mockResolvedValue(jsonResponse({ board: {}, items: [] }));
    await getAzureDevOpsBoardSnapshot("ws-1", "project-1", "team-1", "board/1");
    expect(lastCall().url).toBe(
      `${BASE}/boards/board%2F1?project=project-1&team=team-1&workspace_id=ws-1`,
    );

    fetchSpy.mockResolvedValue(jsonResponse({ id: 101 }));
    await updateAzureDevOpsBoardWorkItem("ws-1", "project-1", "team-1", "board-1", 101, {
      revision: 7,
      assigneeAction: "assign_current_user",
    });
    expect(lastCall()).toMatchObject({
      url: `${BASE}/boards/board-1/work-items/101?project=project-1&team=team-1&workspace_id=ws-1`,
      init: {
        method: "PATCH",
        body: JSON.stringify({ revision: 7, assigneeAction: "assign_current_user" }),
      },
    });
  });

  it("encodes project and repository filters with workspace scope", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ projects: [] }));
    await listAzureDevOpsProjects("ws-1");
    expect(lastCall().url).toBe(`${BASE}/projects?workspace_id=ws-1`);

    fetchSpy.mockResolvedValueOnce(jsonResponse({ repositories: [] }));
    await listAzureDevOpsRepositories("ws-1", "project / one");
    expect(lastCall().url).toBe(`${BASE}/repositories?project=project+%2F+one&workspace_id=ws-1`);

    fetchSpy.mockResolvedValueOnce(jsonResponse({ branches: [] }));
    await listAzureDevOpsBranches("ws-1", "acme", "project / one", "repo / one");
    expect(lastCall().url).toBe(
      `${BASE}/branches?organization=acme&project=project+%2F+one&repository=repo+%2F+one&workspace_id=ws-1`,
    );
  });

  it("posts WIQL and builds pull request filters", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ items: [], count: 0 }));
    await searchAzureDevOpsWorkItems("ws-1", {
      project: "project-1",
      wiql: "SELECT [System.Id] FROM WorkItems",
      top: 50,
    });
    expect(lastCall()).toMatchObject({
      url: `${BASE}/work-items/search?workspace_id=ws-1`,
      init: {
        method: "POST",
        body: JSON.stringify({
          project: "project-1",
          wiql: "SELECT [System.Id] FROM WorkItems",
          top: 50,
        }),
      },
    });

    fetchSpy.mockResolvedValueOnce(jsonResponse({ items: [], count: 0, skip: 0, top: 25 }));
    await listAzureDevOpsPullRequests("ws-1", {
      project: "project-1",
      repository: "repo-1",
      status: "active",
      reviewer: "me",
      top: 25,
    });
    expect(lastCall().url).toContain("project=project-1");
    expect(lastCall().url).toContain("repository=repo-1");
    expect(lastCall().url).toContain("reviewer=me");
    expect(lastCall().url).toContain("workspace_id=ws-1");
  });

  it("encodes provider identifiers in feedback paths", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        pullRequest: {},
        reviewers: [],
        threads: [],
        linkedWorkItems: [],
        policies: [],
        reviewState: "",
        policyState: "",
      }),
    );
    await getAzureDevOpsPullRequestFeedback("ws-1", "project/one", "repo one", 42);
    expect(lastCall().url).toBe(
      `${BASE}/pull-requests/project%2Fone/repo%20one/42/feedback?workspace_id=ws-1`,
    );
  });
});

describe("Azure DevOps task pull request API", () => {
  it("lists workspace associations through the workspace path", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ taskPrs: {} }));
    await listWorkspaceAzureDevOpsTaskPullRequests("ws / one");
    expect(lastCall().url).toBe(`${BASE}/workspaces/ws%20%2F%20one/task-prs`);
  });

  it("associates and syncs pull requests with explicit workspace scope", async () => {
    const payload = {
      repositoryId: "local-repo",
      pullRequestId: 42,
    };
    fetchSpy.mockImplementation(async () => jsonResponse({ id: "association-1" }));

    await associateAzureDevOpsPullRequest("ws-1", "task/1", payload);
    expect(lastCall()).toMatchObject({
      url: `${BASE}/tasks/task%2F1/pull-requests?workspace_id=ws-1`,
      init: { method: "POST", body: JSON.stringify(payload) },
    });

    await syncAzureDevOpsTaskPullRequest("ws-1", "task/1", payload);
    expect(lastCall().url).toBe(`${BASE}/tasks/task%2F1/pull-requests/sync?workspace_id=ws-1`);
  });
});

describe("Azure DevOps task work-item API", () => {
  it("lists and associates work items with explicit workspace scope", async () => {
    fetchSpy.mockImplementation(async () => jsonResponse({ taskWorkItems: {} }));

    await listWorkspaceAzureDevOpsTaskWorkItems("ws / one");
    expect(lastCall().url).toBe(`${BASE}/workspaces/ws%20%2F%20one/task-work-items`);

    const payload = { projectId: "project-1", workItemId: 42 };
    await associateAzureDevOpsWorkItem("ws-1", "task/1", payload);
    expect(lastCall()).toMatchObject({
      url: `${BASE}/tasks/task%2F1/work-items?workspace_id=ws-1`,
      init: { method: "POST", body: JSON.stringify(payload) },
    });
  });
});
