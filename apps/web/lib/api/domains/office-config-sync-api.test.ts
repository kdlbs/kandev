import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  getOfficeConfigSyncConfig,
  setOfficeConfigSyncConfig,
  deleteOfficeConfigSyncConfig,
  forceOfficeConfigSync,
} from "./office-config-sync-api";

const WORKSPACE_ID = "ws-1";
const CONFIG_PATH = `/api/v1/office/workspaces/${WORKSPACE_ID}/config-sync/config`;
const SYNC_PATH = `/api/v1/office/workspaces/${WORKSPACE_ID}/config-sync/sync`;
const REPO_NAME = "kandev-office-config";

const originalFetch = global.fetch;

function mockResponse(data: unknown, status = 200) {
  return new Response(data === undefined ? null : JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function makeConfigBody(overrides: Record<string, unknown> = {}) {
  return {
    workspace_id: WORKSPACE_ID,
    provider: "github",
    repo_owner: "kdlbs",
    repo_name: REPO_NAME,
    project_path: "",
    branch: "main",
    path: "",
    interval_seconds: 300,
    poll_enabled: true,
    last_ok: true,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

describe("office-config-sync-api", () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchSpy = vi.fn();
    global.fetch = fetchSpy as unknown as typeof fetch;
  });

  afterEach(() => {
    global.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it("getOfficeConfigSyncConfig calls the workspace-scoped config path", async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse(makeConfigBody()));
    const config = await getOfficeConfigSyncConfig(WORKSPACE_ID);
    expect(fetchSpy).toHaveBeenCalledTimes(1);
    const url = String(fetchSpy.mock.calls[0]![0]);
    expect(url).toContain(CONFIG_PATH);
    expect(config?.repo_owner).toBe("kdlbs");
  });

  it("getOfficeConfigSyncConfig returns null on 204 (not configured)", async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse(undefined, 204));
    const config = await getOfficeConfigSyncConfig(WORKSPACE_ID);
    expect(config).toBeNull();
  });

  it("setOfficeConfigSyncConfig POSTs the payload to the config path", async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse(makeConfigBody({ last_ok: false })));
    const saved = await setOfficeConfigSyncConfig(WORKSPACE_ID, {
      provider: "github",
      repo_owner: "kdlbs",
      repo_name: REPO_NAME,
    });
    const url = String(fetchSpy.mock.calls[0]![0]);
    const init = fetchSpy.mock.calls[0]![1] as RequestInit;
    expect(url).toContain(CONFIG_PATH);
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body as string)).toEqual({
      provider: "github",
      repo_owner: "kdlbs",
      repo_name: REPO_NAME,
    });
    expect(saved.repo_name).toBe(REPO_NAME);
  });

  it("deleteOfficeConfigSyncConfig issues DELETE against the config path", async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse({ deleted: true }));
    const result = await deleteOfficeConfigSyncConfig(WORKSPACE_ID);
    const url = String(fetchSpy.mock.calls[0]![0]);
    const init = fetchSpy.mock.calls[0]![1] as RequestInit;
    expect(url).toContain(CONFIG_PATH);
    expect(init.method).toBe("DELETE");
    expect(result.deleted).toBe(true);
  });

  it("forceOfficeConfigSync POSTs to the sync path and returns config + result", async () => {
    fetchSpy.mockResolvedValueOnce(
      mockResponse({
        config: makeConfigBody(),
        result: {
          created: ["agent.yaml"],
          updated: [],
          deleted: [],
          warnings: [],
          unchanged: false,
        },
      }),
    );
    const res = await forceOfficeConfigSync(WORKSPACE_ID);
    const url = String(fetchSpy.mock.calls[0]![0]);
    const init = fetchSpy.mock.calls[0]![1] as RequestInit;
    expect(url).toContain(SYNC_PATH);
    expect(init.method).toBe("POST");
    expect(res.config.repo_owner).toBe("kdlbs");
    expect(res.result?.created).toEqual(["agent.yaml"]);
  });

  it("forceOfficeConfigSync rejects with the parsed error body on 404 (not configured)", async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse({ error: "not configured" }, 404));
    await expect(forceOfficeConfigSync(WORKSPACE_ID)).rejects.toThrow("not configured");
  });

  it("forceOfficeConfigSync resolves 200 even when the sync itself failed", async () => {
    fetchSpy.mockResolvedValueOnce(
      mockResponse({
        config: makeConfigBody({ last_ok: false, last_error: "clone failed" }),
        error: "clone failed",
      }),
    );
    const res = await forceOfficeConfigSync(WORKSPACE_ID);
    expect(res.config.last_ok).toBe(false);
    expect(res.error).toBe("clone failed");
  });
});
