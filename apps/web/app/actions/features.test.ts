import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://backend.test" }),
}));

import { getRuntimeDebugModeAction } from "./features";

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

describe("getRuntimeDebugModeAction", () => {
  it("returns true when the effective debug runtime flag is enabled", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        flags: [{ key: "debug.devMode", effective_value: true }],
      }),
    );

    await expect(getRuntimeDebugModeAction()).resolves.toBe(true);

    expect(fetchSpy).toHaveBeenCalledWith("http://backend.test/api/v1/runtime-flags", {
      cache: "no-store",
    });
  });

  it("falls back to false when runtime flags cannot be loaded", async () => {
    fetchSpy.mockResolvedValueOnce(new Response("nope", { status: 503 }));

    await expect(getRuntimeDebugModeAction()).resolves.toBe(false);
  });

  it("returns false when the debug runtime flag is absent", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        flags: [{ key: "features.office", effective_value: true }],
      }),
    );

    await expect(getRuntimeDebugModeAction()).resolves.toBe(false);
  });
});
