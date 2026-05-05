import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Pin the backend config to a deterministic base so URL assertions don't
// depend on whatever environment the tests inherit.
vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import {
  createLinearIssueWatch,
  deleteLinearConfig,
  deleteLinearIssueWatch,
  getLinearConfig,
  getLinearIssue,
  listLinearIssueWatches,
  listLinearStates,
  listLinearTeams,
  searchLinearIssues,
  setLinearConfig,
  setLinearIssueState,
  testLinearConnection,
  triggerLinearIssueWatch,
  updateLinearIssueWatch,
} from "./linear-api";

const BASE = "http://api.test/api/v1/linear";
const CONFIG_URL = `${BASE}/config`;
const AUTH = "api_key" as const;

type FetchInput = Parameters<typeof fetch>[0];
type FetchInit = Parameters<typeof fetch>[1];

const fetchSpy = vi.fn<[FetchInput, FetchInit?], Promise<Response>>();

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

function noContent(): Response {
  return new Response(null, { status: 204 });
}

function lastCall(): { url: string; init: FetchInit | undefined } {
  const call = fetchSpy.mock.calls.at(-1);
  if (!call) throw new Error("expected fetch to have been called");
  return { url: String(call[0]), init: call[1] };
}

describe("getLinearConfig", () => {
  it("returns null on 204 No Content (not configured yet)", async () => {
    fetchSpy.mockResolvedValueOnce(noContent());
    const cfg = await getLinearConfig();
    expect(cfg).toBeNull();
  });

  it("hits the install-wide /config route with no query params", async () => {
    fetchSpy.mockResolvedValueOnce(noContent());
    await getLinearConfig();
    expect(lastCall().url).toBe(CONFIG_URL);
  });

  it("returns the parsed config on 200", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        authMethod: AUTH,
        defaultTeamKey: "ENG",
        hasSecret: true,
        lastOk: true,
        createdAt: "2026-01-01T00:00:00Z",
        updatedAt: "2026-01-01T00:00:00Z",
      }),
    );
    const cfg = await getLinearConfig();
    expect(cfg?.defaultTeamKey).toBe("ENG");
  });
});

describe("setLinearConfig", () => {
  it("POSTs the payload to /api/v1/linear/config", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ authMethod: AUTH }));
    await setLinearConfig({ authMethod: AUTH, secret: "tok" });
    const { url, init } = lastCall();
    expect(url).toBe(CONFIG_URL);
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toMatchObject({
      authMethod: AUTH,
      secret: "tok",
    });
  });
});

describe("deleteLinearConfig", () => {
  it("issues DELETE on /config", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ deleted: true }));
    await deleteLinearConfig();
    const { url, init } = lastCall();
    expect(url).toBe(CONFIG_URL);
    expect(init?.method).toBe("DELETE");
  });
});

describe("testLinearConnection", () => {
  it("POSTs to /api/v1/linear/config/test", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ ok: true }));
    await testLinearConnection({ authMethod: AUTH, secret: "x" });
    const { url, init } = lastCall();
    expect(url).toBe(`${CONFIG_URL}/test`);
    expect(init?.method).toBe("POST");
  });
});

describe("listLinearTeams + listLinearStates", () => {
  it("listLinearTeams targets /api/v1/linear/teams", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ teams: [] }));
    await listLinearTeams();
    expect(lastCall().url).toBe(`${BASE}/teams`);
  });

  it("listLinearStates includes the team_key", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ states: [] }));
    await listLinearStates("ENG");
    expect(lastCall().url).toBe(`${BASE}/states?team_key=ENG`);
  });
});

describe("searchLinearIssues", () => {
  it("joins stateIds as a CSV in the state_ids query param", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ issues: [], maxResults: 25, isLast: true }));
    await searchLinearIssues({ stateIds: ["s1", "s2", "s3"] });
    const { url } = lastCall();
    expect(url).toContain("state_ids=s1%2Cs2%2Cs3");
  });

  it("omits empty optional filters from the URL", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ issues: [], maxResults: 25, isLast: true }));
    await searchLinearIssues({});
    const { url } = lastCall();
    expect(url).toBe(`${BASE}/issues`);
  });

  it("encodes a multi-word query string", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ issues: [], maxResults: 25, isLast: true }));
    await searchLinearIssues({ query: "fix login & signup" });
    const { url } = lastCall();
    // URLSearchParams uses + for spaces, %26 for & — both indicate proper encoding.
    expect(url).toContain("query=fix+login+%26+signup");
  });
});

describe("getLinearIssue", () => {
  it("URL-encodes the identifier as a path segment", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ id: "i1", identifier: "ENG/1" }));
    await getLinearIssue("ENG/1");
    expect(lastCall().url).toBe(`${BASE}/issues/ENG%2F1`);
  });
});

describe("setLinearIssueState", () => {
  it("POSTs { stateId } in the body to the issue's /state route", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ transitioned: true }));
    await setLinearIssueState("ENG-1", "state-id-123");
    const { url, init } = lastCall();
    expect(url).toBe(`${BASE}/issues/ENG-1/state`);
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual({ stateId: "state-id-123" });
  });
});

// Issue watches: workspace_id is a query param on list/update/delete/trigger
// but absent on create — exactly the kind of subtle URL-construction
// difference that's worth catching at the API client boundary.
describe("listLinearIssueWatches", () => {
  it("hits /watches/issue with no query param when workspaceId omitted", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ watches: [] }));
    await listLinearIssueWatches();
    expect(lastCall().url).toBe(`${BASE}/watches/issue`);
  });

  it("appends workspace_id query param when provided", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ watches: [] }));
    await listLinearIssueWatches("ws-1");
    expect(lastCall().url).toBe(`${BASE}/watches/issue?workspace_id=ws-1`);
  });

  it("returns an empty array when the response has no watches field", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({}));
    await expect(listLinearIssueWatches()).resolves.toEqual([]);
  });
});

describe("createLinearIssueWatch", () => {
  it("POSTs to /watches/issue without workspace_id in the URL", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ id: "w1" }));
    await createLinearIssueWatch({
      workspaceId: "ws-1",
      workflowId: "wf-1",
      workflowStepId: "step-1",
      filter: { teamKey: "ENG" },
    });
    const { url, init } = lastCall();
    expect(url).toBe(`${BASE}/watches/issue`);
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toMatchObject({
      workspaceId: "ws-1",
      filter: { teamKey: "ENG" },
    });
  });
});

describe("updateLinearIssueWatch", () => {
  it("PATCHes /watches/issue/:id with workspace_id query param", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ id: "w1" }));
    await updateLinearIssueWatch("ws-1", "w1", { enabled: false });
    const { url, init } = lastCall();
    expect(url).toBe(`${BASE}/watches/issue/w1?workspace_id=ws-1`);
    expect(init?.method).toBe("PATCH");
    expect(JSON.parse(String(init?.body))).toEqual({ enabled: false });
  });

  it("URL-encodes both id and workspace_id", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ id: "w/1" }));
    await updateLinearIssueWatch("ws/space", "w/1", {});
    expect(lastCall().url).toBe(`${BASE}/watches/issue/w%2F1?workspace_id=ws%2Fspace`);
  });
});

describe("deleteLinearIssueWatch", () => {
  it("issues DELETE on /watches/issue/:id with workspace_id query param", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ deleted: true }));
    await deleteLinearIssueWatch("ws-1", "w1");
    const { url, init } = lastCall();
    expect(url).toBe(`${BASE}/watches/issue/w1?workspace_id=ws-1`);
    expect(init?.method).toBe("DELETE");
  });
});

describe("triggerLinearIssueWatch", () => {
  it("POSTs to /watches/issue/:id/trigger with workspace_id query param", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ newIssues: 3 }));
    const res = await triggerLinearIssueWatch("ws-1", "w1");
    const { url, init } = lastCall();
    expect(url).toBe(`${BASE}/watches/issue/w1/trigger?workspace_id=ws-1`);
    expect(init?.method).toBe("POST");
    expect(res.newIssues).toBe(3);
  });
});
