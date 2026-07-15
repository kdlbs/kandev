import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  getWorkflowSyncConfig,
  setWorkflowSyncConfig,
  deleteWorkflowSyncConfig,
  forceWorkflowSync,
} from "./workflow-sync-api";

const originalFetch = global.fetch;

function mockResponse(data: unknown, status = 200) {
  return new Response(data === undefined ? null : JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function makeConfigBody(overrides: Record<string, unknown> = {}) {
  return {
    workspace_id: "ws-1",
    repo_owner: "kdlbs",
    repo_name: "kandev",
    branch: "main",
    path: ".kandev/workflows",
    interval_seconds: 300,
    last_ok: true,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

describe("workflow-sync-api", () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchSpy = vi.fn();
    global.fetch = fetchSpy as unknown as typeof fetch;
  });

  afterEach(() => {
    global.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it("getWorkflowSyncConfig calls /api/v1/workflow-sync/config with workspace_id", async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse(makeConfigBody()));
    const config = await getWorkflowSyncConfig({ workspaceId: "ws-1" });
    expect(fetchSpy).toHaveBeenCalledTimes(1);
    const url = fetchSpy.mock.calls[0]![0] as string;
    expect(url).toContain("/api/v1/workflow-sync/config");
    expect(url).toContain("workspace_id=ws-1");
    expect(config?.repo_owner).toBe("kdlbs");
  });

  it("getWorkflowSyncConfig returns null on 204 (not configured)", async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse(undefined, 204));
    const config = await getWorkflowSyncConfig({ workspaceId: "ws-1" });
    expect(config).toBeNull();
  });

  it("setWorkflowSyncConfig POSTs the payload", async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse(makeConfigBody({ last_ok: false })));
    const saved = await setWorkflowSyncConfig(
      { repo_owner: "kdlbs", repo_name: "kandev" },
      { workspaceId: "ws-1" },
    );
    const url = fetchSpy.mock.calls[0]![0] as string;
    const init = fetchSpy.mock.calls[0]![1] as RequestInit;
    expect(url).toContain("/api/v1/workflow-sync/config");
    expect(url).toContain("workspace_id=ws-1");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body as string)).toEqual({ repo_owner: "kdlbs", repo_name: "kandev" });
    expect(saved.repo_name).toBe("kandev");
  });

  it("deleteWorkflowSyncConfig issues DELETE", async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse({ deleted: true }));
    const result = await deleteWorkflowSyncConfig({ workspaceId: "ws-1" });
    const init = fetchSpy.mock.calls[0]![1] as RequestInit;
    expect(init.method).toBe("DELETE");
    expect(result.deleted).toBe(true);
  });

  it("forceWorkflowSync POSTs with no body and returns config + result", async () => {
    fetchSpy.mockResolvedValueOnce(
      mockResponse({
        config: makeConfigBody(),
        result: { created: ["a"], updated: [], deleted: [], warnings: [], unchanged: false },
      }),
    );
    const res = await forceWorkflowSync({ workspaceId: "ws-1" });
    const url = fetchSpy.mock.calls[0]![0] as string;
    const init = fetchSpy.mock.calls[0]![1] as RequestInit;
    expect(url).toContain("/api/v1/workflow-sync/sync");
    expect(url).toContain("workspace_id=ws-1");
    expect(init.method).toBe("POST");
    expect(res.config.repo_owner).toBe("kdlbs");
    expect(res.result?.created).toEqual(["a"]);
  });

  it("forceWorkflowSync rejects with the parsed error body on 404 (not configured)", async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse({ error: "not configured" }, 404));
    await expect(forceWorkflowSync({ workspaceId: "ws-1" })).rejects.toThrow("not configured");
  });
});
