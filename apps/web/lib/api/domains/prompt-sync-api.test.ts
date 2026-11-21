import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  checkPromptSync,
  getPromptSyncKeptVersions,
  deletePromptSyncConfig,
  getPromptSyncConfig,
  runPromptSync,
  savePromptVersions,
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

describe("prompt sync conflict api", () => {
  afterEach(() => {
    global.fetch = originalFetch;
  });

  it("checks a candidate source or the saved one, and sends conflict choices with Sync now", async () => {
    const spy = vi.fn().mockImplementation(async () => mockResponse({ conflicts: [], config }));
    global.fetch = spy as unknown as typeof fetch;
    await checkPromptSync({ workspace_id: "ws-1", provider: "gitlab", project_path: "acme/eng" });
    await checkPromptSync();
    await runPromptSync({ principles: "repository" });
    const calls = spy.mock.calls as [string, RequestInit][];
    expect(calls[0]![0]).toContain("/api/v1/prompt-sync/check");
    expect(calls[0]![1].method).toBe("POST");
    expect(JSON.parse(calls[0]![1].body as string).project_path).toBe("acme/eng");
    expect(calls[1]![1].body).toBeUndefined();
    expect(JSON.parse(calls[2]![1].body as string)).toEqual({
      resolutions: { principles: "repository" },
    });
  });
});

describe("prompt sync removal api", () => {
  afterEach(() => {
    global.fetch = originalFetch;
  });

  it("lists prompts with two versions and sends the version choices with removal", async () => {
    const spy = vi
      .fn()
      .mockImplementation(async () => mockResponse({ prompts: [], deleted: true }));
    global.fetch = spy as unknown as typeof fetch;
    await getPromptSyncKeptVersions();
    await deletePromptSyncConfig({
      choices: { "house-rules": "local" },
      delete_repository_only: ["principles"],
    });
    await deletePromptSyncConfig();
    const calls = spy.mock.calls as [string, RequestInit][];
    expect(calls[0]![0]).toContain("/api/v1/prompt-sync/kept-versions");
    expect(calls[1]![1].method).toBe("DELETE");
    expect(JSON.parse(calls[1]![1].body as string)).toEqual({
      choices: { "house-rules": "local" },
      delete_repository_only: ["principles"],
    });
    expect(calls[2]![1].body).toBeUndefined();
  });
});

describe("prompt versions api", () => {
  const original = global.fetch;
  afterEach(() => {
    global.fetch = original;
  });

  it("puts the switch and the edited local version for a prompt", async () => {
    const spy = vi.fn().mockImplementation(
      async () =>
        new Response(JSON.stringify({ id: "p 1", source: "override" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
    );
    global.fetch = spy as unknown as typeof fetch;
    await savePromptVersions("p 1", {
      use_local: true,
      local_content: "Mine",
      expected_updated_at: "2026-01-01T00:00:00Z",
    });
    const calls = spy.mock.calls as [string, RequestInit][];
    expect(calls[0]![0]).toContain("/api/v1/prompt-sync/prompts/p%201/versions");
    expect(calls[0]![1].method).toBe("PUT");
    expect(JSON.parse(calls[0]![1].body as string)).toEqual({
      use_local: true,
      local_content: "Mine",
      expected_updated_at: "2026-01-01T00:00:00Z",
    });
  });
});
