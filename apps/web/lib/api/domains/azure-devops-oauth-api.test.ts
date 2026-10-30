import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import {
  completeAzureDevOpsOAuth,
  signOutAzureDevOpsOAuth,
  getAzureDevOpsOAuthAvailability,
  startAzureDevOpsOAuth,
} from "./azure-devops-oauth-api";

const BASE = "http://api.test/api/v1/azure-devops/oauth";
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

describe("Azure DevOps Microsoft sign-in API", () => {
  it("reads availability and the installation defaults for the workspace", async () => {
    const availability = {
      available: true,
      defaultClientId: "client-1",
      defaultRedirectUrl: "http://localhost:38429/api/v1/azure-devops/oauth/callback",
    };
    fetchSpy.mockResolvedValueOnce(jsonResponse(availability));

    await expect(getAzureDevOpsOAuthAvailability("ws one")).resolves.toEqual(availability);
    expect(lastCall().url).toBe(`${BASE}/availability?workspace_id=ws+one`);
  });

  it("starts sign-in with the organization and default project", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ authUrl: "https://login/x", state: "s1" }));
    const payload = {
      organizationUrl: "https://dev.azure.com/acme",
      defaultProjectId: "p-1",
      defaultProjectName: "Platform",
    };

    await expect(startAzureDevOpsOAuth("ws-1", payload)).resolves.toEqual({
      authUrl: "https://login/x",
      state: "s1",
    });
    expect(lastCall()).toMatchObject({
      url: `${BASE}/start?workspace_id=ws-1`,
      init: { method: "POST", body: JSON.stringify(payload) },
    });
  });

  it("completes sign-in with the code and state", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ authMethod: "oauth", hasSecret: true }));

    await completeAzureDevOpsOAuth("ws-1", { code: "c1", state: "s1" });
    expect(lastCall()).toMatchObject({
      url: `${BASE}/complete?workspace_id=ws-1`,
      init: { method: "POST", body: JSON.stringify({ code: "c1", state: "s1" }) },
    });
  });

  it("signs the workspace out of its Microsoft account", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ authMethod: "oauth", hasSecret: false }));

    await signOutAzureDevOpsOAuth("ws-1");
    expect(lastCall()).toMatchObject({
      url: `${BASE}/sign-out?workspace_id=ws-1`,
      init: { method: "POST" },
    });
  });
});
