import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { getWorkspacePause, postWorkspacePause, postWorkspaceResume } from "./office-pause-api";

const WORKSPACE_ID = "ws-1";
const PAUSE_PATH = `/api/v1/office/workspaces/${WORKSPACE_ID}/pause`;
const RESUME_PATH = `/api/v1/office/workspaces/${WORKSPACE_ID}/resume`;

const originalFetch = global.fetch;

function mockResponse(data: unknown, status = 200) {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function rawPause(overrides: Record<string, unknown> = {}) {
  return {
    id: "pause-1",
    reason: "incident",
    created_by: "default-user",
    created_by_kind: "user",
    created_at: "2026-09-08T00:00:00Z",
    ...overrides,
  };
}

describe("office-pause-api", () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchSpy = vi.fn();
    global.fetch = fetchSpy as unknown as typeof fetch;
  });

  afterEach(() => {
    global.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it("getWorkspacePause GETs the workspace-scoped pause path and normalizes snake_case fields", async () => {
    fetchSpy.mockResolvedValueOnce(
      mockResponse({ workspace_id: WORKSPACE_ID, paused: true, pause: rawPause() }),
    );
    const res = await getWorkspacePause(WORKSPACE_ID);
    expect(String(fetchSpy.mock.calls[0]![0])).toContain(PAUSE_PATH);
    expect(res).toEqual({
      workspaceId: WORKSPACE_ID,
      paused: true,
      record: {
        id: "pause-1",
        reason: "incident",
        createdBy: "default-user",
        createdByKind: "user",
        createdAt: "2026-09-08T00:00:00Z",
      },
    });
  });

  it("getWorkspacePause normalizes a running workspace's null pause to a null record", async () => {
    fetchSpy.mockResolvedValueOnce(
      mockResponse({ workspace_id: WORKSPACE_ID, paused: false, pause: null }),
    );
    const res = await getWorkspacePause(WORKSPACE_ID);
    expect(res).toEqual({ workspaceId: WORKSPACE_ID, paused: false, record: null });
  });

  it("postWorkspacePause POSTs the reason to the pause path", async () => {
    fetchSpy.mockResolvedValueOnce(
      mockResponse({ workspace_id: WORKSPACE_ID, paused: true, pause: rawPause() }),
    );
    const res = await postWorkspacePause(WORKSPACE_ID, "incident");
    const url = String(fetchSpy.mock.calls[0]![0]);
    const init = fetchSpy.mock.calls[0]![1] as RequestInit;
    expect(url).toContain(PAUSE_PATH);
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body as string)).toEqual({ reason: "incident" });
    expect(res.paused).toBe(true);
  });

  it("postWorkspaceResume POSTs to the resume path and normalizes the cleared record", async () => {
    fetchSpy.mockResolvedValueOnce(
      mockResponse({ workspace_id: WORKSPACE_ID, paused: false, pause: null }),
    );
    const res = await postWorkspaceResume(WORKSPACE_ID, "");
    const url = String(fetchSpy.mock.calls[0]![0]);
    const init = fetchSpy.mock.calls[0]![1] as RequestInit;
    expect(url).toContain(RESUME_PATH);
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body as string)).toEqual({ reason: "" });
    expect(res).toEqual({ workspaceId: WORKSPACE_ID, paused: false, record: null });
  });

  it("propagates a non-2xx response as an ApiError", async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse({ error: "workspace not found" }, 404));
    await expect(getWorkspacePause("does-not-exist")).rejects.toMatchObject({
      status: 404,
    });
  });
});
