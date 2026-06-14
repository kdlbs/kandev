import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import { fetchRuntimeFlags, updateRuntimeFlag } from "./runtime-flags-api";

const BASE = "http://api.test/api/v1/runtime-flags";

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

describe("runtime flags api", () => {
  it("fetchRuntimeFlags forces no-store cache for the runtime flags endpoint", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ flags: [] }));
    const res = await fetchRuntimeFlags({ cache: "force-cache" });
    const { url, init } = lastCall();
    expect(url).toBe(BASE);
    expect((init?.method ?? "GET").toUpperCase()).toBe("GET");
    expect(init?.cache).toBe("no-store");
    expect(res.flags).toEqual([]);
  });

  it("updateRuntimeFlag PATCHes a boolean override", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ flags: [] }));
    await updateRuntimeFlag("features.office", true, {
      init: { method: "GET", body: JSON.stringify({ override: false }) },
    });
    const { url, init } = lastCall();
    expect(url).toBe(`${BASE}/features.office`);
    expect((init?.method ?? "").toUpperCase()).toBe("PATCH");
    expect(init?.body).toBe(JSON.stringify({ override: true }));
  });

  it("updateRuntimeFlag PATCHes null to clear an override", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ flags: [] }));
    await updateRuntimeFlag("debug.devMode", null);
    expect(lastCall().init?.body).toBe(JSON.stringify({ override: null }));
  });
});
