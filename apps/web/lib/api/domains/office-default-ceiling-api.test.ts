import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import { getDefaultCeiling, setDefaultCeiling } from "./office-api";

type FetchInput = Parameters<typeof fetch>[0];
type FetchInit = Parameters<typeof fetch>[1];
const fetchSpy = vi.fn<(...args: [FetchInput, FetchInit?]) => Promise<Response>>();
const WORKSPACE_ID = "workspace-1";
const URL = `http://api.test/api/v1/office/workspaces/${WORKSPACE_ID}/budgets/default`;

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

describe("built-in default ceiling api", () => {
  it("reads the effective limit from the snake_case wire response", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ limit_subcents: 500_000 }));

    const res = await getDefaultCeiling(WORKSPACE_ID);

    expect(lastCall().url).toBe(URL);
    expect(res.limit_subcents).toBe(500_000);
  });

  it("PUTs a snake_case body and returns the updated limit", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ limit_subcents: 1_000_000 }));

    const res = await setDefaultCeiling(WORKSPACE_ID, 1_000_000);

    expect(lastCall().url).toBe(URL);
    expect(lastCall().init?.method).toBe("PUT");
    expect(JSON.parse(String(lastCall().init?.body))).toEqual({ limit_subcents: 1_000_000 });
    expect(res.limit_subcents).toBe(1_000_000);
  });
});
