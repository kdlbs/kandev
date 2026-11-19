import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  deletePromptSyncConfig,
  getPromptSyncConfig,
  runPromptSync,
  setPromptSyncConfig,
} from "./prompt-sync-api";

const originalFetch = global.fetch;

function mockResponse(data: unknown, status = 200) {
  return new Response(data === undefined ? null : JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const config = {
  workspace_id: "ws-1",
  provider: "gitlab",
  project_path: "acme/eng",
  branch: "main",
  path: ".kandev/prompts",
  interval_seconds: 300,
  poll_enabled: true,
  last_ok: true,
};

describe("prompt-sync-api", () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchSpy = vi.fn();
    global.fetch = fetchSpy as unknown as typeof fetch;
  });

  afterEach(() => {
    global.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it("reads the install-wide config without a workspace parameter", async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse(config));
    const result = await getPromptSyncConfig();
    const url = fetchSpy.mock.calls[0]![0] as string;
    expect(url).toContain("/api/v1/prompt-sync/config");
    expect(url).not.toContain("workspace_id");
    expect(result?.project_path).toBe("acme/eng");
  });

  it("returns null when not configured", async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse(undefined, 204));
    expect(await getPromptSyncConfig()).toBeNull();
  });

  it("saves, removes, and runs with the expected methods", async () => {
    fetchSpy
      .mockResolvedValueOnce(mockResponse(config))
      .mockResolvedValueOnce(mockResponse({ deleted: true }))
      .mockResolvedValueOnce(mockResponse({ config, result: { created: ["a"] } }));
    await setPromptSyncConfig({
      workspace_id: "ws-1",
      provider: "gitlab",
      project_path: "acme/eng",
    });
    await deletePromptSyncConfig();
    const run = await runPromptSync();
    const calls = fetchSpy.mock.calls as [string, RequestInit][];
    expect(calls[0]![1].method).toBe("POST");
    expect(JSON.parse(calls[0]![1].body as string).workspace_id).toBe("ws-1");
    expect(calls[1]![1].method).toBe("DELETE");
    expect(calls[2]![0]).toContain("/api/v1/prompt-sync/sync");
    expect(run.result?.created).toEqual(["a"]);
  });
});
