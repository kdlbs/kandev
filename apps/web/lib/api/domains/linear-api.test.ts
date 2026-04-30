import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Pin the backend config to a deterministic base so URL assertions don't
// depend on whatever environment the tests inherit.
vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import {
  deleteLinearConfig,
  getLinearConfig,
  getLinearIssue,
  listLinearStates,
  listLinearTeams,
  searchLinearIssues,
  setLinearConfig,
  setLinearIssueState,
  testLinearConnection,
} from "./linear-api";

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
    const cfg = await getLinearConfig("ws-1");
    expect(cfg).toBeNull();
  });

  it("URL-encodes the workspace id", async () => {
    fetchSpy.mockResolvedValueOnce(noContent());
    await getLinearConfig("ws/with space");
    expect(lastCall().url).toBe(
      "http://api.test/api/v1/linear/config?workspace_id=ws%2Fwith%20space",
    );
  });

  it("returns the parsed config on 200", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        workspaceId: "ws-1",
        authMethod: "api_key",
        defaultTeamKey: "ENG",
        hasSecret: true,
        lastOk: true,
        createdAt: "2026-01-01T00:00:00Z",
        updatedAt: "2026-01-01T00:00:00Z",
      }),
    );
    const cfg = await getLinearConfig("ws-1");
    expect(cfg?.defaultTeamKey).toBe("ENG");
  });
});

describe("setLinearConfig", () => {
  it("POSTs the payload to /api/v1/linear/config", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ workspaceId: "ws-1" }));
    await setLinearConfig({ workspaceId: "ws-1", authMethod: "api_key", secret: "tok" });
    const { url, init } = lastCall();
    expect(url).toBe("http://api.test/api/v1/linear/config");
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toMatchObject({
      workspaceId: "ws-1",
      secret: "tok",
    });
  });
});

describe("deleteLinearConfig", () => {
  it("issues DELETE with the workspace id in the query string", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ deleted: true }));
    await deleteLinearConfig("ws-1");
    const { url, init } = lastCall();
    expect(url).toBe("http://api.test/api/v1/linear/config?workspace_id=ws-1");
    expect(init?.method).toBe("DELETE");
  });
});

describe("testLinearConnection", () => {
  it("POSTs to /api/v1/linear/config/test", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ ok: true }));
    await testLinearConnection({ workspaceId: "ws-1", authMethod: "api_key", secret: "x" });
    const { url, init } = lastCall();
    expect(url).toBe("http://api.test/api/v1/linear/config/test");
    expect(init?.method).toBe("POST");
  });
});

describe("listLinearTeams + listLinearStates", () => {
  it("listLinearTeams targets /api/v1/linear/teams", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ teams: [] }));
    await listLinearTeams("ws-1");
    expect(lastCall().url).toBe("http://api.test/api/v1/linear/teams?workspace_id=ws-1");
  });

  it("listLinearStates includes the team_key", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ states: [] }));
    await listLinearStates("ws-1", "ENG");
    expect(lastCall().url).toBe(
      "http://api.test/api/v1/linear/states?workspace_id=ws-1&team_key=ENG",
    );
  });
});

describe("searchLinearIssues", () => {
  it("joins stateIds as a CSV in the state_ids query param", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ issues: [], maxResults: 25, isLast: true }));
    await searchLinearIssues("ws-1", { stateIds: ["s1", "s2", "s3"] });
    const { url } = lastCall();
    expect(url).toContain("state_ids=s1%2Cs2%2Cs3");
  });

  it("omits empty optional filters from the URL", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ issues: [], maxResults: 25, isLast: true }));
    await searchLinearIssues("ws-1", {});
    const { url } = lastCall();
    expect(url).toContain("workspace_id=ws-1");
    expect(url).not.toContain("query=");
    expect(url).not.toContain("team_key=");
    expect(url).not.toContain("assigned=");
    expect(url).not.toContain("state_ids=");
    expect(url).not.toContain("page_token=");
    expect(url).not.toContain("max_results=");
  });

  it("encodes a multi-word query string", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ issues: [], maxResults: 25, isLast: true }));
    await searchLinearIssues("ws-1", { query: "fix login & signup" });
    const { url } = lastCall();
    // URLSearchParams uses + for spaces, %26 for & — both indicate proper encoding.
    expect(url).toContain("query=fix+login+%26+signup");
  });
});

describe("getLinearIssue", () => {
  it("URL-encodes the identifier as a path segment and workspaceId as a query param", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ id: "i1", identifier: "ENG/1" }));
    await getLinearIssue("ws/space", "ENG/1");
    expect(lastCall().url).toBe(
      "http://api.test/api/v1/linear/issues/ENG%2F1?workspace_id=ws%2Fspace",
    );
  });
});

describe("setLinearIssueState", () => {
  it("POSTs { stateId } in the body to the issue's /state route", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ transitioned: true }));
    await setLinearIssueState("ws-1", "ENG-1", "state-id-123");
    const { url, init } = lastCall();
    expect(url).toBe("http://api.test/api/v1/linear/issues/ENG-1/state?workspace_id=ws-1");
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual({ stateId: "state-id-123" });
  });
});
