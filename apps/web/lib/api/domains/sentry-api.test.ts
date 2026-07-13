import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import { copySentryInstances } from "./sentry-api";

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

describe("copySentryInstances", () => {
  it("scopes the copy to its source workspace and sends only the target in JSON", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ instances: [] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await copySentryInstances("ws-target", { workspaceId: "ws-source" });

    const [url, init] = fetchSpy.mock.calls.at(-1) ?? [];
    expect(String(url)).toBe("http://api.test/api/v1/sentry/config/copy?workspace_id=ws-source");
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual({ targetWorkspaceId: "ws-target" });
  });
});
